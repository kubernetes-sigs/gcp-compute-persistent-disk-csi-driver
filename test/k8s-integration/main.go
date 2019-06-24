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
	"syscall"
	"time"

	"github.com/golang/glog"

	testutils "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/test/e2e/utils"

	"k8s.io/apimachinery/pkg/util/uuid"
)

var (
	// Kubernetes cluster flags
	teardownCluster  = flag.Bool("teardown-cluster", true, "teardown the cluster after the e2e test")
	teardownDriver   = flag.Bool("teardown-driver", true, "teardown the driver after the e2e test")
	bringupCluster   = flag.Bool("bringup-cluster", true, "build kubernetes and bringup a cluster")
	gceZone          = flag.String("gce-zone", "", "zone that the gce k8s cluster is created/found in")
	kubeVersion      = flag.String("kube-version", "master", "version of Kubernetes to download and use")
	kubeFeatureGates = flag.String("kube-feature-gates", "", "feature gates to set on new kubernetes cluster")
	localK8sDir      = flag.String("local-k8s-dir", "", "local kubernetes/kubernetes directory to run e2e tests from")
	deploymentStrat  = flag.String("deployment-strategy", "gce", "choose between deploying on gce or gke")
	gkeClusterVer    = flag.String("gke-cluster-version", "latest", "version of Kubernetes master and node for gke")

	// Test infrastructure flags
	boskosResourceType = flag.String("boskos-resource-type", "gce-project", "name of the boskos resource type to reserve")
	storageClassFile   = flag.String("storageclass-file", "", "name of storageclass yaml file to use for test relative to test/k8s-integration/config")
	inProw             = flag.Bool("run-in-prow", false, "is the test running in PROW")

	// Driver flags
	stagingImage      = flag.String("staging-image", "", "name of image to stage to")
	saFile            = flag.String("service-account-file", "", "path of service account file")
	deployOverlayName = flag.String("deploy-overlay-name", "", "which kustomize overlay to deploy the driver with")
	doDriverBuild     = flag.Bool("do-driver-build", true, "building the driver from source")

	// Test flags
	migrationTest = flag.Bool("migration-test", false, "sets the flag on the e2e binary signalling migration")
	testFocus     = flag.String("test-focus", "", "test focus for Kubernetes e2e")
)

const (
	pdImagePlaceholder = "gke.gcr.io/gcp-compute-persistent-disk-csi-driver"
	k8sBuildBinDir     = "_output/dockerized/bin/linux/amd64"
	gkeTestClusterName = "gcp-pd-csi-driver-test-cluster"
)

func init() {
	flag.Set("logtostderr", "true")
}
func main() {
	flag.Parse()

	if len(*stagingImage) == 0 && !*inProw {
		glog.Fatalf("staging-image is a required flag, please specify the name of image to stage to")
	}

	if len(*saFile) == 0 {
		glog.Fatalf("service-account-file is a required flag")
	}

	if len(*deployOverlayName) == 0 {
		glog.Fatalf("deploy-overlay-name is a required flag")
	}

	if len(*storageClassFile) == 0 && !*migrationTest {
		glog.Fatalf("One of storageclass-file and migration-test must be set")
	}

	if len(*storageClassFile) != 0 && *migrationTest {
		glog.Fatalf("storage-class-file and migration-test cannot both be set")
	}

	if !*bringupCluster && len(*kubeFeatureGates) > 0 {
		glog.Fatalf("kube-feature-gates set but not bringing up new cluster")
	}

	if len(*testFocus) == 0 {
		glog.Fatalf("test-focus is a required flag")
	}

	if len(*gceZone) == 0 {
		glog.Fatalf("gce-zone is a required flag")
	}

	if *deploymentStrat == "gke" && *migrationTest {
		glog.Fatalf("Cannot set deployment strategy to 'gke' for migration tests.")
	}

	err := handle()
	if err != nil {
		glog.Fatalf("Failed to run integration test: %v", err)
	}
}

