/*
Copyright 2016 Google

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
	"path/filepath"
	"time"

	"github.com/golang/glog"
)

// E2ERemote is type for GCE PD CSI Driver Remote E2E Tests
type E2ERemote struct{}

// InitE2ERemote initializes the GCE PD CSI Driver remote E2E suite
func InitE2ERemote() TestSuite {
	return &E2ERemote{}
}

// SetupTestPackage sets up the test package with binaries k8s required for node e2e tests
func (n *E2ERemote) SetupTestPackage(tardir string) error {
	// TODO(dyzz): build the gce driver tests instead.

	cmd := exec.Command("go", "test", "-c", "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/test/e2e", "-o", "test/e2e/e2e.test")
	err := cmd.Run()

	if err != nil {
		return fmt.Errorf("Failed to build test: %v", err)
	}

	cmd = exec.Command("mkdir", "-p", "bin")
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("Failed to mkdir bin/: %v", err)
	}

	cmd = exec.Command("cp", "test/e2e/e2e.test", "bin/e2e.test")
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("Failed to copy: %v", err)
	}
	// Copy binaries
	requiredBins := []string{"e2e.test"}
	for _, bin := range requiredBins {
		source := filepath.Join("bin", bin)
		if _, err := os.Stat(source); err != nil {
			return fmt.Errorf("failed to locate test binary %s: %v", bin, err)
		}
		out, err := exec.Command("cp", source, filepath.Join(tardir, bin)).CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to copy %q: %v Output: %q", bin, err, out)
		}
	}

	return nil
}

// RunTest runs test on the node.
func (n *E2ERemote) RunTest(host, workspace, results, testArgs, ginkgoArgs string, timeout time.Duration) (string, error) {
	glog.V(2).Infof("Starting tests on %q", host)
	cmd := getSSHCommand(" && ",
		fmt.Sprintf("cd %s", workspace),
		fmt.Sprintf("./e2e.test"),
	)
	return SSH(host, "sh", "-c", cmd)
}
