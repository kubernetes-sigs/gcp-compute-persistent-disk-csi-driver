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

	"github.com/golang/glog"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	computebeta "google.golang.org/api/compute/v0.beta"
	compute "google.golang.org/api/compute/v1"
	testutils "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/test/e2e/utils"
	remote "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/test/remote"
)

var (
	project         = flag.String("project", "", "Project to run tests in")
	serviceAccount  = flag.String("service-account", "", "Service account to bring up instance with")
	runInProw       = flag.Bool("run-in-prow", false, "If true, use a Boskos loaned project and special CI service accounts and ssh keys")
	deleteInstances = flag.Bool("delete-instances", false, "Delete the instances after tests run")

	testContexts       = []*remote.TestContext{}
	computeService     *compute.Service
	betaComputeService *computebeta.Service
)

func TestE2E(t *testing.T) {
	flag.Parse()
	RegisterFailHandler(Fail)
	RunSpecs(t, "Google Compute Engine Persistent Disk Container Storage Interface Driver Tests")
}

var _ = BeforeSuite(func() {
	var err error
	tcc := make(chan *remote.TestContext)
	defer close(tcc)

	zones := []string{"us-central1-c", "us-central1-b"}

	rand.Seed(time.Now().UnixNano())

	computeService, err = remote.GetComputeClient()
	Expect(err).To(BeNil())

	betaComputeService, err = remote.GetBetaComputeClient()
	Expect(err).To(BeNil())

	if *runInProw {
		*project, *serviceAccount = testutils.SetupProwConfig("gce-project")
	}

	Expect(*project).ToNot(BeEmpty(), "Project should not be empty")
	Expect(*serviceAccount).ToNot(BeEmpty(), "Service account should not be empty")

	glog.Infof("Running in project %v with service account %v\n\n", *project, *serviceAccount)

	for _, zone := range zones {
		go func(curZone string) {
			defer GinkgoRecover()
			nodeID := fmt.Sprintf("gce-pd-csi-e2e-%s", curZone)
			glog.Infof("Setting up node %s\n", nodeID)

			i, err := remote.SetupInstance(*project, curZone, nodeID, *serviceAccount, computeService)
			if err != nil {
				glog.Fatalf("Failed to setup instance %v: %v", nodeID, err)
			}

			glog.Infof("Creating new driver and client for node %s\n", i.GetName())
			// Create new driver and client
			testContext, err := testutils.GCEClientAndDriverSetup(i)
			if err != nil {
				glog.Fatalf("Failed to set up Test Context for instance %v: %v", i.GetName(), err)
			}
			tcc <- testContext
		}(zone)
	}

	for i := 0; i < len(zones); i++ {
		tc := <-tcc
		glog.Infof("Test Context for node %s set up\n", tc.Instance.GetName())
		testContexts = append(testContexts, tc)
	}
})

var _ = AfterSuite(func() {

	for _, tc := range testContexts {
		err := remote.TeardownDriverAndClient(tc)
		Expect(err).To(BeNil(), "Teardown Driver and Client failed with error")
		if *deleteInstances {
			tc.Instance.DeleteInstance()
		}
	}
})

func getRandomTestContext() *remote.TestContext {
	Expect(testContexts).ToNot(BeEmpty())
	rn := rand.Intn(len(testContexts))
	return testContexts[rn]
}
