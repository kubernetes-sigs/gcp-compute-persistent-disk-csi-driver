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

package main

import (
	"flag"
	"os"

	"github.com/golang/glog"

	gce "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider"
	driver "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-csi-driver"
	mountmanager "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/mount-manager"
)

func init() {
	flag.Set("logtostderr", "true")
}

var (
	endpoint   = flag.String("endpoint", "unix://tmp/csi.sock", "CSI endpoint")
	driverName = flag.String("drivername", "com.google.csi.gcepd", "name of the driver")
	nodeID     = flag.String("nodeid", "", "node id")
)

func main() {
	flag.Parse()

	handle()
	os.Exit(0)
}

func handle() {
	gceDriver := driver.GetGCEDriver()

	//Initialize GCE Driver (Move setup to main?)
	cloudProvider, err := gce.CreateCloudProvider()
	if err != nil {
		glog.Fatalf("Failed to get cloud provider: %v", err)
	}

	mounter, err := mountmanager.CreateMounter()
	if err != nil {
		glog.Fatalf("Failed to get mounter: %v", err)
	}

	err = gceDriver.SetupGCEDriver(cloudProvider, mounter, *driverName, *nodeID)
	if err != nil {
		glog.Fatalf("Failed to initialize GCE CSI Driver: %v", err)
	}

	gceDriver.Run(*endpoint)
}
