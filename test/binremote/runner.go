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

package binremote

import (
	"fmt"
	"path"
	"path/filepath"

	"github.com/golang/glog"
)

func (i *InstanceInfo) UploadAndRun(archivePath, remoteWorkspace, driverRunCmd string) error {

	// Create the temp staging directory
	glog.V(4).Infof("Staging test binaries on %q", i.name)

	// Do not sudo here, so that we can use scp to copy test archive to the directdory.
	if output, err := i.SSHNoSudo("mkdir", remoteWorkspace); err != nil {
		// Exit failure with the error
		return fmt.Errorf("failed to create remoteWorkspace directory %q on i.name %q: %v output: %q", remoteWorkspace, i.name, err, output)
	}

	// Copy the archive to the staging directory
	if output, err := runSSHCommand("scp", archivePath, fmt.Sprintf("%s:%s/", i.GetSSHTarget(), remoteWorkspace)); err != nil {
		// Exit failure with the error
		return fmt.Errorf("failed to copy test archive: %v, output: %q", err, output)
	}

	// Extract the archive
	archiveName := path.Base(archivePath)
	cmd := getSSHCommand(" && ",
		fmt.Sprintf("cd %s", remoteWorkspace),
		fmt.Sprintf("tar -xzvf ./%s", archiveName),
	)
	glog.V(4).Infof("Extracting tar on %q", i.name)
	// Do not use sudo here, because `sudo tar -x` will recover the file ownership inside the tar ball, but
	// we want the extracted files to be owned by the current user.
	if output, err := i.SSHNoSudo("sh", "-c", cmd); err != nil {
		// Exit failure with the error
		return fmt.Errorf("failed to extract test archive: %v, output: %q", err, output)
	}

	glog.V(4).Infof("Starting driver on %q", i.name)
	// When the process is killed the driver should close the TCP endpoint, then we want to download the logs
	output, err := i.SSH(driverRunCmd)

	if err != nil {
		// Exit failure with the error
		return fmt.Errorf("failed start GCE PD driver, got output: %v, error: %v", output, err)
	}

	// TODO: return the PID so that we can kill the driver later
	// Actually just do a pkill -f my_pattern

	return nil
}

func NewWorkspaceDir(workspaceDirPrefix string) string {
	return filepath.Join("/tmp", workspaceDirPrefix+getTimestamp())
}