func handle() error {
	oldmask := syscall.Umask(0000)
	defer syscall.Umask(oldmask)

	stagingVersion := string(uuid.NewUUID())

	goPath, ok := os.LookupEnv("GOPATH")
	if !ok {
		return fmt.Errorf("Could not find env variable GOPATH")
	}
	pkgDir := filepath.Join(goPath, "src", "sigs.k8s.io", "gcp-compute-persistent-disk-csi-driver")
	k8sIoDir := filepath.Join(pkgDir, "test", "k8s-integration", "src", "k8s.io")
	k8sDir := filepath.Join(k8sIoDir, "kubernetes")

	if *inProw {
		project, _ := testutils.SetupProwConfig(*boskosResourceType)

		oldProject, err := exec.Command("gcloud", "config", "get-value", "project").CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to get gcloud project: %s, err: %v", oldProject, err)
		}

		err = setEnvProject(project)
		if err != nil {
			return fmt.Errorf("failed to set project environment to %s: %v", project, err)
		}
		defer func() {
			err = setEnvProject(string(oldProject))
			if err != nil {
				glog.Errorf("failed to set project environment to %s: %v", oldProject, err)
			}
		}()

		if *doDriverBuild {
			*stagingImage = fmt.Sprintf("gcr.io/%s/gcp-persistent-disk-csi-driver", project)
		}

		if _, ok := os.LookupEnv("USER"); !ok {
			err = os.Setenv("USER", "prow")
			if err != nil {
				return fmt.Errorf("failed to set user in prow to prow: %v", err)
			}
		}
	}

	if *doDriverBuild {
		err := pushImage(pkgDir, *stagingImage, stagingVersion)
		if err != nil {
			return fmt.Errorf("failed pushing image: %v", err)
		}
		defer func() {
			if *teardownCluster {
				err = deleteImage(*stagingImage, stagingVersion)
				if err != nil {
					glog.Errorf("failed to delete image: %v", err)
				}
			}
		}()
	}

	if *bringupCluster {
		err := downloadKubernetesSource(pkgDir, k8sIoDir, *kubeVersion)
		if err != nil {
			return fmt.Errorf("failed to download Kubernetes source: %v", err)
		}

		err = buildKubernetes(k8sDir)
		if err != nil {
			return fmt.Errorf("failed to build Kubernetes: %v", err)
		}

		kshPath := filepath.Join(k8sDir, "cluster", "kubectl.sh")
		_, err = os.Stat(kshPath)
		if err == nil {
			// Set kubectl to the one bundled in the k8s tar for versioning
			err = os.Setenv("GCE_PD_KUBECTL", kshPath)
			if err != nil {
				return fmt.Errorf("failed to set cluster specific kubectl: %v", err)
			}
		} else {
			glog.Errorf("could not find cluster kubectl at %s, falling back to default kubectl", kshPath)
		}

		if len(*kubeFeatureGates) != 0 {
			err = os.Setenv("KUBE_FEATURE_GATES", *kubeFeatureGates)
			if err != nil {
				return fmt.Errorf("failed to set kubernetes feature gates: %v", err)
			}
			glog.V(4).Infof("Set Kubernetes feature gates: %v", *kubeFeatureGates)
		}

		switch *deploymentStrat {
		case "gce":
			err = clusterUpGCE(k8sDir, *gceZone)
			if err != nil {
				return fmt.Errorf("failed to cluster up: %v", err)
			}
		case "gke":
			err = clusterUpGKE(*gceZone)
			if err != nil {
				return fmt.Errorf("failed to cluster up: %v", err)
			}
		default:
			return fmt.Errorf("deployment-strategy must be set to 'gce' or 'gke', but is: %s", *deploymentStrat)
		}

	}

	if *teardownCluster {
		defer func() {
			switch *deploymentStrat {
			case "gce":
				err := clusterDownGCE(k8sDir)
				if err != nil {
					glog.Errorf("failed to cluster down: %v", err)
				}
			case "gke":
				err := clusterDownGKE(*gceZone)
				if err != nil {
					glog.Errorf("failed to cluster down: %v", err)
				}
			default:
				glog.Errorf("deployment-strategy must be set to 'gce' or 'gke', but is: %s", *deploymentStrat)
			}
		}()
	}

	err := installDriver(goPath, pkgDir, k8sDir, *stagingImage, stagingVersion, *deployOverlayName, *doDriverBuild)
	if *teardownDriver {
		defer func() {
			// TODO (#140): collect driver logs
			if teardownErr := deleteDriver(goPath, pkgDir, *deployOverlayName); teardownErr != nil {
				glog.Errorf("failed to delete driver: %v", teardownErr)
			}
		}()
	}
	if err != nil {
		return fmt.Errorf("failed to install CSI Driver: %v", err)
	}

	if len(*localK8sDir) != 0 {
		k8sDir = *localK8sDir
	}

	if len(*storageClassFile) != 0 {
		err = runCSITests(pkgDir, k8sDir, *testFocus, *storageClassFile, *gceZone)
	} else if *migrationTest {
		err = runMigrationTests(pkgDir, k8sDir, *testFocus, *gceZone)
	} else {
		return fmt.Errorf("Did not run either CSI or Migration test")
	}

	if err != nil {
		return fmt.Errorf("failed to run tests: %v", err)
	}

	return nil
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

