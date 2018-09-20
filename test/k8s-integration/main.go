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

	"github.com/golang/glog"

	testutils "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/test/e2e/utils"

	"k8s.io/apimachinery/pkg/util/uuid"
)

var (
	teardownCluster = flag.Bool("teardown-cluster", true, "teardown the cluster after the e2e test")
	stagingImage    = flag.String("staging-image", "", "name of image to stage to")
	kubeVersion     = flag.String("kube-version", "master", "version of Kubernetes to download and use")
	inProw          = flag.Bool("run-in-prow", false, "is the test running in PROW")
)

func init() {
	flag.Set("logtostderr", "true")
}
func main() {
	flag.Parse()

	if len(*stagingImage) == 0 && !*inProw {
		glog.Fatalf("staging-image is a required flag, please specify the name of image to stage to")
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
		project, _ := testutils.SetupProwConfig()

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

		*stagingImage = fmt.Sprintf("gcr.io/%s/csi/gce-pd-driver", project)

		if _, ok := os.LookupEnv("USER"); !ok {
			err = os.Setenv("USER", "prow")
			if err != nil {
				return fmt.Errorf("failed to set user in prow to prow: %v", err)
			}
		}
	}

	err := pushImage(pkgDir, *stagingImage, stagingVersion)
	if err != nil {
		return fmt.Errorf("failed pushing image: %v", err)
	}
	defer func() {
		err = deleteImage(*stagingImage, stagingVersion)
		if err != nil {
			glog.Errorf("failed to delete image: %v", err)
		}
	}()

	err = downloadKubernetesSource(pkgDir, k8sIoDir, *kubeVersion)
	if err != nil {
		return fmt.Errorf("failed to download Kubernetes source: %v", err)
	}

	err = prepareManifests(pkgDir, k8sDir, *stagingImage, stagingVersion)
	if err != nil {
		return fmt.Errorf("failed to prepare manifests: %v", err)
	}

	err = buildKubernetes(k8sDir)
	if err != nil {
		return fmt.Errorf("failed to build Kubernetes: %v", err)
	}

	err = clusterUp(k8sDir)
	if err != nil {
		return fmt.Errorf("failed to cluster up: %v", err)
	}
	if *teardownCluster {
		defer func() {
			err = clusterDown(k8sDir)
			if err != nil {
				glog.Errorf("failed to cluster down: %v", err)
			}
		}()
	}

	err = runTests(k8sDir)
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

func runTests(k8sDir string) error {
	err := os.Chdir(k8sDir)
	if err != nil {
		return err
	}
	testArgs := "--test_args=--ginkgo.focus=CSI\\sdriver:\\sgcePD"
	cmd := exec.Command("go", "run", "hack/e2e.go", "--", "--check-version-skew=false", "--test", testArgs)
	err = runLongRunningCommand("Running Tests", cmd)
	if err != nil {
		return fmt.Errorf("failed to run tests on e2e cluster: %v", err)
	}

	return nil
}

func runLongRunningCommand(action string, cmd *exec.Cmd) error {
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr

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

func clusterDown(k8sDir string) error {
	cmd := exec.Command(filepath.Join(k8sDir, "hack", "e2e-internal", "e2e-down.sh"))
	err := runLongRunningCommand("Bringing Down E2E Cluster", cmd)
	if err != nil {
		return fmt.Errorf("failed to bring down kubernetes e2e cluster: %v", err)
	}
	return nil
}

func buildKubernetes(k8sDir string) error {
	cmd := exec.Command("make", "-C", k8sDir, "quick-release")
	err := runLongRunningCommand("Building Kubernetes", cmd)
	if err != nil {
		return fmt.Errorf("failed to build Kubernetes: %v", err)
	}
	return nil
}

func clusterUp(k8sDir string) error {
	cmd := exec.Command(filepath.Join(k8sDir, "hack", "e2e-internal", "e2e-up.sh"))
	err := runLongRunningCommand("Starting E2E Cluster", cmd)
	if err != nil {
		return fmt.Errorf("failed to bring up kubernetes e2e cluster: %v", err)
	}

	return nil
}

func prepareManifests(pkgDir, k8sDir, stagingImage, stagingVersion string) error {
	// Copy manifests to manifest directory
	controllerFile := filepath.Join(pkgDir, "deploy", "kubernetes", "dev", "controller.yaml")
	nodeFile := filepath.Join(pkgDir, "deploy", "kubernetes", "dev", "node.yaml")

	kubeManifestDir := filepath.Join(k8sDir, "test", "e2e", "testing-manifests", "storage-csi", "gce-pd")

	out, err := exec.Command("cp", controllerFile, filepath.Join(kubeManifestDir, "controller_ss.yaml")).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to copy controller manifest: %s, err: %v", out, err)
	}

	out, err = exec.Command("cp", nodeFile, filepath.Join(kubeManifestDir, "node_ds.yaml")).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to copy node manifest: %s, err: %v", out, err)
	}

	// Change manifest container to staging image and version
	err = mutateManifests(filepath.Join(kubeManifestDir, "node_ds.yaml"), filepath.Join(kubeManifestDir, "controller_ss.yaml"), stagingImage, stagingVersion)
	if err != nil {
		return err
	}
	return nil
}

func mutateManifests(nodeFile, controllerFile, stagingImage, stagingVersion string) error {
	// TODO: Make this existing image configurable, ideally we aren't actually reliant on
	// a hardcoded image to replace.

	// The actual solution we need to look into doing this is using Kustomize
	// to mutate the YAML files
	existingImage := "gcr.io/dyzz-csi-staging/csi/gce-pd-driver:latest"

	escapedImage := strings.Replace(stagingImage, "/", "\\/", -1)
	escapedVersion := strings.Replace(stagingVersion, "/", "\\/", -1)
	escapedExistingImage := strings.Replace(existingImage, "/", "\\/", -1)

	sedCommand := fmt.Sprintf("s/%s/%s:%s/g", escapedExistingImage, escapedImage, escapedVersion)

	out, err := exec.Command("sed", "-i", "-e", sedCommand, nodeFile).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to sed node file %s: %s, err: %v", nodeFile, out, err)
	}
	out, err = exec.Command("sed", "-i", "-e", sedCommand, controllerFile).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to sed controller file %s: %s, err: %v", controllerFile, out, err)
	}

	return nil
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
	err = runLongRunningCommand("Pushing GCP Container", cmd)
	if err != nil {
		return fmt.Errorf("failed to run make command: err: %v", err)
	}
	return nil
}

func deleteImage(stagingImage, stagingVersion string) error {
	cmd := exec.Command("gcloud", "container", "images", "delete", fmt.Sprintf("%s:%s", stagingImage, stagingVersion), "--quiet")
	err := runLongRunningCommand("Deleting GCR Container", cmd)
	if err != nil {
		return fmt.Errorf("failed to delete container image %s:%s: %s", stagingImage, stagingVersion, err)
	}
	return nil
}
