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
	"os"
	"os/exec"
	"os/user"
	"strings"

	"github.com/golang/glog"
)

var (
	sshOption     = "-o UserKnownHostsFile=/dev/null -o IdentitiesOnly=yes -o CheckHostIP=no -o StrictHostKeyChecking=no -o ServerAliveInterval=30 -o LogLevel=ERROR"
	sshDefaultKey string
)

func init() {
	usr, err := user.Current()
	if err != nil {
		glog.Fatal(err)
	}
	sshDefaultKey = fmt.Sprintf("%s/.ssh/google_compute_engine", usr.HomeDir)

}

// GetHostnameOrIP converts hostname into ip and apply user if necessary.
func (i *InstanceInfo) GetSSHTarget() string {
	var target string

	if _, ok := os.LookupEnv("JENKINS_GCE_SSH_PRIVATE_KEY_FILE"); ok {
		target = fmt.Sprintf("prow@%s", i.externalIP)
	} else {
		target = fmt.Sprintf("%s", i.externalIP)
	}
	return target
}

// getSSHCommand handles proper quoting so that multiple commands are executed in the same shell over ssh
func getSSHCommand(sep string, args ...string) string {
	return fmt.Sprintf("'%s'", strings.Join(args, sep))
}

// SSH executes ssh command with runSSHCommand as root. The `sudo` makes sure that all commands
// are executed by root, so that there won't be permission mismatch between different commands.
func (i *InstanceInfo) SSH(cmd ...string) (string, error) {
	return runSSHCommand("ssh", append([]string{i.GetSSHTarget(), "--", "sudo"}, cmd...)...)
}

func (i *InstanceInfo) CreateSSHTunnel(localPort, serverPort string) (int, error) {
	args := []string{"-nNT", "-L", fmt.Sprintf("%s:localhost:%s", localPort, serverPort), i.GetSSHTarget()}
	if pk, ok := os.LookupEnv("JENKINS_GCE_SSH_PRIVATE_KEY_FILE"); ok {
		glog.V(4).Infof("Running on Jenkins, using special private key file at %v", pk)
		args = append([]string{"-i", pk}, args...)
	} else {
		args = append([]string{"-i", sshDefaultKey}, args...)
	}
	args = append(strings.Split(sshOption, " "), args...)
	cmd := exec.Command("ssh", args...)
	err := cmd.Start()
	if err != nil {
		return 0, err
	}

	return cmd.Process.Pid, nil
}

// SSHNoSudo executes ssh command with runSSHCommand as normal user. Sometimes we need this,
// for example creating a directory that we'll copy files there with scp.
func (i *InstanceInfo) SSHNoSudo(cmd ...string) (string, error) {
	return runSSHCommand("ssh", append([]string{i.GetSSHTarget(), "--"}, cmd...)...)
}

// SSHCheckAlive just pings the server quickly to check whether it is reachable by SSH
func (i *InstanceInfo) SSHCheckAlive() (string, error) {
	return runSSHCommand("ssh", []string{i.GetSSHTarget(), "-o", "ConnectTimeout=10", "--", "echo"}...)
}

// runSSHCommand executes the ssh or scp command, adding the flag provided --ssh-options
func runSSHCommand(cmd string, args ...string) (string, error) {
	if pk, ok := os.LookupEnv("JENKINS_GCE_SSH_PRIVATE_KEY_FILE"); ok {
		glog.V(4).Infof("Running on Jenkins, using special private key file at %v", pk)
		args = append([]string{"-i", pk}, args...)
	} else {
		args = append([]string{"-i", sshDefaultKey}, args...)
	}
	args = append(strings.Split(sshOption, " "), args...)

	glog.V(4).Infof("Executing SSH command: %v %v", cmd, args)

	output, err := exec.Command(cmd, args...).CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("command [%s %s] failed with error: %v", cmd, strings.Join(args, " "), err)
	}
	return string(output), nil
}