func runMigrationTests(pkgDir, k8sDir, testFocus, gceZone string) error {
	return runTestsWithConfig(pkgDir, k8sDir, gceZone, testFocus, "-storage.migratedPlugins=kubernetes.io/gce-pd")
}

func runCSITests(pkgDir, k8sDir, testFocus, storageClassFile, gceZone string) error {
	testDriverConfigFile, err := generateDriverConfigFile(pkgDir, storageClassFile)
	if err != nil {
		return err
	}
	testConfigArg := fmt.Sprintf("-storage.testdriver=%s", testDriverConfigFile)
	return runTestsWithConfig(pkgDir, k8sDir, gceZone, testFocus, testConfigArg)
}

func runTestsWithConfig(pkgDir, k8sDir, gceZone, testFocus, testConfigArg string) error {
	err := os.Chdir(k8sDir)
	if err != nil {
		return err
	}

	homeDir, _ := os.LookupEnv("HOME")
	os.Setenv("KUBECONFIG", filepath.Join(homeDir, ".kube/config"))

	artifactsDir, _ := os.LookupEnv("ARTIFACTS")
	reportArg := fmt.Sprintf("-report-dir=%s", artifactsDir)

	testFocusArg := fmt.Sprintf("-focus=%s", testFocus)

	cmd := exec.Command(filepath.Join(k8sBuildBinDir, "ginkgo"),
		"-p",
		testFocusArg,
		"-skip=\\[Disruptive\\]|\\[Serial\\]|\\[Feature:.+\\]",
		filepath.Join(k8sBuildBinDir, "e2e.test"),
		"--",
		reportArg,
		"-provider=gce",
		fmt.Sprintf("-gce-zone=%s", gceZone),
		testConfigArg)

	err = runCommand("Running Tests", cmd)
	if err != nil {
		return fmt.Errorf("failed to run tests on e2e cluster: %v", err)
	}

	return nil
}

func runCommand(action string, cmd *exec.Cmd) error {
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr

	fmt.Printf("%s\n", action)
	fmt.Printf("%s\n", cmd.Args)

	err := cmd.Start()
	if err != nil {
		return err
	}

	err = cmd.Wait()
	if err != nil {
		return err
	}
	return nil
}

func clusterDownGCE(k8sDir string) error {
	cmd := exec.Command(filepath.Join(k8sDir, "hack", "e2e-internal", "e2e-down.sh"))
	err := runCommand("Bringing Down E2E Cluster on GCE", cmd)
	if err != nil {
		return fmt.Errorf("failed to bring down kubernetes e2e cluster on gce: %v", err)
	}
	return nil
}

func clusterDownGKE(gceZone string) error {
	cmd := exec.Command("gcloud", "container", "clusters", "delete", gkeTestClusterName,
		"--zone", gceZone, "--quiet")
	err := runCommand("Bringing Down E2E Cluster on GKE", cmd)
	if err != nil {
		return fmt.Errorf("failed to bring down kubernetes e2e cluster on gke: %v", err)
	}
	return nil
}

func buildKubernetes(k8sDir string) error {
	cmd := exec.Command("make", "-C", k8sDir, "quick-release")
	err := runCommand("Building Kubernetes", cmd)
	if err != nil {
		return fmt.Errorf("failed to build Kubernetes: %v", err)
	}
	return nil
}

func clusterUpGCE(k8sDir, gceZone string) error {
	err := os.Setenv("KUBE_GCE_ZONE", gceZone)
	if err != nil {
		return err
	}
	cmd := exec.Command(filepath.Join(k8sDir, "hack", "e2e-internal", "e2e-up.sh"))
	err = runCommand("Starting E2E Cluster on GCE", cmd)
	if err != nil {
		return fmt.Errorf("failed to bring up kubernetes e2e cluster on gce: %v", err)
	}

	return nil
}

