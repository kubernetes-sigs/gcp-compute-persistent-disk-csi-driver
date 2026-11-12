package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	apimachineryversion "k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

func gkeLocationArgs(gceZone, gceRegion string) (locationArg, locationVal string, err error) {
	switch {
	case len(gceZone) > 0:
		locationArg = "--zone"
		locationVal = gceZone
	case len(gceRegion) > 0:
		locationArg = "--region"
		locationVal = gceRegion
	default:
		return "", "", fmt.Errorf("zone and region unspecified")
	}
	return
}

func isRegionalGKECluster(gceZone, gceRegion string) bool {
	return len(gceRegion) > 0
}

func clusterDownGCE(k8sDir string) error {
	cmd := exec.Command(filepath.Join(k8sDir, "hack", "e2e-internal", "e2e-down.sh"))
	cmd.Env = os.Environ()
	err := runCommand("Bringing Down E2E Cluster on GCE", cmd)
	if err != nil {
		return fmt.Errorf("failed to bring down kubernetes e2e cluster on gce: %v", err)
	}
	return nil
}

func clusterDownGKE(gceZone, gceRegion string) error {
	locationArg, locationVal, err := gkeLocationArgs(gceZone, gceRegion)
	if err != nil {
		return err
	}

	cmd := exec.Command("gcloud", "container", "clusters", "delete", *gkeTestClusterName,
		locationArg, locationVal, "--quiet")
	err = runCommand("Bringing Down E2E Cluster on GKE", cmd)
	if err != nil {
		return fmt.Errorf("failed to bring down kubernetes e2e cluster on gke: %v", err.Error())
	}
	return nil
}

func buildKubernetes(k8sDir, command string) error {
	cmd := exec.Command("make", "-C", k8sDir, command)
	cmd.Env = os.Environ()
	err := runCommand(fmt.Sprintf("Running command in kubernetes/kubernetes path=%s", k8sDir), cmd)
	if err != nil {
		return fmt.Errorf("failed to build Kubernetes: %w", err)
	}
	return nil
}

