package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"k8s.io/klog"
)

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
		"--zone", gceZone, "--quiet", "--machine-type", "n1-standard-2")
	err := runCommand("Bringing Down E2E Cluster on GKE", cmd)
	if err != nil {
		return fmt.Errorf("failed to bring down kubernetes e2e cluster on gke: %v", err)
	}
	return nil
}

func buildKubernetes(k8sDir, command string) error {
	cmd := exec.Command("make", "-C", k8sDir, command)
	err := runCommand("Building Kubernetes", cmd)
	if err != nil {
		return fmt.Errorf("failed to build Kubernetes: %v", err)
	}
	return nil
}

func clusterUpGCE(k8sDir, gceZone string, numNodes int) error {
	kshPath := filepath.Join(k8sDir, "cluster", "kubectl.sh")
	_, err := os.Stat(kshPath)
	if err == nil {
		// Set kubectl to the one bundled in the k8s tar for versioning
		err = os.Setenv("GCE_PD_KUBECTL", kshPath)
		if err != nil {
			return fmt.Errorf("failed to set cluster specific kubectl: %v", err)
		}
	} else {
		klog.Errorf("could not find cluster kubectl at %s, falling back to default kubectl", kshPath)
	}

	if len(*kubeFeatureGates) != 0 {
		err = os.Setenv("KUBE_FEATURE_GATES", *kubeFeatureGates)
		if err != nil {
			return fmt.Errorf("failed to set kubernetes feature gates: %v", err)
		}
		klog.V(4).Infof("Set Kubernetes feature gates: %v", *kubeFeatureGates)
	}

	err = os.Setenv("NUM_NODES", strconv.Itoa(numNodes))
	if err != nil {
		return err
	}

	err = os.Setenv("KUBE_GCE_ZONE", gceZone)
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

func clusterUpGKE(gceZone string, numNodes int) error {
	out, err := exec.Command("gcloud", "container", "clusters", "list", "--zone", gceZone,
		"--filter", fmt.Sprintf("name=%s", gkeTestClusterName)).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to check for previous test cluster: %v %s", err, out)
	}
	if len(out) > 0 {
		klog.Infof("Detected previous cluster %s. Deleting so a new one can be created...", gkeTestClusterName)
		err = clusterDownGKE(gceZone)
		if err != nil {
			return err
		}
	}
	cmd := exec.Command("gcloud", "container", "clusters", "create", gkeTestClusterName,
		"--zone", gceZone, "--cluster-version", *gkeClusterVer, "--num-nodes", strconv.Itoa(numNodes), "--quiet")
	err = runCommand("Staring E2E Cluster on GKE", cmd)
	if err != nil {
		return fmt.Errorf("failed to bring up kubernetes e2e cluster on gke: %v", err)
	}

	return nil
}

func downloadKubernetesSource(pkgDir, k8sIoDir, kubeVersion string) error {
	k8sDir := filepath.Join(k8sIoDir, "kubernetes")
	/*
		// TODO: Download a fresh copy every time until mutate manifests hardcoding existing image is solved.
		if _, err := os.Stat(k8sDir); !os.IsNotExist(err) {
			klog.Infof("Staging Kubernetes already found at %s, skipping download", k8sDir)
			return nil
		}
	*/

	klog.V(4).Infof("Staging Kubernetes folder not found, downloading now")

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

	klog.V(4).Infof("Successfully downloaded Kubernetes v%s to %s", kubeVersion, k8sDir)

	return nil
}