func clusterUpGKE(gceZone string) error {
	out, err := exec.Command("gcloud", "container", "clusters", "list", "--zone", gceZone,
		"--filter", fmt.Sprintf("name=%s", gkeTestClusterName)).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to check for previous test cluster: %v %s", err, out)
	}
	if len(out) > 0 {
		glog.Infof("Detected previous cluster %s. Deleting so a new one can be created...", gkeTestClusterName)
		err = clusterDownGKE(gceZone)
		if err != nil {
			return err
		}
	}
	cmd := exec.Command("gcloud", "container", "clusters", "create", gkeTestClusterName,
		"--zone", gceZone, "--cluster-version", *gkeClusterVer, "--quiet")
	err = runCommand("Staring E2E Cluster on GKE", cmd)
	if err != nil {
		return fmt.Errorf("failed to bring up kubernetes e2e cluster on gke: %v", err)
	}

	return nil
}

func getOverlayDir(pkgDir, deployOverlayName string) string {
	return filepath.Join(pkgDir, "deploy", "kubernetes", "overlays", deployOverlayName)
}

func installDriver(goPath, pkgDir, k8sDir, stagingImage, stagingVersion, deployOverlayName string, doDriverBuild bool) error {
	if doDriverBuild {
		// Install kustomize
		out, err := exec.Command(filepath.Join(pkgDir, "deploy", "kubernetes", "install-kustomize.sh")).CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to install kustomize: %s, err: %v", out, err)
		}

		// Edit ci kustomization to use given image tag
		overlayDir := getOverlayDir(pkgDir, deployOverlayName)
		err = os.Chdir(overlayDir)
		if err != nil {
			return fmt.Errorf("failed to change to overlay directory: %s, err: %v", out, err)
		}

		// TODO (#138): in a local environment this is going to modify the actual kustomize files.
		// maybe a copy should be made instead
		out, err = exec.Command(
			filepath.Join(pkgDir, "bin", "kustomize"),
			"edit",
			"set",
			"image",
			fmt.Sprintf("%s=%s:%s", pdImagePlaceholder, stagingImage, stagingVersion)).CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to edit kustomize: %s, err: %v", out, err)
		}
	}

	// setup service account file for secret creation
	tmpSaFile := fmt.Sprintf("/tmp/%s/cloud-sa.json", string(uuid.NewUUID()))

	os.MkdirAll(filepath.Dir(tmpSaFile), 0750)
	defer os.Remove(filepath.Dir(tmpSaFile))

	// Need to copy it to name the file "cloud-sa.json"
	out, err := exec.Command("cp", *saFile, tmpSaFile).CombinedOutput()
	if err != nil {
		return fmt.Errorf("error copying service account key: %s, err: %v", out, err)
	}
	defer shredFile(tmpSaFile)

	// deploy driver
	deployCmd := exec.Command(filepath.Join(pkgDir, "deploy", "kubernetes", "deploy-driver.sh"), "--skip-sa-check")
	deployCmd.Env = append(os.Environ(),
		fmt.Sprintf("GOPATH=%s", goPath),
		fmt.Sprintf("GCE_PD_SA_DIR=%s", filepath.Dir(tmpSaFile)),
		fmt.Sprintf("GCE_PD_DRIVER_VERSION=%s", deployOverlayName),
	)
	err = runCommand("Deploying driver", deployCmd)
	if err != nil {
		return fmt.Errorf("failed to deploy driver: %v", err)
	}

	// TODO (#139): wait for driver to be running
	time.Sleep(10 * time.Second)
	statusCmd := exec.Command("kubectl", "describe", "pods", "-n", "default")
	err = runCommand("Checking driver pods", statusCmd)
	if err != nil {
		return fmt.Errorf("failed to check driver pods: %v", err)
	}

	return nil
}

func deleteDriver(goPath, pkgDir, deployOverlayName string) error {
	deleteCmd := exec.Command(filepath.Join(pkgDir, "deploy", "kubernetes", "delete-driver.sh"))
	deleteCmd.Env = append(os.Environ(),
		fmt.Sprintf("GOPATH=%s", goPath),
		fmt.Sprintf("GCE_PD_DRIVER_VERSION=%s", deployOverlayName),
	)
	err := runCommand("Deleting driver", deleteCmd)
	if err != nil {
		return fmt.Errorf("failed to delete driver: %v", err)
	}
	return nil
}

