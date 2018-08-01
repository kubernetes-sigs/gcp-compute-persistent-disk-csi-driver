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

package tests

import (
	"flag"
	"fmt"
	"math/rand"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	compute "google.golang.org/api/compute/v1"
	testutils "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/test/e2e/utils"
	remote "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/test/remote"
)

var (
	project         = flag.String("project", "", "Project to run tests in")
	serviceAccount  = flag.String("service-account", "", "Service account to bring up instance with")
	runInProw       = flag.Bool("run-in-prow", false, "If true, use a Boskos loaned project and special CI service accounts and ssh keys")
	deleteInstances = flag.Bool("delete-instances", false, "Delete the instances after tests run")

	testInstances  = []*remote.InstanceInfo{}
	computeService *compute.Service
)

func TestE2E(t *testing.T) {
	flag.Parse()
	RegisterFailHandler(Fail)
	RunSpecs(t, "Google Compute Engine Persistent Disk Container Storage Interface Driver Tests")
}

var _ = BeforeSuite(func() {
	var err error
	zones := []string{"us-central1-c", "us-central1-b"}

	rand.Seed(time.Now().UnixNano())

	computeService, err = remote.GetComputeClient()
	Expect(err).To(BeNil())

	for _, zone := range zones {
		nodeID := fmt.Sprintf("gce-pd-csi-e2e-%s", zone)

		if *runInProw {
			*project, *serviceAccount = testutils.SetupProwConfig()
		}

		Expect(*project).ToNot(BeEmpty(), "Project should not be empty")
		Expect(*serviceAccount).ToNot(BeEmpty(), "Service account should not be empty")

		i, err := remote.SetupInstance(*project, zone, nodeID, *serviceAccount, computeService)
		Expect(err).To(BeNil())

		testInstances = append(testInstances, i)
	}

})

var _ = AfterSuite(func() {
	/*
		err := node.client.CloseConn()
		if err != nil {
			Logf("Failed to close the client")
		} else {
			Logf("Closed the client")
	*/
	for _, i := range testInstances {
		if *deleteInstances {
			i.DeleteInstance()
		}
	}
})