func clusterUpGCE(k8sDir, gceZone string, numNodes int, numWindowsNodes int, imageType string) error {
	kshPath := filepath.Join(k8sDir, "cluster", "kubectl.sh")
	_, err := os.Stat(kshPath)
	if err == nil {
		// Set kubectl to the one bundled in the k8s tar for versioning
		err = os.Setenv("GCE_PD_KUBECTL", kshPath)
		if err != nil {
			return fmt.Errorf("failed to set cluster specific kubectl: %w", err)
		}
	} else {
		klog.Errorf("could not find cluster kubectl at %s, falling back to default kubectl", kshPath)
	}

	if len(*kubeFeatureGates) != 0 {
		err = os.Setenv("KUBE_FEATURE_GATES", *kubeFeatureGates)
		if err != nil {
			return fmt.Errorf("failed to set kubernetes feature gates: %w", err)
		}
		klog.V(4).Infof("Set Kubernetes feature gates: %v", *kubeFeatureGates)
	}

	if len(*kubeRuntimeConfig) != 0 {
		err = os.Setenv("KUBE_RUNTIME_CONFIG", *kubeRuntimeConfig)
		if err != nil {
			return fmt.Errorf("failed to set kubernetes runtime config: %w", err)
		}
		klog.V(4).Infof("Set Kubernetes runtime config: %v", *kubeRuntimeConfig)
		// If runtime config is set, the cluster runs the hyperdisk tests, so
		// the nodes need a machine type and boot disk type that support them.
		if err := setGCEHyperdiskNodeEnvs(); err != nil {
			return err
		}
	}

	err = setImageTypeEnvs(imageType)
	if err != nil {
		return fmt.Errorf("failed to set image type environment variables: %w", err)
	}

	err = os.Setenv("NUM_NODES", strconv.Itoa(numNodes))
	if err != nil {
		return err
	}

	// the chain is NUM_WINDOWS_NODES -> --num-windows-nodes -> NUM_WINDOWS_NODES
	// runCommand runs e2e-up.sh inheriting env vars so the `--num-windows-nodes`
	// flags might not be needed, added to be similar to the setup of NUM_NODES
	err = os.Setenv("NUM_WINDOWS_NODES", strconv.Itoa(numWindowsNodes))
	if err != nil {
		return err
	}

	// The default master size with few nodes is too small; the tests must hit the API server
	// more than usual. The main issue seems to be memory, to reduce GC times that stall the
	// api server. For defaults, get-master-size in k/k/cluster/gce/config-common.sh.
	if numNodes < 20 {
		err = os.Setenv("MASTER_SIZE", "n1-standard-4")
		if err != nil {
			return err
		}
	}

	// Cap how long kube-up waits for the node instance group to become stable
	// so that a zonal capacity stockout surfaces with enough of the job's time
	// budget left to retry in another zone. kube-up ignores this timeout
	// expiring and lets validate-cluster keep waiting for nodes to join, so a
	// merely slow bring-up still succeeds.
	if len(os.Getenv("MIG_WAIT_UNTIL_STABLE_TIMEOUT")) == 0 {
		err = os.Setenv("MIG_WAIT_UNTIL_STABLE_TIMEOUT", defaultMIGWaitUntilStableTimeout)
		if err != nil {
			return fmt.Errorf("failed to set MIG_WAIT_UNTIL_STABLE_TIMEOUT: %w", err)
		}
	}

	_, err = bringUpGCEClusterWithZoneFallback(k8sDir, gceZone)
	if err != nil {
		return err
	}

	// In GCE clusters created by kube-up.sh, the master node is tainted with
	// node-role.kubernetes.io/control-plane:NoSchedule but lacks the corresponding
	// label. Apply the standard label so that nodeAffinity rules in the DaemonSet
	// can properly exclude it.
	nodesCmd := exec.Command("kubectl", "get", "nodes", "-o", "jsonpath={.items[*].metadata.name}")
	out, err := nodesCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to get nodes: %s, err: %w", out, err)
	}
	foundMaster := false
	for _, node := range strings.Fields(string(out)) {
		if strings.HasSuffix(node, "-master") {
			foundMaster = true
			labelCmd := exec.Command("kubectl", "label", "node", node, "node-role.kubernetes.io/control-plane=", "--overwrite")
			out, err := labelCmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("failed to label master node %s: %s, err: %w", node, out, err)
			}
			klog.Infof("Labeled master node %s with node-role.kubernetes.io/control-plane", node)
		}
	}
	if !foundMaster {
		klog.Warningf("No master node found with suffix '-master' to label with node-role.kubernetes.io/control-plane")
	}

	return nil
}

var (
	// defaultMIGWaitUntilStableTimeout bounds, in seconds, how long kube-up
	// waits for the node managed instance group to become stable. kube-up's
	// own default is 1800s; on a zonal stockout that wait plus
	// validate-cluster's ~25 minute node wait burns over an hour before the
	// failure surfaces, leaving no room for a retry within the CI job timeout.
	defaultMIGWaitUntilStableTimeout = "600"

	// gceHyperdiskNodeMachineType is the node machine type of GCE kube-up
	// clusters that run the hyperdisk tests. The whole suite runs on these
	// nodes, the pd-balanced storage class and snapshot tests as well as the
	// hyperdisk-balanced VolumeAttributesClass tests, and they boot from
	// gceHyperdiskNodeDiskType, so the machine type has to support both
	// Persistent Disk and Hyperdisk Balanced. That rules out the Hyperdisk-only
	// N4 and C4 families; C3 and C3D are the generally available x86 families
	// that support both. C3D is used rather than C3 because C3 capacity in the
	// CI zones has been the recurring cause of node stockouts.
	gceHyperdiskNodeMachineType = "c3d-standard-4"

	// gceHyperdiskNodeDiskType is the node boot disk type to go with
	// gceHyperdiskNodeMachineType; kube-up's default pd-standard is not
	// supported by C3 or C3D.
	gceHyperdiskNodeDiskType = "pd-ssd"

	// fallbackGCEZones are tried, after the remaining zones of the requested
	// region, when kube-up cannot get node capacity in the requested zone.
	fallbackGCEZones = []string{"us-central1-b", "us-central1-c", "us-central1-a", "us-central1-f", "us-west1-b", "us-west1-c", "us-east1-b", "us-east1-c"}

	// Indirections so that the zone fallback logic can be unit tested.
	runGCEClusterUp           = runGCEClusterUpScript
	runGCEClusterDown         = clusterDownGCE
	gceMachineTypeInZone      = gceMachineTypeAvailableInZone
	logGCEInstanceGroupErrors = logGCEManagedInstanceGroupErrors
)