func shredFile(filePath string) {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		glog.V(4).Infof("File %v was not found, skipping shredding", filePath)
		return
	}
	glog.V(4).Infof("Shredding file %v", filePath)
	out, err := exec.Command("shred", "--remove", filePath).CombinedOutput()
	if err != nil {
		glog.V(4).Infof("Failed to shred file %v: %v\nOutput:%v", filePath, err, out)
	}
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		glog.V(4).Infof("File %v successfully shredded", filePath)
		return
	}

	// Shred failed Try to remove the file for good meausure
	err = os.Remove(filePath)
	if err != nil {
		glog.V(4).Infof("Failed to remove service account file %s: %v", filePath, err)
	}
}

func downloadKubernetesSource(pkgDir, k8sIoDir, kubeVersion string) error {
	k8sDir := filepath.Join(k8sIoDir, "kubernetes")
	/*
		// TODO: Download a fresh copy every time until mutate manifests hardcoding existing image is solved.
		if _, err := os.Stat(k8sDir); !os.IsNotExist(err) {
			glog.Infof("Staging Kubernetes already found at %s, skipping download", k8sDir)
			return nil
		}
	*/

	glog.V(4).Infof("Staging Kubernetes folder not found, downloading now")

	err := os.MkdirAll(k8sIoDir, 0777)
	if err != nil {
		return err
	}

	kubeTarDir := filepath.Join(k8sIoDir, fmt.Sprintf("kubernetes-%s.tar.gz", kubeVersion))

	var vKubeVersion string
	if kubeVersion == "master" {
		vKubeVersion = kubeVersion
		// A hack to be able to build Kubernetes in this nested place
		// KUBE_GIT_VERSION_FILE set to file to load kube version from
		err = os.Setenv("KUBE_GIT_VERSION_FILE", filepath.Join(pkgDir, "test", "k8s-integration", ".dockerized-kube-version-defs"))
		if err != nil {
			return err
		}
	} else {
		vKubeVersion = "v" + kubeVersion
	}
	out, err := exec.Command("curl", "-L", fmt.Sprintf("https://github.com/kubernetes/kubernetes/archive/%s.tar.gz", vKubeVersion), "-o", kubeTarDir).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to curl kubernetes version %s: %s, err: %v", kubeVersion, out, err)
	}

	out, err = exec.Command("tar", "-C", k8sIoDir, "-xvf", kubeTarDir).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to untar %s: %s, err: %v", kubeTarDir, out, err)
	}

	err = os.RemoveAll(k8sDir)
	if err != nil {
		return err
	}

	err = os.Rename(filepath.Join(k8sIoDir, fmt.Sprintf("kubernetes-%s", kubeVersion)), k8sDir)
	if err != nil {
		return err
	}

	glog.V(4).Infof("Successfully downloaded Kubernetes v%s to %s", kubeVersion, k8sDir)

	return nil
}

func pushImage(pkgDir, stagingImage, stagingVersion string) error {
	err := os.Setenv("GCE_PD_CSI_STAGING_VERSION", stagingVersion)
	if err != nil {
		return err
	}
	err = os.Setenv("GCE_PD_CSI_STAGING_IMAGE", stagingImage)
	if err != nil {
		return err
	}
	cmd := exec.Command("make", "-C", pkgDir, "push-container",
		fmt.Sprintf("GCE_PD_CSI_STAGING_VERSION=%s", stagingVersion),
		fmt.Sprintf("GCE_PD_CSI_STAGING_IMAGE=%s", stagingImage))
	err = runCommand("Pushing GCP Container", cmd)
	if err != nil {
		return fmt.Errorf("failed to run make command: err: %v", err)
	}
	return nil
}

func deleteImage(stagingImage, stagingVersion string) error {
	cmd := exec.Command("gcloud", "container", "images", "delete", fmt.Sprintf("%s:%s", stagingImage, stagingVersion), "--quiet")
	err := runCommand("Deleting GCR Container", cmd)
	if err != nil {
		return fmt.Errorf("failed to delete container image %s:%s: %s", stagingImage, stagingVersion, err)
	}
	return nil
}
