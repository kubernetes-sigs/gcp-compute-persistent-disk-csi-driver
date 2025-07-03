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
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"k8s.io/klog/v2"
)

func (i *InstanceInfo) UploadAndRun(archivePath, remoteWorkspace, driverRunCmd string) (int, error) {

	// Create the temp staging directory
	klog.V(4).Infof("Staging test binaries on %q", i.cfg.Name)

	// Do not sudo here, so that we can use scp to copy test archive to the directdory.
	if output, err := i.SSHNoSudo("mkdir", remoteWorkspace); err != nil {
		// Exit failure with the error
		return -1, fmt.Errorf("failed to create remoteWorkspace directory %q on instance %q: %v output: %q", remoteWorkspace, i.cfg.Name, err.Error(), output)
	}

	// Copy the archive to the staging directory
	if output, err := runSSHCommand("scp", archivePath, fmt.Sprintf("%s:%s/", i.GetSSHTarget(), remoteWorkspace)); err != nil {
		// Exit failure with the error
		return -1, fmt.Errorf("failed to copy test archive: %v, output: %q", err.Error(), output)
	}

	// Extract the archive
	archiveName := path.Base(archivePath)
	cmd := getSSHCommand(" && ",
		fmt.Sprintf("cd %s", remoteWorkspace),
		fmt.Sprintf("tar -xzvf ./%s", archiveName),
	)
	klog.V(4).Infof("Extracting tar on %q", i.cfg.Name)
	// Do not use sudo here, because `sudo tar -x` will recover the file ownership inside the tar ball, but
	// we want the extracted files to be owned by the current user.
	if output, err := i.SSHNoSudo("sh", "-c", cmd); err != nil {
		// Exit failure with the error
		return -1, fmt.Errorf("failed to extract test archive: %v, output: %q", err.Error(), output)
	}

	klog.V(4).Infof("Starting driver on %q", i.cfg.Name)
	// When the process is killed the driver should close the TCP endpoint, then we want to download the logs
	output, err := i.SSH(driverRunCmd)
	if err != nil {
		// Exit failure with the error
		return -1, fmt.Errorf("failed start driver, got output: %v, error: %v", output, err.Error())
	}

	// Get the driver PID
	// ps -aux | grep  /tmp/gce-pd-e2e-0180801T114407/gce-pd-csi-driver | awk '{print $2}'
	driverPIDCmd := getSSHCommand(" | ",
		"ps -aux",
		fmt.Sprintf("grep %s", remoteWorkspace),
		"grep -v grep",
		// All ye who try to deal with escaped/non-escaped quotes with exec beware.
		//`awk "{print \$2}"`,
	)
	driverPIDString, err := i.SSHNoSudo("sh", "-c", driverPIDCmd)
	if err != nil {
		// Exit failure with the error
		return -1, fmt.Errorf("failed to get PID of driver, got output: %v, error: %v", output, err.Error())
	}

	driverPID, err := strconv.Atoi(strings.Fields(driverPIDString)[1])
	if err != nil {
		return -1, fmt.Errorf("failed to convert driver PID from string %s to int: %v", driverPIDString, err.Error())
	}

	return driverPID, nil
}

func NewWorkspaceDir(workspaceDirPrefix string) string {
	return filepath.Join("/tmp", workspaceDirPrefix+getTimestamp())
}