// setGCEHyperdiskNodeEnvs points kube-up at a node machine type and boot disk
// type that support the hyperdisk tests. Values already set in the environment
// are kept, so that a job can try another machine type without a code change.
func setGCEHyperdiskNodeEnvs() error {
	defaults := []struct{ name, value string }{
		{"NODE_SIZE", gceHyperdiskNodeMachineType},
		{"NODE_DISK_TYPE", gceHyperdiskNodeDiskType},
	}
	for _, d := range defaults {
		if cur := os.Getenv(d.name); len(cur) != 0 {
			klog.Infof("Keeping %s=%s from the environment instead of the default %s", d.name, cur, d.value)
			continue
		}
		if err := os.Setenv(d.name, d.value); err != nil {
			return fmt.Errorf("failed to set %s: %w", d.name, err)
		}
		klog.Infof("Set %s=%s for the hyperdisk tests", d.name, d.value)
	}
	return nil
}

// candidateGCEZones returns the zones to try for a GCE kube-up cluster: the
// requested zone first, then the other zones of its region, then
// fallbackGCEZones. Zones that do not exist or lack the node machine type are
// skipped at attempt time by gceMachineTypeInZone.
func candidateGCEZones(primaryZone string) []string {
	zones := []string{primaryZone}
	seen := map[string]bool{primaryZone: true}
	add := func(zone string) {
		if !seen[zone] {
			seen[zone] = true
			zones = append(zones, zone)
		}
	}
	if idx := strings.LastIndex(primaryZone, "-"); idx > 0 {
		region := primaryZone[:idx]
		for _, suffix := range []string{"a", "b", "c", "d", "f"} {
			add(region + "-" + suffix)
		}
	}
	for _, zone := range fallbackGCEZones {
		add(zone)
	}
	return zones
}

// isGCEStockoutOutput reports whether kube-up output shows that GCE could not
// provide the node VMs, as opposed to a cluster configuration or Kubernetes
// failure that another zone would not fix.
func isGCEStockoutOutput(output string) bool {
	stockoutPatterns := []string{
		// cluster/gce/util.sh gave up waiting for the node MIG to finish
		// creating its instances.
		"Timeout while waiting for group to become stable",
		"RESOURCE_POOL_EXHAUSTED",
		"does not have enough resources available",
		"GCE_STOCKOUT",
	}
	for _, pattern := range stockoutPatterns {
		if strings.Contains(output, pattern) {
			return true
		}
	}
	return false
}

func runGCEClusterUpScript(k8sDir string) (string, error) {
	cmd := exec.Command(filepath.Join(k8sDir, "hack", "e2e-internal", "e2e-up.sh"))
	cmd.Env = os.Environ()
	return runCommandWithOutput("Starting E2E Cluster on GCE", cmd)
}

func gceMachineTypeAvailableInZone(machineType, zone string) bool {
	err := exec.Command("gcloud", "compute", "machine-types", "describe", machineType, "--zone", zone, "--verbosity", "none").Run()
	return err == nil
}

// logGCEManagedInstanceGroupErrors logs the errors recorded by the managed
// instance groups in zone, which state why instance creation failed (for
// example ZONE_RESOURCE_POOL_EXHAUSTED). kube-up does not surface them.
// Diagnostics only; failures are logged and ignored.
func logGCEManagedInstanceGroupErrors(zone string) {
	out, err := exec.Command("gcloud", "compute", "instance-groups", "managed", "list",
		"--filter", fmt.Sprintf("zone:(%s)", zone), "--format", "value(name)").CombinedOutput()
	if err != nil {
		klog.Warningf("Failed to list managed instance groups in zone %s: %v %s", zone, err, out)
		return
	}
	for _, mig := range strings.Fields(string(out)) {
		out, err := exec.Command("gcloud", "compute", "instance-groups", "managed", "list-errors", mig,
			"--zone", zone, "--format", "value(error.code,error.message)").CombinedOutput()
		if err != nil {
			klog.Warningf("Failed to list errors of managed instance group %s in zone %s: %v %s", mig, zone, err, out)
			continue
		}
		klog.Infof("Errors reported by managed instance group %s in zone %s:\n%s", mig, zone, out)
	}
}

