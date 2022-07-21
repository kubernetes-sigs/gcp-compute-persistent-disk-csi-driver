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

package utils

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2/google"
	cloudresourcemanager "google.golang.org/api/cloudresourcemanager/v1"
	"k8s.io/klog"
	boskosclient "sigs.k8s.io/boskos/client"
	"sigs.k8s.io/boskos/common"
	utilcommon "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
	remote "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/test/remote"
)

const (
	DiskLabelKey   = "csi"
	DiskLabelValue = "e2e-test"
)

var (
	boskos, _ = boskosclient.NewClient(os.Getenv("JOB_NAME"), "http://boskos", "", "")
)

func GCEClientAndDriverSetup(instance *remote.InstanceInfo) (*remote.TestContext, error) {
	port := fmt.Sprintf("%v", 1024+rand.Intn(10000))
	goPath, ok := os.LookupEnv("GOPATH")
	if !ok {
		return nil, fmt.Errorf("Could not find environment variable GOPATH")
	}
	pkgPath := path.Join(goPath, "src/sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/")
	binPath := path.Join(pkgPath, "bin/gce-pd-csi-driver")

	endpoint := fmt.Sprintf("tcp://localhost:%s", port)

	workspace := remote.NewWorkspaceDir("gce-pd-e2e-")
	driverRunCmd := fmt.Sprintf("sh -c '/usr/bin/nohup %s/gce-pd-csi-driver -v=4 --endpoint=%s --extra-labels=%s=%s 2> %s/prog.out < /dev/null > /dev/null &'",
		workspace, endpoint, DiskLabelKey, DiskLabelValue, workspace)

	config := &remote.ClientConfig{
		PkgPath:      pkgPath,
		BinPath:      binPath,
		WorkspaceDir: workspace,
		RunDriverCmd: driverRunCmd,
		Port:         port,
	}

	err := os.Setenv("GCE_PD_CSI_STAGING_VERSION", "latest")
	if err != nil {
		return nil, err
	}

	return remote.SetupNewDriverAndClient(instance, config)
}

// getBoskosProject retries acquiring a boskos project until success or timeout
func getBoskosProject(resourceType string) *common.Resource {
	timer := time.NewTimer(30 * time.Minute)
	defer timer.Stop()
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-timer.C:
			klog.Fatalf("timed out trying to acquire boskos project")
		case <-ticker.C:
			p, err := boskos.Acquire(resourceType, "free", "busy")
			if err != nil {
				klog.Warningf("boskos failed to acquire project: %v", err)
			} else if p == nil {
				klog.Warningf("boskos does not have a free %s at the moment", resourceType)
			} else {
				return p
			}
		}
	}

}

func SetupProwConfig(resourceType string) (project, serviceAccount string) {
	// Try to get a Boskos project
	klog.V(4).Infof("Running in PROW")
	klog.V(4).Infof("Fetching a Boskos loaned project")

	p := getBoskosProject(resourceType)
	project = p.Name

	go func(c *boskosclient.Client, proj string) {
		for range time.Tick(time.Minute * 5) {
			if err := c.UpdateOne(p.Name, "busy", nil); err != nil {
				klog.Warningf("[Boskos] Update %s failed with %v", p.Name, err)
			}
		}
	}(boskos, p.Name)

	// If we're on CI overwrite the service account
	klog.V(4).Infof("Fetching the default compute service account")

	c, err := google.DefaultClient(context.Background(), cloudresourcemanager.CloudPlatformScope)
	if err != nil {
		klog.Fatalf("Failed to get Google Default Client: %v", err)
	}

	cloudresourcemanagerService, err := cloudresourcemanager.New(c)
	if err != nil {
		klog.Fatalf("Failed to create new cloudresourcemanager: %v", err)
	}

	resp, err := cloudresourcemanagerService.Projects.Get(project).Do()
	if err != nil {
		klog.Fatalf("Failed to get project %v from Cloud Resource Manager: %v", project, err)
	}

	// Default Compute Engine service account
	// [PROJECT_NUMBER]-compute@developer.gserviceaccount.com
	serviceAccount = fmt.Sprintf("%v-compute@developer.gserviceaccount.com", resp.ProjectNumber)
	klog.Infof("Using project %v and service account %v", project, serviceAccount)
	return project, serviceAccount
}

