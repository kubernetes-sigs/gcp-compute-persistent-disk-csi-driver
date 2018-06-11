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
	// Make sure we can find the newly built binaries
	gopath, ok := os.LookupEnv("GOPATH")
	if !ok {
		return fmt.Errorf("Could not find gopath")
	}

	// TODO(dyzz): build the gce driver instead.

	buildOutputDir := filepath.Join(gopath, "src/github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/bin")

	// Copy binaries
	requiredBins := []string{"gce-csi-driver", "gce-csi-driver-test"}
	for _, bin := range requiredBins {
		source := filepath.Join(buildOutputDir, bin)
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
		fmt.Sprintf("./gce-csi-driver-test"),
	)
	return SSH(host, "sh", "-c", cmd)
}