// bringUpGCEClusterWithZoneFallback runs kube-up in primaryZone and, when that
// fails because GCE has no node capacity there, tears the partial cluster down
// and retries in the other candidate zones. It returns the zone the cluster
// came up in and leaves KUBE_GCE_ZONE set to it so that kube-down tears down
// the right cluster.
func bringUpGCEClusterWithZoneFallback(k8sDir, primaryZone string) (string, error) {
	nodeMachineType := os.Getenv("NODE_SIZE")
	if len(nodeMachineType) == 0 {
		// kube-up's default, see cluster/gce/config-test.sh.
		nodeMachineType = "e2-standard-2"
	}

	zones := candidateGCEZones(primaryZone)
	var lastErr error
	for i, zone := range zones {
		if i > 0 && !gceMachineTypeInZone(nodeMachineType, zone) {
			klog.Infof("Skipping fallback zone %s: machine type %s is not available there", zone, nodeMachineType)
			continue
		}
		if err := os.Setenv("KUBE_GCE_ZONE", zone); err != nil {
			return "", err
		}

		out, err := runGCEClusterUp(k8sDir)
		if err == nil {
			if zone != primaryZone {
				klog.Warningf("Brought up GCE cluster in fallback zone %s instead of %s", zone, primaryZone)
			}
			return zone, nil
		}
		lastErr = fmt.Errorf("failed to bring up kubernetes e2e cluster on gce in zone %s: %w", zone, err)
		if !isGCEStockoutOutput(out) {
			return "", lastErr
		}
		if i == len(zones)-1 {
			break
		}

		klog.Warningf("%v: GCE did not provide node capacity in zone %s, tearing the partial cluster down and retrying in another zone", lastErr, zone)
		logGCEInstanceGroupErrors(zone)
		if err := runGCEClusterDown(k8sDir); err != nil {
			return "", fmt.Errorf("%v; also failed to tear down the partial cluster: %w", lastErr, err)
		}
	}
	return "", lastErr
}

func setImageTypeEnvs(imageType string) error {
	switch strings.ToLower(imageType) {
	case "cos":
	case "cos_containerd":
	case "gci": // GCI/COS is default type and does not need env vars set
	case "win2019", "win2022":
		// These are handled by the WINDOWS_NODE_OS_DISTRIBUTION env var and
		// ingested in https://github.com/kubernetes/kubernetes/blob/ded2956c832502da8a0678f5392c24af1cc9dfc0/cluster/gce/util.sh#L112.
	case "ubuntu", "ubuntu_containerd":
		return errors.New("setting environment vars for bringing up *ubuntu* cluster on GCE is unimplemented")
		/* TODO(dyzz) figure out how to bring up a Ubuntu cluster on GCE. The below doesn't work.
		err := os.Setenv("KUBE_OS_DISTRIBUTION", "ubuntu")
		if err != nil {
			return err
		}
		err = os.Setenv("KUBE_GCE_NODE_IMAGE", image)
		if err != nil {
			return err
		}
		err = os.Setenv("KUBE_GCE_NODE_PROJECT", imageProject)
		if err != nil {
			return err
		}
		*/
	default:
		return fmt.Errorf("could not set env for image type %s, only gci, cos, ubuntu supported", imageType)
	}
	return nil
}

var (
	fallbackGKEZones   = []string{"us-central1-b", "us-central1-c", "us-central1-a", "us-west1-b", "us-west1-c", "us-east1-b", "us-east1-c"}
	fallbackGKERegions = []string{"us-central1", "us-west1", "us-east1"}
)

func candidateGKELocations(z, r string) (locationArg string, locations []string, err error) {
	locationArg, primaryLoc, err := gkeLocationArgs(z, r)
	if err != nil {
		return "", nil, err
	}
	locations = []string{primaryLoc}
	var fallbacks []string
	if isRegionalGKECluster(z, r) {
		fallbacks = fallbackGKERegions
	} else {
		fallbacks = fallbackGKEZones
	}
	for _, fb := range fallbacks {
		if fb != primaryLoc {
			locations = append(locations, fb)
		}
	}
	return locationArg, locations, nil
}

