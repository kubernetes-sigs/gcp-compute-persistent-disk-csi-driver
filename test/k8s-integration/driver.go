package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func getOverlayDir(pkgDir, deployOverlayName string) string {
	return filepath.Join(pkgDir, "deploy", "kubernetes", "overlays", deployOverlayName)
}

func installDriver(goPath, pkgDir, stagingImage, stagingVersion, deployOverlayName string, doDriverBuild bool) error {
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
	tmpSaFile := filepath.Join(generateUniqueTmpDir(), "cloud-sa.json")
	defer removeDir(filepath.Dir(tmpSaFile))

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
	time.Sleep(time.Minute)
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
