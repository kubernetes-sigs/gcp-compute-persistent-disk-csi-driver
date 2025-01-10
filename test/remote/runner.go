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

package remote

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"k8s.io/klog/v2"
)

func (i *InstanceInfo) UploadAndRun(archiveName, archivePath, remoteWorkspace, driverRunCmd, pkgPath string) (int, error) {

	// Create the temp staging directory
	klog.V(4).Infof("Staging test binaries on %q", i.cfg.Name)

	// Do not sudo here, so that we can use scp to copy test archive to the directdory.
	if output, err := i.SSHNoSudo("mkdir", remoteWorkspace); err != nil {
		// Exit failure with the error
		return -1, fmt.Errorf("failed to create remoteWorkspace directory %q on instance %q: %v output: %q", remoteWorkspace, i.cfg.Name, err.Error(), output)
	}

	// Copy the setup script to the staging directory
	if output, err := runSSHCommand("scp", fmt.Sprintf("%s/test/e2e/utils/setup-remote.sh", pkgPath), fmt.Sprintf("%s:%s/", i.GetSSHTarget(), remoteWorkspace)); err != nil {
		// Exit failure with the error
		return -1, fmt.Errorf("failed to copy test archive: %v, output: %q", err.Error(), output)
	}

	// Set up the VM env with docker and make
	if output, err := i.SSH("sh", "-c", fmt.Sprintf("%s/setup-remote.sh", remoteWorkspace)); err != nil {
		return -1, fmt.Errorf("failed to setup VM environment: %v, output: %q", err.Error(), output)
	}

	// Upload local image to remote
	if output, err := runSSHCommand("scp", archivePath, fmt.Sprintf("%s:%s/", i.GetSSHTarget(), remoteWorkspace)); err != nil {
		return -1, fmt.Errorf("failed to copy image archive: %v, output: %q", err, output)
	}

	// Run PD CSI driver as a container
	klog.V(4).Infof("Starting driver on %q", i.cfg.Name)
	cmd := getSSHCommand(" && ",
		fmt.Sprintf("docker load -i %v/%v", remoteWorkspace, archiveName),
		driverRunCmd,
	)
	output, err := i.SSH("sh", "-c", cmd)
	if err != nil {
		return -1, fmt.Errorf("failed to load or run docker image: %v, output: %q", err, output)
	}

	// Grab the container ID from `docker run` output
	driverRunOutputs := strings.Split(output, "\n")
	numSplits := len(driverRunOutputs)
	if numSplits < 2 {
		return -1, fmt.Errorf("failed to get driver container ID from driver run outputs, outputs are: %v", output)
	}
	// Grabbing the second last split because it contains an empty string
	driverContainerID := driverRunOutputs[len(driverRunOutputs)-2]

	// Grab driver PID from container ID
	driverPIDStr, err := i.SSH(fmt.Sprintf("docker inspect -f {{.State.Pid}} %v", driverContainerID))
	if err != nil {
		// Exit failure with the error
		return -1, fmt.Errorf("failed to get PID of driver, got output: %v, error: %v", driverPIDStr, err.Error())
	}
	driverPIDStr = strings.TrimSpace(driverPIDStr)
	driverPID, err := strconv.Atoi(driverPIDStr)
	if err != nil {
		return -1, fmt.Errorf("failed to convert driver PID from string %s to int: %v", driverPIDStr, err.Error())
	}
	klog.V(4).Infof("Driver PID is: %v", driverPID)

	return driverPID, nil
}

func NewWorkspaceDir(workspaceDirPrefix string) string {
	return filepath.Join("/tmp", workspaceDirPrefix+getTimestamp())
}