func isGKEStockoutError(output string) bool {
	stockoutPatterns := []string{
		"GCE_STOCKOUT",
		"does not have enough resources available",
		"RESOURCE_POOL_EXHAUSTED",
		"ZONE_RESOURCE_POOL_EXHAUSTED",
	}
	for _, pattern := range stockoutPatterns {
		if strings.Contains(output, pattern) {
			return true
		}
	}
	return false
}

func deleteExistingGKEClusterIfPresent(locationArg, locationVal string) error {
	out, err := exec.Command("gcloud", "container", "clusters", "list",
		locationArg, locationVal, "--verbosity", "none", "--filter",
		fmt.Sprintf("name=%s", *gkeTestClusterName)).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to check for previous test cluster: %v %s", err.Error(), out)
	}
	if len(out) > 0 {
		klog.Infof("Detected previous cluster %s in %s. Deleting so a new one can be created...", *gkeTestClusterName, locationVal)
		var z, r string
		if locationArg == "--zone" {
			z = locationVal
		} else {
			r = locationVal
		}
		if err := clusterDownGKE(z, r); err != nil {
			return err
		}
	}
	return nil
}

func clusterUpGKE(z, r string, numNodes int, numWindowsNodes int, imageType string, useManagedDriver bool) error {
	locationArg, candidateLocations, err := candidateGKELocations(z, r)
	if err != nil {
		return err
	}

	machineType := "e2-standard-2"
	if isVariableSet(gkeMachineType) {
		machineType = *gkeMachineType
	}

	var locationVal string
	var lastCreateErr error
	for i, candidateLoc := range candidateLocations {
		if err := deleteExistingGKEClusterIfPresent(locationArg, candidateLoc); err != nil {
			return err
		}

		// Assemble parameters from passed in arguments.
		cmdParams := []string{"container", "clusters", "create", *gkeTestClusterName,
			locationArg, candidateLoc, "--num-nodes", strconv.Itoa(numNodes),
			"--quiet", "--machine-type", machineType, "--no-enable-autoupgrade"}

		if imageType == "win2019" || imageType == "win2022" {
			cmdParams = append(cmdParams, "--image-type", "WINDOWS_LTSC_CONTAINERD")
			if imageType == "win2019" {
				cmdParams = append(cmdParams, "--windows-os-version", "ltsc2019")
			} else {
				cmdParams = append(cmdParams, "--windows-os-version", "ltsc2022")
			}
		} else {
			cmdParams = append(cmdParams, "--image-type", imageType)
		}

		if isVariableSet(gkeReleaseChannel) {
			cmdParams = append(cmdParams, "--release-channel", *gkeReleaseChannel)

			// Release channel based GKE clusters require autorepair to be enabled.
			cmdParams = append(cmdParams, "--enable-autorepair")

			if *gkeReleaseChannel == "extended" {
				// Extended channel clusters require autoupgrade to be enabled.
				cmdParams = append(cmdParams, "--enable-autoupgrade")

				// We often leverage the extended channel to test the oldest
				// versions.  In that case, we specify both a release channel *and* a
				// cluster version.
				if isVariableSet(gkeClusterVer) {
					cmdParams = append(cmdParams, "--cluster-version", *gkeClusterVer)
				}
			}
		} else if isVariableSet(gkeClusterVer) {
			cmdParams = append(cmdParams, "--cluster-version", *gkeClusterVer)
		} else {
			return fmt.Errorf("must specify either release channel or cluster version")
		}

		if isVariableSet(gkeNodeVersion) {
			cmdParams = append(cmdParams, "--node-version", *gkeNodeVersion)
		}

		if useManagedDriver {
			cmdParams = append(cmdParams, "--addons", "GcePersistentDiskCsiDriver")
		}

		cmd := exec.Command("gcloud", cmdParams...)
		createOut, createErr := runCommandWithOutput("Starting E2E Cluster on GKE", cmd)
		if createErr == nil {
			locationVal = candidateLoc
			if locationArg == "--zone" {
				*gceZone = candidateLoc
			} else {
				*gceRegion = candidateLoc
			}
			lastCreateErr = nil
			break
		}

		lastCreateErr = createErr
		if cleanupErr := deleteExistingGKEClusterIfPresent(locationArg, candidateLoc); cleanupErr != nil {
			klog.Warningf("Failed to clean up partially created cluster in %s: %v", candidateLoc, cleanupErr)
		}
		if isGKEStockoutError(createOut) && i+1 < len(candidateLocations) {
			klog.Warningf("GKE cluster creation in %s failed due to stockout; retrying in fallback location %s", candidateLoc, candidateLocations[i+1])
			continue
		}
		break
	}
	if lastCreateErr != nil {
		return fmt.Errorf("failed to bring up kubernetes e2e cluster on gke: %v", lastCreateErr.Error())
	}

	// To avoid unexpected GKE maintenance, create a 4-hour maintenance exclusion window.
	startExclusionTime := time.Now().UTC()
	cmd := exec.Command("gcloud", "container", "clusters", "update", *gkeTestClusterName, locationArg, locationVal, "--quiet",
		"--add-maintenance-exclusion-name", "no-upgrades-during-test",
		"--add-maintenance-exclusion-start", startExclusionTime.Format(time.RFC3339),
		"--add-maintenance-exclusion-end", startExclusionTime.Add(2*time.Hour).Format(time.RFC3339),
		"--add-maintenance-exclusion-scope", "no_upgrades")
	if err := runCommand("Updating Cluster with maintenance window", cmd); err != nil {
		return fmt.Errorf("failed to update cluster with maintenance window: %w", err)
	}

	// Because gcloud cannot disable addons on cluster create, the deployment has
	// to be disabled on update.
	clusterVersion := mustGetKubeClusterVersion()
	if !useManagedDriver && isGKEDeploymentInstalledByDefault(clusterVersion) {
		cmd = exec.Command(
			"gcloud", "beta", "container", "clusters", "update",
			*gkeTestClusterName, locationArg, locationVal, "--quiet",
			"--update-addons", "GcePersistentDiskCsiDriver=DISABLED")
		err = runCommand("Updating E2E Cluster on GKE to disable driver deployment", cmd)
		if err != nil {
			return fmt.Errorf("failed to update kubernetes e2e cluster on gke: %v", err.Error())
		}
	}

	cmd = exec.Command("gcloud", "container", "clusters", "describe", *gkeTestClusterName, locationArg, locationVal)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to list cluster: %v %s", err, out)
	}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	printedHash := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "id: ") {
			klog.Infof("GKE cluster: %s %s", *gkeTestClusterName, locationVal)
			klog.Infof("GKE cluster hash %s", line)
			printedHash = true
			break
		}
	}
	if !printedHash {
		return fmt.Errorf("failed to find cluster hash in cluster describe: %s", out)
	}

	return nil
}

