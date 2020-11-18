/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"k8s.io/apimachinery/pkg/util/uuid"
	apimachineryversion "k8s.io/apimachinery/pkg/util/version"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/klog"
	testutils "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/test/e2e/utils"
)

var (
	// Kubernetes cluster flags
	teardownCluster    = flag.Bool("teardown-cluster", true, "teardown the cluster after the e2e test")
	teardownDriver     = flag.Bool("teardown-driver", true, "teardown the driver after the e2e test")
	bringupCluster     = flag.Bool("bringup-cluster", true, "build kubernetes and bringup a cluster")
	platform           = flag.String("platform", "linux", "platform that the tests will be run, either linux or windows")
	gceZone            = flag.String("gce-zone", "", "zone that the gce k8s cluster is created/found in")
	gceRegion          = flag.String("gce-region", "", "region that gke regional cluster should be created in")
	kubeVersion        = flag.String("kube-version", "", "version of Kubernetes to download and use for the cluster")
	testVersion        = flag.String("test-version", "", "version of Kubernetes to download and use for tests")
	kubeFeatureGates   = flag.String("kube-feature-gates", "", "feature gates to set on new kubernetes cluster")
	localK8sDir        = flag.String("local-k8s-dir", "", "local prebuilt kubernetes/kubernetes directory to use for cluster and test binaries")
	deploymentStrat    = flag.String("deployment-strategy", "gce", "choose between deploying on gce or gke")
	gkeClusterVer      = flag.String("gke-cluster-version", "", "version of Kubernetes master and node for gke")
	numNodes           = flag.Int("num-nodes", -1, "the number of nodes in the test cluster")
	imageType          = flag.String("image-type", "cos", "the image type to use for the cluster")
	gkeReleaseChannel  = flag.String("gke-release-channel", "", "GKE release channel to be used for cluster deploy. One of 'rapid', 'stable' or 'regular'")
	gkeTestClusterName = flag.String("gke-cluster-name", "gcp-pd-csi-cluster", "GKE cluster name")
	gkeNodeVersion     = flag.String("gke-node-version", "", "GKE cluster worker node version")
	isRegionalCluster  = flag.Bool("is-regional-cluster", false, "tell the test that a regional cluster is being used. Should be used for running on an existing regional cluster (ie, --bringup-cluster=false). The test will fail if a zonal GKE cluster is created when this flag is true")

	// Test infrastructure flags
	boskosResourceType = flag.String("boskos-resource-type", "gce-project", "name of the boskos resource type to reserve")
	storageClassFiles  = flag.String("storageclass-files", "", "name of storageclass yaml file to use for test relative to test/k8s-integration/config. This may be a comma-separated list to test multiple storage classes")
	snapshotClassFile  = flag.String("snapshotclass-file", "", "name of snapshotclass yaml file to use for test relative to test/k8s-integration/config")
	inProw             = flag.Bool("run-in-prow", false, "is the test running in PROW")

	// Driver flags
	stagingImage        = flag.String("staging-image", "", "name of image to stage to")
	saFile              = flag.String("service-account-file", "", "path of service account file")
	deployOverlayName   = flag.String("deploy-overlay-name", "", "which kustomize overlay to deploy the driver with")
	doDriverBuild       = flag.Bool("do-driver-build", true, "building the driver from source")
	useGKEManagedDriver = flag.Bool("use-gke-managed-driver", false, "use GKE managed PD CSI driver for the tests")

	// Test flags
	migrationTest = flag.Bool("migration-test", false, "sets the flag on the e2e binary signalling migration")
	testFocus     = flag.String("test-focus", "", "test focus for Kubernetes e2e")
)

const (
	pdImagePlaceholder        = "gke.gcr.io/gcp-compute-persistent-disk-csi-driver"
	k8sInDockerBuildBinDir    = "_output/dockerized/bin/linux/amd64"
	k8sOutOfDockerBuildBinDir = "_output/bin"
	externalDriverNamespace   = "gce-pd-csi-driver"
	managedDriverNamespace    = "kube-system"
	regionalPDStorageClass    = "sc-regional.yaml"
)

type testParameters struct {
	platform             string
	stagingVersion       string
	goPath               string
	pkgDir               string
	testParentDir        string
	testDir              string
	testFocus            string
	testSkip             string
	snapshotClassFile    string
	cloudProviderArgs    []string
	deploymentStrategy   string
	outputDir            string
	allowedNotReadyNodes int
	useGKEManagedDriver  bool
	clusterVersion       string
	nodeVersion          string
	imageType            string
}

