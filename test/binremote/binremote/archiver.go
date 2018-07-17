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

package binremote

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"

	"github.com/golang/glog"
)

func CreateDriverArchive(archiveName string) (string, error) {
	glog.V(2).Infof("Building archive...")
	tarDir, err := ioutil.TempDir("", "gce-pd-archive")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary directory %v", err)
	}
	defer os.RemoveAll(tarDir)

	// Call the suite function to setup the test package.
	err = setupBinaries(tarDir)
	if err != nil {
		return "", fmt.Errorf("failed to setup test package %q: %v", tarDir, err)
	}

	// Build the tar
	out, err := exec.Command("tar", "-zcvf", archiveName, "-C", tarDir, ".").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to build tar %v.  Output:\n%s", err, out)
	}

	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory %v", err)
	}
	return filepath.Join(dir, archiveName), nil
}

func setupBinaries(tarDir string) error {
	// TODO(dyzz): build the gce driver tests instead
	goPath, ok := os.LookupEnv("GOPATH")
	if !ok {
		return fmt.Errorf("Could not find environment variable GOPATH")
	}
	binPath := path.Join(goPath, "src/sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/bin/gce-pd-csi-driver")

	// TODO make the driver but from the base dir
	out, err := exec.Command("go", "build", "-ldflags", "-X main.vendorVersion=latest", "-o", binPath, "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/cmd").CombinedOutput()
	if err != nil {
		return fmt.Errorf("Failed to make gce-pd-driver: %v: %v", string(out), err)
	}

	// Copy binaries

	if _, err := os.Stat(binPath); err != nil {
		return fmt.Errorf("failed to locate test binary %s: %v", "gce-pd-csi-driver", err)
	}
	out, err = exec.Command("cp", binPath, filepath.Join(tarDir, "gce-pd-csi-driver")).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to copy %q: %v Output: %q", "gce-pd-csi-driver", err, out)
	}

	return nil
}
