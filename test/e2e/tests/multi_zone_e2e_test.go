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
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
	testutils "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/test/e2e/utils"
)

var _ = Describe("GCE PD CSI Driver Multi-Zone", func() {
	BeforeEach(func() {
		Expect(len(testInstances)).To(BeNumerically(">", 1))
		// TODO: Check whether the instances are in different zones???
		// I Think there should be a better way of guaranteeing this. Like a map from zone to instance for testInstances (?)
	})

	It("Should get reasonable topology from nodes with NodeGetInfo", func() {
		for _, instance := range testInstances {
			testContext, err := testutils.SetupNewDriverAndClient(instance)
			Expect(err).To(BeNil(), "Set up new Driver and Client failed with error")

			resp, err := testContext.Client.NodeGetInfo()
			Expect(err).To(BeNil())

			// Get Cloud Instance
			p, z, n := testContext.Instance.GetIdentity()
			cloudInstance, err := computeService.Instances.Get(p, z, n).Do()
			Expect(err).To(BeNil())
			Expect(cloudInstance).ToNot(BeNil())

			// Check topology matches
			segments := resp.GetAccessibleTopology().GetSegments()
			Expect(segments).ToNot(BeNil())

			Expect(segments[common.TopologyKeyZone]).To(Equal(z))
			Expect(len(segments)).To(Equal(1))
		}

	})

})