func init() {
	flag.Set("logtostderr", "true")
}

func main() {
	klog.InitFlags(nil)
	flag.Set("logtostderr", "true")
	flag.Parse()

	if !*inProw && *doDriverBuild {
		ensureVariable(stagingImage, true, "staging-image is a required flag, please specify the name of image to stage to")
	}

	if *useGKEManagedDriver {
		ensureVariableVal(deploymentStrat, "gke", "deployment strategy must be GKE for using managed driver")
		ensureFlag(doDriverBuild, false, "'do-driver-build' must be false when using GKE managed driver")
		ensureFlag(teardownDriver, false, "'teardown-driver' must be false when using GKE managed driver")
		ensureVariable(stagingImage, false, "'staging-image' must not be set when using GKE managed driver")
		ensureVariable(deployOverlayName, false, "'deploy-overlay-name' must not be set when using GKE managed driver")
	}

	if !*useGKEManagedDriver {
		ensureVariable(deployOverlayName, true, "deploy-overlay-name is a required flag")
		if *deployOverlayName != "noauth" {
			ensureVariable(saFile, true, "service-account-file is a required flag")
		}
	}

	ensureVariable(testFocus, true, "test-focus is a required flag")

	if len(*gceRegion) != 0 {
		ensureVariable(gceZone, false, "gce-zone and gce-region cannot both be set")
	} else {
		ensureVariable(gceZone, true, "One of gce-zone or gce-region must be set")
	}

	if *migrationTest {
		ensureVariable(storageClassFiles, false, "storage-class-file and migration-test cannot both be set")
	} else {
		ensureVariable(storageClassFiles, true, "One of storageclass-file and migration-test must be set")
	}

	if !*bringupCluster {
		ensureVariable(kubeFeatureGates, false, "kube-feature-gates set but not bringing up new cluster")
	} else {
		ensureVariable(imageType, true, "image type is a required flag. Available options include 'cos' and 'ubuntu'")
		if *isRegionalCluster {
			klog.Error("is-regional-cluster can only be set when using an existing cluster")
		}
	}

	if *platform == "windows" {
		ensureFlag(bringupCluster, false, "bringupCluster is set to false if it is for testing in windows cluster")
	}

	if *deploymentStrat == "gke" {
		ensureFlag(migrationTest, false, "Cannot set deployment strategy to 'gke' for migration tests.")
		ensureVariable(kubeVersion, false, "Cannot set kube-version when using deployment strategy 'gke'. Use gke-cluster-version.")
		ensureExactlyOneVariableSet([]*string{gkeClusterVer, gkeReleaseChannel},
			"For GKE cluster deployment, exactly one of 'gke-cluster-version' or 'gke-release-channel' must be set")
		ensureVariable(kubeFeatureGates, false, "Cannot set feature gates when using deployment strategy 'gke'.")
		if len(*localK8sDir) == 0 {
			ensureVariable(testVersion, true, "Must set either test-version or local k8s dir when using deployment strategy 'gke'.")
		}
	} else if *deploymentStrat == "gce" {
		ensureVariable(gceRegion, false, "regional clusters not supported for 'gce' deployment")
		ensureVariable(gceZone, true, "gce-zone required for 'gce' deployment")
	}

	if len(*localK8sDir) != 0 {
		ensureVariable(kubeVersion, false, "Cannot set a kube version when using a local k8s dir.")
		ensureVariable(testVersion, false, "Cannot set a test version when using a local k8s dir.")
	}

	if *numNodes == -1 && *bringupCluster {
		klog.Fatalf("num-nodes must be set to number of nodes in cluster")
	}

	err := handle()
	if err != nil {
		klog.Fatalf("Failed to run integration test: %v", err)
	}
}

