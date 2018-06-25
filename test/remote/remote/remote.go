/*
Copyright 2016 The Kubernetes Authors.

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
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/golang/glog"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
)

var testTimeoutSeconds = flag.Duration("test-timeout", 45*time.Minute, "How long (in golang duration format) to wait for ginkgo tests to complete.")
var resultsDir = flag.String("results-dir", "/tmp/", "Directory to scp test results to.")

const archiveName = "e2e_gce_pd_test.tar.gz"

func CreateTestArchive(suite TestSuite) (string, error) {
	glog.V(2).Infof("Building archive...")
	tardir, err := ioutil.TempDir("", "gce-pd-e2e-archive")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary directory %v", err)
	}
	defer os.RemoveAll(tardir)

	// Call the suite function to setup the test package.
	err = suite.SetupTestPackage(tardir)
	if err != nil {
		return "", fmt.Errorf("failed to setup test package %q: %v", tardir, err)
	}

	// Build the tar
	out, err := exec.Command("tar", "-zcvf", archiveName, "-C", tardir, ".").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to build tar %v.  Output:\n%s", err, out)
	}

	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory %v", err)
	}
	return filepath.Join(dir, archiveName), nil
}

// RunRemote returns the command output, whether the exit was ok, and any errors
func RunRemote(suite TestSuite, archive string, host string, cleanup bool, testArgs string, ginkgoArgs string) (string, bool, error) {
	// Create the temp staging directory
	glog.V(2).Infof("Staging test binaries on %q", host)
	workspace := newWorkspaceDir()
	// Do not sudo here, so that we can use scp to copy test archive to the directdory.
	if output, err := SSHNoSudo(host, "mkdir", workspace); err != nil {
		// Exit failure with the error
		return "", false, fmt.Errorf("failed to create workspace directory %q on host %q: %v output: %q", workspace, host, err, output)
	}
	if cleanup {
		defer func() {
			output, err := SSH(host, "rm", "-rf", workspace)
			if err != nil {
				glog.Errorf("failed to cleanup workspace %q on host %q: %v.  Output:\n%s", workspace, host, err, output)
			}
		}()
	}

	// Copy the archive to the staging directory
	if output, err := runSSHCommand("scp", archive, fmt.Sprintf("%s:%s/", GetHostnameOrIP(host), workspace)); err != nil {
		// Exit failure with the error
		return "", false, fmt.Errorf("failed to copy test archive: %v, output: %q", err, output)
	}

	// Extract the archive
	cmd := getSSHCommand(" && ",
		fmt.Sprintf("cd %s", workspace),
		fmt.Sprintf("tar -xzvf ./%s", archiveName),
	)
	glog.V(2).Infof("Extracting tar on %q", host)
	// Do not use sudo here, because `sudo tar -x` will recover the file ownership inside the tar ball, but
	// we want the extracted files to be owned by the current user.
	if output, err := SSHNoSudo(host, "sh", "-c", cmd); err != nil {
		// Exit failure with the error
		return "", false, fmt.Errorf("failed to extract test archive: %v, output: %q", err, output)
	}

	// Create the test result directory.
	resultDir := filepath.Join(workspace, "results")
	if output, err := SSHNoSudo(host, "mkdir", resultDir); err != nil {
		// Exit failure with the error
		return "", false, fmt.Errorf("failed to create test result directory %q on host %q: %v output: %q", resultDir, host, err, output)
	}

	glog.V(2).Infof("Running test on %q", host)
	output, err := suite.RunTest(host, workspace, resultDir, testArgs, ginkgoArgs, *testTimeoutSeconds)

	aggErrs := []error{}
	// Do not log the output here, let the caller deal with the test output.
	if err != nil {
		aggErrs = append(aggErrs, err)
	}

	/*
		glog.V(2).Infof("Copying test artifacts from %q", host)
		scpErr := getTestArtifacts(host, workspace)
		if scpErr != nil {
			aggErrs = append(aggErrs, scpErr)
		}
	*/

	return output, len(aggErrs) == 0, utilerrors.NewAggregate(aggErrs)
}

const (
	// workspaceDirPrefix is the string prefix used in the workspace directory name.
	workspaceDirPrefix = "gce-pd-e2e-"
	// timestampFormat is the timestamp format used in the e2e directory name.
	timestampFormat = "20060102T150405"
)

func getTimestamp() string {
	return fmt.Sprintf(time.Now().Format(timestampFormat))
}

func newWorkspaceDir() string {
	return filepath.Join("/tmp", workspaceDirPrefix+getTimestamp())
}

// Parses the workspace directory name and gets the timestamp part of it.
// This can later be used to name other artifacts (such as the
// kubelet-${instance}.service systemd transient service used to launch
// Kubelet) so that they can be matched to each other.
func GetTimestampFromWorkspaceDir(dir string) string {
	dirTimestamp := strings.TrimPrefix(filepath.Base(dir), workspaceDirPrefix)
	re := regexp.MustCompile("^\\d{8}T\\d{6}$")
	if re.MatchString(dirTimestamp) {
		return dirTimestamp
	}
	// Fallback: if we can't find that timestamp, default to using Now()
	return getTimestamp()
}

func getTestArtifacts(host, testDir string) error {
	logPath := filepath.Join(*resultsDir, host)
	if err := os.MkdirAll(logPath, 0755); err != nil {
		return fmt.Errorf("failed to create log directory %q: %v", logPath, err)
	}
	// Copy logs to artifacts/hostname
	if _, err := runSSHCommand("scp", "-r", fmt.Sprintf("%s:%s/results/*.log", GetHostnameOrIP(host), testDir), logPath); err != nil {
		return err
	}
	// Copy json files (if any) to artifacts.
	if _, err := SSH(host, "ls", fmt.Sprintf("%s/results/*.json", testDir)); err == nil {
		if _, err = runSSHCommand("scp", "-r", fmt.Sprintf("%s:%s/results/*.json", GetHostnameOrIP(host), testDir), *resultsDir); err != nil {
			return err
		}
	}
	if _, err := SSH(host, "ls", fmt.Sprintf("%s/results/junit*", testDir)); err == nil {
		// Copy junit (if any) to the top of artifacts
		if _, err = runSSHCommand("scp", fmt.Sprintf("%s:%s/results/junit*", GetHostnameOrIP(host), testDir), *resultsDir); err != nil {
			return err
		}
	}
	return nil
}

// WriteLog is a temporary function to make it possible to write log
// in the runner. This is used to collect serial console log.
// TODO(random-liu): Use the log-dump script in cluster e2e.
func WriteLog(host, filename, content string) error {
	logPath := filepath.Join(*resultsDir, host)
	if err := os.MkdirAll(logPath, 0755); err != nil {
		return fmt.Errorf("failed to create log directory %q: %v", logPath, err)
	}
	f, err := os.Create(filepath.Join(logPath, filename))
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(content)
	return err
}