func downloadTarball(k8sDir, releaseVersion, subDir, tarballName string) error {
	tarballPath := filepath.Join(k8sDir, subDir)
	if err := os.MkdirAll(tarballPath, 0777); err != nil {
		return err
	}
	tarballOutput := filepath.Join(tarballPath, tarballName)
	downloadUrl := fmt.Sprintf("https://dl.k8s.io/release/%s/%s", releaseVersion, tarballName)
	klog.Infof("Downloading tarball %s to path=%s from url=%s", tarballName, tarballPath, downloadUrl)
	_, err := exec.Command("curl", "-Lsf", "--output", tarballOutput, downloadUrl).CombinedOutput()
	if err != nil {
		return err
	}

	return nil
}

func downloadAndExtractSrcTarball(k8sDir, releaseVersion, tarballName string) error {
	tarballPath := filepath.Join(k8sDir, tarballName)
	if err := downloadTarball(k8sDir, releaseVersion, "", tarballName); err != nil {
		return err
	}
	out, err := exec.Command("tar", "xf", tarballPath, "-C", k8sDir).CombinedOutput()
	if err != nil {
		return fmt.Errorf("Failed to extract src tarball %s: %s: %s", tarballName, out, err)
	}
	return nil
}

func getReleaseVersionFromKubeVersion(kubeVersion string) (string, error) {
	releaseVersion := fmt.Sprintf("v%s", kubeVersion)
	if kubeVersion == "stable" || kubeVersion == "latest" {
		// See https://kubernetes.io/releases/download/
		out, err := exec.Command("curl", "-Lsf", "https://dl.k8s.io/release/stable.txt").CombinedOutput()
		if err != nil {
			return "", err
		}
		releaseVersion = string(out)
	}
	return releaseVersion, nil
}