func handle() error {
	oldmask := syscall.Umask(0000)
	defer syscall.Umask(oldmask)

	testParams := &testParameters{
		platform:            *platform,
		testFocus:           *testFocus,
		snapshotClassFile:   *snapshotClassFile,
		stagingVersion:      string(uuid.NewUUID()),
		deploymentStrategy:  *deploymentStrat,
		useGKEManagedDriver: *useGKEManagedDriver,
		imageType:           *imageType,
	}

	goPath, ok := os.LookupEnv("GOPATH")
	if !ok {
		return fmt.Errorf("Could not find env variable GOPATH")
	}
	testParams.goPath = goPath
	testParams.pkgDir = filepath.Join(goPath, "src", "sigs.k8s.io", "gcp-compute-persistent-disk-csi-driver")

	// If running in Prow, then acquire and set up a project through Boskos
	if *inProw {
		oldProject, err := exec.Command("gcloud", "config", "get-value", "project").CombinedOutput()
		project := strings.TrimSpace(string(oldProject))
		if err != nil {
			return fmt.Errorf("failed to get gcloud project: %s, err: %v", oldProject, err)
		}
		// TODO: Currently for prow tests with linux cluster, here it manually sets up a project from Boskos.
		// For Windows, we used kubernetes_e2e.py which already set up the project and kubernetes automatically.
		// Will update Linux in the future to use the same way as Windows test.
		if *platform != "windows" {
			newproject, _ := testutils.SetupProwConfig(*boskosResourceType)
			err = setEnvProject(newproject)
			if err != nil {
				return fmt.Errorf("failed to set project environment to %s: %v", newproject, err)
			}

			defer func() {
				err = setEnvProject(string(oldProject))
				if err != nil {
					klog.Errorf("failed to set project environment to %s: %v", oldProject, err)
				}
			}()
			project = newproject
		}
		if *doDriverBuild {
			*stagingImage = fmt.Sprintf("gcr.io/%s/gcp-persistent-disk-csi-driver", strings.TrimSpace(string(project)))
		}
		if _, ok := os.LookupEnv("USER"); !ok {
			err = os.Setenv("USER", "prow")
			if err != nil {
				return fmt.Errorf("failed to set user in prow to prow: %v", err)
			}
		}
	}

	// Build and push the driver, if required. Defer the driver image deletion.
	if *doDriverBuild {
		err := pushImage(testParams.pkgDir, *stagingImage, testParams.stagingVersion, testParams.platform)
		if err != nil {
			return fmt.Errorf("failed pushing image: %v", err)
		}
		defer func() {
			if *teardownCluster {
				err := deleteImage(*stagingImage, testParams.stagingVersion)
				if err != nil {
					klog.Errorf("failed to delete image: %v", err)
				}
			}
		}()
	}

	// Create temporary directories for kubernetes builds
	k8sParentDir := generateUniqueTmpDir()
	k8sDir := filepath.Join(k8sParentDir, "kubernetes")
	testParams.testParentDir = generateUniqueTmpDir()
	testParams.testDir = filepath.Join(testParams.testParentDir, "kubernetes")
	defer removeDir(k8sParentDir)
	defer removeDir(testParams.testParentDir)

	// If kube version is set, then download and build Kubernetes for cluster creation
	// Otherwise, either GKE or a prebuild local K8s dir is being used
	if len(*kubeVersion) != 0 {
		err := downloadKubernetesSource(testParams.pkgDir, k8sParentDir, *kubeVersion)
		if err != nil {
			return fmt.Errorf("failed to download Kubernetes source: %v", err)
		}
		err = buildKubernetes(k8sDir, "quick-release")
		if err != nil {
			return fmt.Errorf("failed to build Kubernetes: %v", err)
		}
	} else {
		k8sDir = *localK8sDir
	}

	// If test version is set, then download and build Kubernetes to run K8s tests
	// Otherwise, either kube version is set (which implies GCE) or a local K8s dir is being used
	if len(*testVersion) != 0 && *testVersion != *kubeVersion {
		err := downloadKubernetesSource(testParams.pkgDir, testParams.testParentDir, *testVersion)
		if err != nil {
			return fmt.Errorf("failed to download Kubernetes source: %v", err)
		}
		err = buildKubernetes(testParams.testDir, "WHAT=test/e2e/e2e.test")
		if err != nil {
			return fmt.Errorf("failed to build Kubernetes e2e: %v", err)
		}
		// kubetest relies on ginkgo and kubectl already built in the test k8s directory
		err = buildKubernetes(testParams.testDir, "ginkgo")
		if err != nil {
			return fmt.Errorf("failed to build gingko: %v", err)
		}
		err = buildKubernetes(testParams.testDir, "kubectl")
		if err != nil {
			return fmt.Errorf("failed to build kubectl: %v", err)
		}
	} else {
		testParams.testDir = k8sDir
	}

	if *deploymentStrat == "gke" {
		gkeRegional := isRegionalGKECluster(*gceZone, *gceRegion)
		if *isRegionalCluster && !gkeRegional {
			return fmt.Errorf("--is-regional-cluster set but deployed GKE cluster would be zonal")
		}
		*isRegionalCluster = gkeRegional
	}

	// Create a cluster either through GKE or GCE
	if *bringupCluster {
		var err error = nil
		switch *deploymentStrat {
		case "gce":
			err = clusterUpGCE(k8sDir, *gceZone, *numNodes, testParams.imageType)
		case "gke":
			err = clusterUpGKE(*gceZone, *gceRegion, *numNodes, testParams.imageType, testParams.useGKEManagedDriver)
		default:
			err = fmt.Errorf("deployment-strategy must be set to 'gce' or 'gke', but is: %s", testParams.deploymentStrategy)
		}
		if err != nil {
			return fmt.Errorf("failed to cluster up: %v", err)
		}
	}

	// Defer the tear down of the cluster through GKE or GCE
	if *teardownCluster {
		defer func() {
			switch testParams.deploymentStrategy {
			case "gce":
				err := clusterDownGCE(k8sDir)
				if err != nil {
					klog.Errorf("failed to cluster down: %v", err)
				}
			case "gke":
				err := clusterDownGKE(*gceZone, *gceRegion)
				if err != nil {
					klog.Errorf("failed to cluster down: %v", err)
				}
			default:
				klog.Errorf("deployment-strategy must be set to 'gce' or 'gke', but is: %s", testParams.deploymentStrategy)
			}
		}()
	}

	// For windows cluster, when cluster is up, all Windows nodes are tainted with NoSchedule to avoid linux pods
	// being scheduled to Windows nodes. When running windows tests, we need to remove the taint.
	if testParams.platform == "windows" {
		nodesCmd := exec.Command("kubectl", "get", "nodes", "-l", "kubernetes.io/os=windows", "-o", "name")
		out, err := nodesCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to get windows nodes: %v", err)
		}
		nodes := strings.Fields(string(out))
		for _, node := range nodes {
			taintCmd := exec.Command("kubectl", "taint", "node", node, "node.kubernetes.io/os:NoSchedule-")
			out, err := taintCmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("failed to untaint windows node %v", err)
			}
			klog.Infof("untaint windows nodes: %s, output %s", node, string(out))
		}

		// It typically takes 5+ minutes to download Windows container image. To avoid tests being timed out,
		// pre-pulling the test images as best effort.
		klog.Infof("Prepulling test images.")
		err = os.Setenv("PREPULL_YAML", filepath.Join(testParams.pkgDir, "test", "k8s-integration", "prepull.yaml"))
		if err != nil {
			return err
		}
		out, err = exec.Command(filepath.Join(testParams.pkgDir, "test", "k8s-integration", "prepull-image.sh")).CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to prepull images: %s, err: %v", out, err)
		}
		out, err = exec.Command("kubectl", "describe", "pods", "-n", getDriverNamespace()).CombinedOutput()
		klog.Infof("describe pods \n %s", string(out))

		if err != nil {
			return fmt.Errorf("failed to describe pods: %v", err)
		}

	}

	if !testParams.useGKEManagedDriver {
		// Install the driver and defer its teardown
		err := installDriver(testParams, *stagingImage, *deployOverlayName, *doDriverBuild)
		if *teardownDriver {
			defer func() {
				if teardownErr := deleteDriver(testParams, *deployOverlayName); teardownErr != nil {
					klog.Errorf("failed to delete driver: %v", teardownErr)
				}
			}()
		}
		if err != nil {
			return fmt.Errorf("failed to install CSI Driver: %v", err)
		}
	}

	// Dump all driver logs to the test artifacts
	cancel, err := dumpDriverLogs()
	if err != nil {
		return fmt.Errorf("failed to start driver logging: %v", err)
	}
	defer func() {
		if cancel != nil {
			cancel()
		}
	}()

	// For windows cluster, it has both Windows nodes and Linux nodes. Before triggering the tests, taint Linux nodes
	// with NoSchedule to avoid test pods being scheduled on Linux. Need to do this step after driver is deployed.
	// Also the test framework will not proceed to run tests unless all nodes are ready
	// AND schedulable. Allow not-ready nodes since we make Linux nodes
	// unschedulable.
	testParams.allowedNotReadyNodes = 0
	if *platform == "windows" {
		nodesCmd := exec.Command("kubectl", "get", "nodes", "-l", "kubernetes.io/os=linux", "-o", "name")
		out, err := nodesCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to get linux nodes: %v", err)
		}
		nodes := strings.Fields(string(out))
		testParams.allowedNotReadyNodes = len(nodes)
		for _, node := range nodes {
			taintCmd := exec.Command("kubectl", "taint", "node", node, "node.kubernetes.io/os:NoSchedule")
			out, err := taintCmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("failed to untaint windows node %v", err)
			}
			klog.Infof("taint linux nodes: %s, output %s", node, string(out))
		}
	}

	switch testParams.deploymentStrategy {
	case "gke":
		testParams.cloudProviderArgs, err = getGKEKubeTestArgs(*gceZone, *gceRegion, testParams.imageType)
		if err != nil {
			return fmt.Errorf("failed to build GKE kubetest args: %v", err)
		}
	}

	// Kubernetes version of GKE deployments are expected to be of the pattern x.y.z-gke.k,
	// hence we use the main.Version utils to parse and compare GKE managed cluster versions.
	// For clusters deployed on GCE, use the apimachinery version utils (which supports non-gke based semantic versioning).
	testParams.clusterVersion = mustGetKubeClusterVersion()
	klog.Infof("kubernetes cluster server version: %s", testParams.clusterVersion)
	switch testParams.deploymentStrategy {
	case "gce":
		testParams.testSkip = generateGCETestSkip(testParams)
	case "gke":
		testParams.nodeVersion = *gkeNodeVersion
		testParams.testSkip = generateGKETestSkip(testParams)

	default:
		return fmt.Errorf("Unknown deployment strategy %s", testParams.deploymentStrategy)
	}

	// Run the tests using the testDir kubernetes
	if len(*storageClassFiles) != 0 {
		applicableStorageClassFiles := []string{}
		for _, rawScFile := range strings.Split(*storageClassFiles, ",") {
			scFile := strings.TrimSpace(rawScFile)
			if len(scFile) == 0 {
				continue
			}
			if scFile == regionalPDStorageClass && !*isRegionalCluster {
				klog.Warningf("Skipping regional StorageClass in zonal cluster")
				continue
			}
			applicableStorageClassFiles = append(applicableStorageClassFiles, scFile)
		}
		if len(applicableStorageClassFiles) == 0 {
			return fmt.Errorf("No applicable storage classes found")
		}
		var ginkgoErrors []string
		var testOutputDirs []string
		for _, scFile := range applicableStorageClassFiles {
			outputDir := strings.TrimSuffix(scFile, ".yaml")
			testOutputDirs = append(testOutputDirs, outputDir)
			if err = runCSITests(testParams, scFile, outputDir); err != nil {
				ginkgoErrors = append(ginkgoErrors, err.Error())
			}
		}
		if err = mergeArtifacts(testOutputDirs); err != nil {
			return fmt.Errorf("artifact merging failed: %w", err)
		}
		if ginkgoErrors != nil {
			return fmt.Errorf("runCSITests failed: %v", strings.Join(ginkgoErrors, " "))
		}
	} else if *migrationTest {
		err = runMigrationTests(testParams)
	} else {
		return fmt.Errorf("did not run either CSI or Migration test")
	}

	if err != nil {
		return fmt.Errorf("failed to run tests: %w", err)
	}

	return nil
}