func ForceChmod(instance *remote.InstanceInfo, filePath string, perms string) error {
	originalumask, err := instance.SSHNoSudo("umask")
	if err != nil {
		return fmt.Errorf("failed to umask. Output: %v, errror: %v", originalumask, err)
	}
	output, err := instance.SSHNoSudo("umask", "0000")
	if err != nil {
		return fmt.Errorf("failed to umask. Output: %v, errror: %v", output, err)
	}
	output, err = instance.SSH("chmod", "-R", perms, filePath)
	if err != nil {
		return fmt.Errorf("failed to chmod file %s. Output: %v, errror: %v", filePath, output, err)
	}
	output, err = instance.SSHNoSudo("umask", originalumask)
	if err != nil {
		return fmt.Errorf("failed to umask. Output: %v, errror: %v", output, err)
	}
	return nil
}

func WriteFile(instance *remote.InstanceInfo, filePath, fileContents string) error {
	output, err := instance.SSHNoSudo("echo", fileContents, ">", filePath)
	if err != nil {
		return fmt.Errorf("failed to write test file %s. Output: %v, errror: %v", filePath, output, err)
	}
	return nil
}

func ReadFile(instance *remote.InstanceInfo, filePath string) (string, error) {
	output, err := instance.SSHNoSudo("cat", filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read test file %s. Output: %v, errror: %v", filePath, output, err)
	}
	return output, nil
}

func WriteBlock(instance *remote.InstanceInfo, path, fileContents string) error {
	output, err := instance.SSHNoSudo("echo", fileContents, "|", "dd", "of="+path)
	if err != nil {
		return fmt.Errorf("failed to write test file %s. Output: %v, errror: %v", path, output, err)
	}
	return nil
}

func ReadBlock(instance *remote.InstanceInfo, path string, length int) (string, error) {
	lengthStr := strconv.Itoa(length)
	output, err := instance.SSHNoSudo("dd", "if="+path, "bs="+lengthStr, "count=1", "2>", "/dev/null")
	if err != nil {
		return "", fmt.Errorf("failed to read test file %s. Output: %v, errror: %v", path, output, err)
	}
	return output, nil
}

func GetFSSizeInGb(instance *remote.InstanceInfo, mountPath string) (int64, error) {
	output, err := instance.SSH("df", "--output=size", "-BG", mountPath, "|", "awk", "'NR==2'")
	if err != nil {
		return -1, fmt.Errorf("failed to get size of path %s. Output: %v, error: %v", mountPath, output, err)
	}
	output = strings.TrimSuffix(strings.TrimSpace(output), "G")
	n, err := strconv.ParseInt(output, 10, 64)
	if err != nil {
		return -1, fmt.Errorf("failed to parse size %s into int", output)
	}
	return n, nil
}

func GetBlockSizeInGb(instance *remote.InstanceInfo, devicePath string) (int64, error) {
	output, err := instance.SSH("blockdev", "--getsize64", devicePath)
	if err != nil {
		return -1, fmt.Errorf("failed to get size of path %s. Output: %v, error: %v", devicePath, output, err)
	}
	n, err := strconv.ParseInt(strings.TrimSpace(output), 10, 64)
	if err != nil {
		return -1, fmt.Errorf("failed to parse size %s into int", output)
	}
	return utilcommon.BytesToGbRoundDown(n), nil
}

func Symlink(instance *remote.InstanceInfo, src, dest string) error {
	output, err := instance.SSH("ln", "-s", src, dest)
	if err != nil {
		return fmt.Errorf("failed to symlink from %s to %s. Output: %v, errror: %v", src, dest, output, err)
	}
	return nil
}