func downloadKubernetesRelease(k8sDir, kubeVersion, platform, arch string) error {
	releaseVersion, err := getReleaseVersionFromKubeVersion(kubeVersion)
	if err != nil {
		return err
	}

	// Download precompiled tarballs
	serverTarballName := fmt.Sprintf("kubernetes-server-%s-%s.tar.gz", platform, arch)
	if err := downloadTarball(k8sDir, releaseVersion, "server", serverTarballName); err != nil {
		return err
	}

	manifestsTarballName := fmt.Sprintf("kubernetes-manifests.tar.gz")
	if err := downloadTarball(k8sDir, releaseVersion, "server", manifestsTarballName); err != nil {
		return err
	}

	nodeTarballName := fmt.Sprintf("kubernetes-node-%s-%s.tar.gz", platform, arch)
	if err := downloadTarball(k8sDir, releaseVersion, "node", nodeTarballName); err != nil {
		return err
	}

	clientTarballName := fmt.Sprintf("kubernetes-client-%s-%s.tar.gz", platform, arch)
	if err := downloadTarball(k8sDir, releaseVersion, "client", clientTarballName); err != nil {
		return err
	}

	return nil
}

func downloadKubernetesSource(k8sDir, kubeVersion string) error {
	klog.Infof("Downloading Kubernetes source v=%s to path=%s", kubeVersion, k8sDir)

	if err := os.MkdirAll(k8sDir, 0777); err != nil {
		return err
	}
	if err := os.RemoveAll(k8sDir); err != nil {
		return err
	}

	// We clone rather than download from release archives, because the file naming has not been
	// stable.  For example, in late 2021 it appears that archives of minor versions (eg v1.21.tgz)
	// stopped and was replaced with just patch version.
	if kubeVersion == "master" {
		// Clone of master. We cannot use a shallow clone, because the k8s version is not set, and
		// in order to find the revision git searches through the tags, and tags are not fetched in
		// a shallow clone. Not using a shallow clone adds about 700M to the ~5G archive directory,
		// after make quick-release, so this is not disastrous.
		klog.Info("cloning k8s master")
		out, err := exec.Command("git", "clone", "https://github.com/kubernetes/kubernetes", k8sDir).CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to clone kubernetes master: %s, err: %v", out, err.Error())
		}
	} else {
		releaseVersion, err := getReleaseVersionFromKubeVersion(kubeVersion)
		if err != nil {
			return err
		}

		// Download versioned source.
		if err := downloadAndExtractSrcTarball(k8sDir, releaseVersion, "kubernetes-src.tar.gz"); err != nil {
			return err
		}
		if err := downloadAndExtractSrcTarball(k8sDir, releaseVersion, "kubernetes.tar.gz"); err != nil {
			return err
		}
	}
	return nil
}