func generateGCETestSkip(testParams *testParameters) string {
	skipString := "\\[Disruptive\\]|\\[Serial\\]"
	v := apimachineryversion.MustParseSemantic(testParams.clusterVersion)

	// "volumeMode should not mount / map unused volumes in a pod" tests a
	// (https://github.com/kubernetes/kubernetes/pull/81163)
	// bug-fix introduced in 1.16
	if v.LessThan(apimachineryversion.MustParseSemantic("1.16.0")) {
		skipString = skipString + "|volumeMode\\sshould\\snot\\smount\\s/\\smap\\sunused\\svolumes\\sin\\sa\\spod"
	}
	if v.LessThan(apimachineryversion.MustParseSemantic("1.17.0")) {
		skipString = skipString + "|VolumeSnapshotDataSource"
	}
	if v.LessThan(apimachineryversion.MustParseSemantic("1.20.0")) {
		skipString = skipString + "|fsgroupchangepolicy"
	}
	if testParams.platform == "windows" {
		skipString = skipString + "|\\[LinuxOnly\\]"
	}
	return skipString
}

func generateGKETestSkip(testParams *testParameters) string {
	skipString := "\\[Disruptive\\]|\\[Serial\\]"
	curVer := mustParseVersion(testParams.clusterVersion)
	var nodeVer *version
	if testParams.nodeVersion != "" {
		nodeVer = mustParseVersion(testParams.nodeVersion)
	}

	// "volumeMode should not mount / map unused volumes in a pod" tests a
	// (https://github.com/kubernetes/kubernetes/pull/81163)
	// bug-fix introduced in 1.16
	if curVer.lessThan(mustParseVersion("1.16.0")) {
		skipString = skipString + "|volumeMode\\sshould\\snot\\smount\\s/\\smap\\sunused\\svolumes\\sin\\sa\\spod"
	}

	// Check master and node version to skip Pod FsgroupChangePolicy test suite.
	if curVer.lessThan(mustParseVersion("1.20.0")) || (nodeVer != nil && nodeVer.lessThan(mustParseVersion("1.20.0"))) {
		skipString = skipString + "|fsgroupchangepolicy"
	}

	// For GKE deployed PD CSI driver, resizer sidecar is enabled in 1.16.8-gke.3
	if (testParams.useGKEManagedDriver && curVer.lessThan(mustParseVersion("1.16.8-gke.3"))) ||
		(!testParams.useGKEManagedDriver && curVer.lessThan(mustParseVersion("1.16.0"))) {
		skipString = skipString + "|allowExpansion"
	}

	// For GKE deployed PD CSI snapshot is enabled in 1.17.6-gke.4(and higher), 1.18.3-gke.0(and higher).
	if (testParams.useGKEManagedDriver && curVer.lessThan(mustParseVersion("1.17.6-gke.4"))) ||
		(!testParams.useGKEManagedDriver && (*curVer).lessThan(mustParseVersion("1.17.0"))) {
		skipString = skipString + "|VolumeSnapshotDataSource"
	}

	return skipString
}

