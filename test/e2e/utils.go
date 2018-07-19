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

package e2e

import (
	"context"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/golang/glog"
	"golang.org/x/oauth2/google"
	cloudresourcemanager "google.golang.org/api/cloudresourcemanager/v1"
	boskosclient "k8s.io/test-infra/boskos/client"
	remote "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/test/binremote"
)

var (
	boskos = boskosclient.NewClient(os.Getenv("JOB_NAME"), "http://boskos")
)

const (
	archiveName = "e2e_gce_pd_test.tar.gz"
)

func setupInstanceAndDriver(instanceProject, instanceZone, instanceName, port, instanceServiceAccount string) (*remote.InstanceInfo, error) {
	// Create the instance in the requisite zone
	instance, err := remote.CreateInstanceInfo(instanceProject, instanceZone, instanceName)
	if err != nil {
		return nil, err
	}

	err = instance.CreateInstance(instanceServiceAccount)

	if err != nil {
		return nil, err
	}

	// Create Driver Archive

	goPath, ok := os.LookupEnv("GOPATH")
	if !ok {
		return nil, fmt.Errorf("Could not find environment variable GOPATH")
	}
	pkgPath := path.Join(goPath, "src/sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/")
	binPath := path.Join(pkgPath, "bin/gce-pd-csi-driver")
	archivePath, err := remote.CreateDriverArchive(archiveName, pkgPath, binPath)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = os.Remove(archivePath)
		if err != nil {
			glog.Warningf("Failed to remove archive file %s: %v", archivePath, err)
		}
	}()

	// Upload archive to instance and run binaries
	endpoint := fmt.Sprintf("tcp://localhost:%s", port)
	workspace := remote.NewWorkspaceDir("gce-pd-e2e-")
	driverRunCmd := fmt.Sprintf("sh -c '/usr/bin/nohup %s/gce-pd-csi-driver --endpoint=%s --nodeid=%s > %s/prog.out 2> %s/prog.err < /dev/null &'",
		workspace, endpoint, instanceName, workspace, workspace)
	err = instance.UploadAndRun(archivePath, workspace, driverRunCmd)
	if err != nil {
		return nil, err
	}

	// Create an SSH tunnel from port to port
	res, err := instance.CreateSSHTunnel(port, port)
	if err != nil {
		return nil, fmt.Errorf("SSH Tunnel pid %v encountered error: %v", res, err)
	}

	return instance, nil
}

func setupProwConfig() (project, serviceAccount string) {
	// Try to get a Boskos project
	glog.V(4).Infof("Running in PROW")
	glog.V(4).Infof("Fetching a Boskos loaned project")

	p, err := boskos.Acquire("gce-project", "free", "busy")
	if err != nil {
		glog.Fatal("boskos failed to acquire project: %v", err)
	}

	if p == nil {
		glog.Fatal("boskos does not have a free gce-project at the moment")
	}

	project = p.GetName()

	go func(c *boskosclient.Client, proj string) {
		for range time.Tick(time.Minute * 5) {
			if err := c.UpdateOne(p.Name, "busy", nil); err != nil {
				glog.Warningf("[Boskos] Update %s failed with %v", p, err)
			}
		}
	}(boskos, p.Name)

	// If we're on CI overwrite the service account
	glog.V(4).Infof("Fetching the default compute service account")

	c, err := google.DefaultClient(context.TODO(), cloudresourcemanager.CloudPlatformScope)
	if err != nil {
		glog.Fatalf("Failed to get Google Default Client: %v", err)
	}

	cloudresourcemanagerService, err := cloudresourcemanager.New(c)
	if err != nil {
		glog.Fatalf("Failed to create new cloudresourcemanager: %v", err)
	}

	resp, err := cloudresourcemanagerService.Projects.Get(project).Do()
	if err != nil {
		glog.Fatal("Failed to get project %v from Cloud Resource Manager: %v", project, err)
	}

	// Default Compute Engine service account
	// [PROJECT_NUMBER]-compute@developer.gserviceaccount.com
	serviceAccount = fmt.Sprintf("%v-compute@developer.gserviceaccount.com", resp.ProjectNumber)
	return project, serviceAccount
}

func forceChmod(instance *remote.InstanceInfo, filePath string, perms string) error {
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

func writeFile(instance *remote.InstanceInfo, filePath, fileContents string) error {
	output, err := instance.SSHNoSudo("echo", fileContents, ">", filePath)
	if err != nil {
		return fmt.Errorf("failed to write test file %s. Output: %v, errror: %v", filePath, output, err)
	}
	return nil
}

func readFile(instance *remote.InstanceInfo, filePath string) (string, error) {
	output, err := instance.SSHNoSudo("cat", filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read test file %s. Output: %v, errror: %v", filePath, output, err)
	}
	return output, nil
}

func rmAll(instance *remote.InstanceInfo, filePath string) error {
	output, err := instance.SSH("rm", "-rf", filePath)
	if err != nil {
		return fmt.Errorf("failed to delete all %s. Output: %v, errror: %v", filePath, output, err)
	}
	return nil
}