func getGKEKubeTestArgs(gceZone, gceRegion, imageType string, useKubetest2 bool) ([]string, error) {
	var locationArg, locationVal, locationArgK2 string
	switch {
	case len(gceZone) > 0:
		locationArg = "--gcp-zone"
		locationArgK2 = "--zone"
		locationVal = gceZone
	case len(gceRegion) > 0:
		locationArg = "--gcp-region"
		locationArgK2 = "--region"
		locationVal = gceRegion
	}

	var gkeEnv string
	switch gkeURL := os.Getenv("CLOUDSDK_API_ENDPOINT_OVERRIDES_CONTAINER"); gkeURL {
	case "https://staging-container.sandbox.googleapis.com/":
		gkeEnv = "staging"
	case "https://test-container.sandbox.googleapis.com/":
		gkeEnv = "test"
	case "":
		gkeEnv = "prod"
	default:
		// if the URL does not match to an option, assume it is a custom GKE backend
		// URL and pass that to kubetest
		gkeEnv = gkeURL
	}

	cmd := exec.Command("gcloud", "config", "get-value", "project")
	project, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get current project: %v", err.Error())
	}

	// kubetest arguments
	args := []string{
		"--up=false",
		"--down=false",
		"--provider=gke",
		"--gcp-network=default",
		"--check-version-skew=false",
		"--deployment=gke",
		fmt.Sprintf("--gcp-node-image=%s", imageType),
		"--gcp-network=default",
		fmt.Sprintf("--cluster=%s", *gkeTestClusterName),
		fmt.Sprintf("--gke-environment=%s", gkeEnv),
		fmt.Sprintf("%s=%s", locationArg, locationVal),
		fmt.Sprintf("--gcp-project=%s", project[:len(project)-1]),
	}

	// kubetest2 arguments
	argsK2 := []string{
		"--up=false",
		"--down=false",
		fmt.Sprintf("--cluster-name=%s", *gkeTestClusterName),
		fmt.Sprintf("--environment=%s", gkeEnv),
		fmt.Sprintf("%s=%s", locationArgK2, locationVal),
		fmt.Sprintf("--project=%s", project[:len(project)-1]),
	}

	if useKubetest2 {
		return argsK2, nil
	} else {
		return args, nil
	}
}

func getNormalizedVersion(kubeVersion, gkeVersion string) (string, error) {
	if kubeVersion != "" && gkeVersion != "" {
		return "", fmt.Errorf("both kube version (%s) and gke version (%s) specified", kubeVersion, gkeVersion)
	}
	if kubeVersion == "" && gkeVersion == "" {
		return "", errors.New("neither kube version nor gke version specified")
	}
	var v string
	if kubeVersion != "" {
		v = kubeVersion
	} else if gkeVersion != "" {
		v = gkeVersion
	}
	if v == "master" || v == "latest" {
		// Ugh
		return v, nil
	}
	toks := strings.Split(v, ".")
	if len(toks) < 2 || len(toks) > 3 {
		return "", fmt.Errorf("got unexpected number of tokens in version string %s - wanted 2 or 3", v)
	}
	return strings.Join(toks[:2], "."), nil

}

func getKubeClusterVersion() (string, error) {
	out, err := exec.Command("kubectl", "version", "-o=json").Output()
	if err != nil {
		return "", fmt.Errorf("failed to obtain cluster version, error: %v; output was %s", err.Error(), out)
	}
	type version struct {
		ClientVersion *apimachineryversion.Info `json:"clientVersion,omitempty" yaml:"clientVersion,omitempty"`
		ServerVersion *apimachineryversion.Info `json:"serverVersion,omitempty" yaml:"serverVersion,omitempty"`
	}

	var v version
	err = json.Unmarshal(out, &v)
	if err != nil {
		return "", fmt.Errorf("Failed to parse kubectl version output, error: %v", err.Error())
	}

	return v.ServerVersion.GitVersion, nil
}

func mustGetKubeClusterVersion() string {
	ver, err := getKubeClusterVersion()
	if err != nil {
		klog.Fatalf("Error: %v", err.Error())
	}
	return ver
}

// getKubeConfig returns the full path to the
// kubeconfig file set in $KUBECONFIG env.
// If unset, then it defaults to $HOME/.kube/config
func getKubeConfig() (string, error) {
	config, ok := os.LookupEnv("KUBECONFIG")
	if ok {
		return config, nil
	}
	homeDir, ok := os.LookupEnv("HOME")
	if !ok {
		return "", fmt.Errorf("HOME env not set")
	}
	return filepath.Join(homeDir, ".kube/config"), nil
}

// getKubeClient returns a Kubernetes client interface
// for the test cluster
func getKubeClient() (kubernetes.Interface, error) {
	kubeConfig, err := getKubeConfig()
	if err != nil {
		return nil, err
	}
	config, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create config: %v", err.Error())
	}
	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %v", err.Error())
	}
	return kubeClient, nil
}

func isGKEDeploymentInstalledByDefault(clusterVersion string) bool {
	cv := mustParseVersion(clusterVersion)
	return cv.atLeast(mustParseVersion("1.18.10-gke.2101")) &&
		cv.lessThan(mustParseVersion("1.19.0")) ||
		cv.atLeast(mustParseVersion("1.19.3-gke.2100"))
}