func setEnvProject(project string) error {
	out, err := exec.Command("gcloud", "config", "set", "project", project).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to set gcloud project to %s: %s, err: %v", project, out, err)
	}

	err = os.Setenv("PROJECT", project)
	if err != nil {
		return err
	}
	return nil
}

func runMigrationTests(testParams *testParameters) error {
	return runTestsWithConfig(testParams, "--storage.migratedPlugins=kubernetes.io/gce-pd", "")
}

func runCSITests(testParams *testParameters, storageClassFile string, reportPrefix string) error {
	testDriverConfigFile, err := generateDriverConfigFile(testParams, storageClassFile)
	if err != nil {
		return err
	}
	testConfigArg := fmt.Sprintf("--storage.testdriver=%s", testDriverConfigFile)
	return runTestsWithConfig(testParams, testConfigArg, reportPrefix)
}

func runTestsWithConfig(testParams *testParameters, testConfigArg, reportPrefix string) error {
	err := os.Chdir(testParams.testDir)
	if err != nil {
		return err
	}

	kubeconfig, err := getKubeConfig()
	if err != nil {
		return err
	}
	os.Setenv("KUBECONFIG", kubeconfig)

	artifactsDir, ok := os.LookupEnv("ARTIFACTS")
	reportArg := ""
	kubetestDumpDir := ""
	if ok {
		if len(reportPrefix) > 0 {
			kubetestDumpDir = filepath.Join(artifactsDir, reportPrefix)
			if err := os.MkdirAll(kubetestDumpDir, 0755); err != nil {
				return err
			}
		} else {
			kubetestDumpDir = artifactsDir
		}
	}
	ginkgoArgs := fmt.Sprintf("--ginkgo.focus=%s --ginkgo.skip=%s", testParams.testFocus, testParams.testSkip)
	if testParams.platform == "windows" {
		ginkgoArgs = ginkgoArgs + fmt.Sprintf(" --node-os-distro=%s --allowed-not-ready-nodes=%d", testParams.platform, testParams.allowedNotReadyNodes)
	}
	testArgs := fmt.Sprintf("%s %s %s",
		ginkgoArgs,
		testConfigArg,
		reportArg)

	kubeTestArgs := []string{
		"--test",
		"--ginkgo-parallel",
		"--check-version-skew=false",
		fmt.Sprintf("--test_args=%s", testArgs),
	}
	if kubetestDumpDir != "" {
		kubeTestArgs = append(kubeTestArgs, fmt.Sprintf("--dump=%s", kubetestDumpDir))
	}
	kubeTestArgs = append(kubeTestArgs, testParams.cloudProviderArgs...)

	err = runCommand("Running Tests", exec.Command("kubetest", kubeTestArgs...))
	if err != nil {
		return fmt.Errorf("failed to run tests on e2e cluster: %v", err)
	}

	return nil
}