func RmAll(instance *remote.InstanceInfo, filePath string) error {
	output, err := instance.SSH("rm", "-rf", filePath)
	if err != nil {
		return fmt.Errorf("failed to delete all %s. Output: %v, errror: %v", filePath, output, err)
	}
	return nil
}

func MkdirAll(instance *remote.InstanceInfo, dir string) error {
	output, err := instance.SSH("mkdir", "-p", dir)
	if err != nil {
		return fmt.Errorf("failed to mkdir -p %s. Output: %v, errror: %v", dir, output, err)
	}
	return nil
}

func CopyFile(instance *remote.InstanceInfo, src, dest string) error {
	output, err := instance.SSH("cp", src, dest)
	if err != nil {
		return fmt.Errorf("failed to copy %s to %s. Output: %v, errror: %v", src, dest, output, err)
	}
	return nil
}

// ValidateLogicalLinkIsDisk takes a symlink location at "link" and finds the
// link location - it then finds the backing PD using either scsi_id or
// google_nvme_id (depending on the /dev path) and validates that it is the
// same as diskName
func ValidateLogicalLinkIsDisk(instance *remote.InstanceInfo, link, diskName string) (bool, error) {
	const (
		scsiPattern       = `^0Google\s+PersistentDisk\s+([\S]+)\s*$`
		sdPattern         = `sd\w+`
		nvmeSerialPattern = `ID_SERIAL_SHORT=([\S]+)\s*`
		nvmeDevPattern    = `nvme\w+`
	)
	// regex to parse scsi_id output and extract the serial
	scsiRegex := regexp.MustCompile(scsiPattern)
	sdRegex := regexp.MustCompile(sdPattern)
	nvmeSerialRegex := regexp.MustCompile(nvmeSerialPattern)
	nvmeDevRegex := regexp.MustCompile(nvmeDevPattern)

	devFsPath, err := instance.SSH("find", link, "-printf", "'%l'")
	if err != nil {
		return false, fmt.Errorf("failed to find symbolic link for %s. Output: %v, errror: %v", link, devFsPath, err)
	}
	if len(devFsPath) == 0 {
		return false, nil
	}
	if sdx := sdRegex.FindString(devFsPath); len(sdx) != 0 {
		fullDevPath := path.Join("/dev/", string(sdx))
		scsiIDOut, err := instance.SSH("/lib/udev_containerized/scsi_id", "--page=0x83", "--whitelisted", fmt.Sprintf("--device=%v", fullDevPath))
		if err != nil {
			return false, fmt.Errorf("failed to find %s's SCSI ID. Output: %v, errror: %v", devFsPath, scsiIDOut, err)
		}
		scsiID := scsiRegex.FindStringSubmatch(scsiIDOut)
		if len(scsiID) == 0 {
			return false, fmt.Errorf("scsi_id output cannot be parsed: %s. Output: %v", scsiID, scsiIDOut)
		}
		if scsiID[1] != diskName {
			return false, fmt.Errorf("scsiID %s did not match expected diskName %s", scsiID, diskName)
		}
		return true, nil
	} else if nvmex := nvmeDevRegex.FindString(devFsPath); len(nvmex) != 0 {
		fullDevPath := path.Join("/dev/", string(nvmex))
		nvmeIDOut, err := instance.SSH("/lib/udev_containerized/google_nvme_id", fmt.Sprintf("-d%v", fullDevPath))
		if err != nil {
			return false, fmt.Errorf("failed to find %s's NVME ID. Output: %v, errror: %v", devFsPath, nvmeIDOut, err)
		}
		nvmeID := nvmeSerialRegex.FindStringSubmatch(nvmeIDOut)
		if len(nvmeID) == 0 {
			return false, fmt.Errorf("google_nvme_id output cannot be parsed: %s. Output: %v", nvmeID, nvmeIDOut)
		}
		if nvmeID[1] != diskName {
			return false, fmt.Errorf("nvmeID %s did not match expected diskName %s", nvmeID, diskName)
		}
		return true, nil
	}
	return false, fmt.Errorf("symlinked disk %s for diskName %s does not match a supported /dev/sd* or /dev/nvme* path", devFsPath, diskName)
}
