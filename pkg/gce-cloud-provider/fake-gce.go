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

package gcecloudprovider

import (
	"fmt"
	"strings"

	"github.com/golang/glog"
	"github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pkg/utils"
	"golang.org/x/net/context"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type FakeCloudProvider struct {
	project string
	zone    string

	disks     map[string]*compute.Disk
	instances map[string]*compute.Instance
}

func FakeCreateCloudProvider(project, zone string) (*FakeCloudProvider, error) {
	return &FakeCloudProvider{
		project:   project,
		zone:      zone,
		disks:     map[string]*compute.Disk{},
		instances: map[string]*compute.Instance{},
	}, nil

}

// Getters
func (cloud *FakeCloudProvider) GetProject() (string, error) {
	return cloud.project, nil
}
func (cloud *FakeCloudProvider) GetZone() (string, error) {
	return cloud.zone, nil
}

// Disk Methods
func (cloud *FakeCloudProvider) GetDiskOrError(ctx context.Context, volumeZone, volumeName string) (*compute.Disk, error) {
	disk, ok := cloud.disks[volumeName]
	if !ok {
		return nil, fmt.Errorf("Disk %v not found", volumeName)
	}
	return disk, nil
}

func (cloud *FakeCloudProvider) GetAndValidateExistingDisk(ctx context.Context, configuredZone, name, diskType string, reqBytes, limBytes int64) (exists bool, err error) {
	disk, ok := cloud.disks[name]
	if !ok {
		// Disk doesn't exist
		return false, nil
	}
	if disk != nil {
		// Check that disk is the same
		requestValid := utils.GbToBytes(disk.SizeGb) >= reqBytes || reqBytes == 0
		responseValid := utils.GbToBytes(disk.SizeGb) <= limBytes || limBytes == 0
		if !requestValid || !responseValid {
			return true, status.Error(codes.AlreadyExists, fmt.Sprintf(
				"Disk already exists with incompatible capacity. Need %v (Required) < %v (Existing) < %v (Limit)",
				reqBytes, utils.GbToBytes(disk.SizeGb), limBytes))
		}

		respType := strings.Split(disk.Type, "/")
		typeMatch := respType[len(respType)-1] != diskType
		typeDefault := diskType == "" && respType[len(respType)-1] == "pd-standard"
		if !typeMatch && !typeDefault {
			return true, status.Error(codes.AlreadyExists, fmt.Sprintf(
				"Disk already exists with incompatible type. Need %v. Got %v",
				diskType, respType[len(respType)-1]))
		}

		// Volume exists with matching name, capacity, type.
		glog.Infof("Compatible disk already exists. Reusing existing.")
		return true, nil
	}

	return false, nil
}

func (cloud *FakeCloudProvider) InsertDisk(ctx context.Context, zone string, diskToCreate *compute.Disk) (*compute.Operation, error) {
	cloud.disks[diskToCreate.Name] = diskToCreate
	return &compute.Operation{}, nil
}

func (cloud *FakeCloudProvider) DeleteDisk(ctx context.Context, zone, name string) (*compute.Operation, error) {
	delete(cloud.disks, name)
	return &compute.Operation{}, nil
}

func (cloud *FakeCloudProvider) AttachDisk(ctx context.Context, zone, instanceName string, attachedDisk *compute.AttachedDisk) (*compute.Operation, error) {
	instance, ok := cloud.instances[instanceName]
	if !ok {
		return nil, fmt.Errorf("Failed to get instance %v", instanceName)
	}
	instance.Disks = append(instance.Disks, attachedDisk)
	return nil, nil
}

func (cloud *FakeCloudProvider) DetachDisk(ctx context.Context, volumeZone, instanceName, volumeName string) (*compute.Operation, error) {
	instance, ok := cloud.instances[instanceName]
	if !ok {
		return nil, fmt.Errorf("Failed to get instance %v", instanceName)
	}
	found := -1
	for i, disk := range instance.Disks {
		if disk.DeviceName == volumeName {
			found = i
			break
		}
	}
	instance.Disks[found] = instance.Disks[len(instance.Disks)-1]
	instance.Disks = instance.Disks[:len(instance.Disks)-1]
	return nil, nil
}

/*
func (cloud *CloudProvider) GetDiskSourceURI(disk *compute.Disk, zone string) string {
	projectsApiEndpoint := gceComputeAPIEndpoint + "projects/"
	if cloud.service != nil {
		projectsApiEndpoint = cloud.service.BasePath
	}

	return projectsApiEndpoint + fmt.Sprintf(
		diskSourceURITemplateSingleZone,
		cloud.project,
		zone,
		disk.Name)
}

func (cloud *CloudProvider) GetDiskTypeURI(zone, diskType string) string {
	return fmt.Sprintf(diskTypeURITemplateSingleZone, cloud.project, zone, diskType)
}
*/
func (cloud *FakeCloudProvider) GetDiskSourceURI(disk *compute.Disk, zone string) string {
	return ""
}

func (cloud *FakeCloudProvider) GetDiskTypeURI(zone, diskType string) string {
	return ""
}

// Instance Methods
func (cloud *FakeCloudProvider) InsertInstance(instance *compute.Instance, instanceName string) {
	cloud.instances[instanceName] = instance
	return
}

func (cloud *FakeCloudProvider) GetInstanceOrError(ctx context.Context, instanceZone, instanceName string) (*compute.Instance, error) {
	instance, ok := cloud.instances[instanceName]
	if !ok {
		return nil, fmt.Errorf("Could not find instance %v", instanceName)
	}
	return instance, nil
}

// Operation Methods
func (cloud *FakeCloudProvider) WaitForOp(ctx context.Context, op *compute.Operation, zone string) error {
	return nil
}
