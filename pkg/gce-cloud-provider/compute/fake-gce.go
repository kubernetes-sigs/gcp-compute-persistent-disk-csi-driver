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

	csi "github.com/container-storage-interface/spec/lib/go/csi/v0"
	"github.com/golang/glog"
	"golang.org/x/net/context"
	computebeta "google.golang.org/api/compute/v0.beta"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
	"k8s.io/kubernetes/pkg/cloudprovider/providers/gce/cloud/meta"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
)

type FakeCloudProvider struct {
	project string
	zone    string

	disks     map[string]*CloudDisk
	instances map[string]*compute.Instance
}

var _ GCECompute = &FakeCloudProvider{}

func FakeCreateCloudProvider(project, zone string) (*FakeCloudProvider, error) {
	return &FakeCloudProvider{
		project:   project,
		zone:      zone,
		disks:     map[string]*CloudDisk{},
		instances: map[string]*compute.Instance{},
	}, nil

}

func (cloud *FakeCloudProvider) ListZones(ctx context.Context, region string) ([]string, error) {
	return []string{cloud.zone, "country-region-fakesecondzone"}, nil
}

// Disk Methods
func (cloud *FakeCloudProvider) GetDisk(ctx context.Context, volKey *meta.Key) (*CloudDisk, error) {
	disk, ok := cloud.disks[volKey.Name]
	if !ok {
		return nil, notFoundError()
	}
	return disk, nil
}

func (cloud *FakeCloudProvider) ValidateExistingDisk(ctx context.Context, resp *CloudDisk, diskType string, reqBytes, limBytes int64) error {
	if resp == nil {
		return fmt.Errorf("disk does not exist")
	}
	requestValid := common.GbToBytes(resp.GetSizeGb()) >= reqBytes || reqBytes == 0
	responseValid := common.GbToBytes(resp.GetSizeGb()) <= limBytes || limBytes == 0
	if !requestValid || !responseValid {
		return fmt.Errorf(
			"disk already exists with incompatible capacity. Need %v (Required) < %v (Existing) < %v (Limit)",
			reqBytes, common.GbToBytes(resp.GetSizeGb()), limBytes)
	}

	respType := strings.Split(resp.GetType(), "/")
	typeMatch := strings.TrimSpace(respType[len(respType)-1]) == strings.TrimSpace(diskType)
	typeDefault := diskType == "" && strings.TrimSpace(respType[len(respType)-1]) == "pd-standard"
	if !typeMatch && !typeDefault {
		return fmt.Errorf("disk already exists with incompatible type. Need %v. Got %v",
			diskType, respType[len(respType)-1])
	}
	glog.V(4).Infof("Compatible disk already exists")
	return nil
}

func (cloud *FakeCloudProvider) InsertDisk(ctx context.Context, volKey *meta.Key, diskType string, capBytes int64, capacityRange *csi.CapacityRange, replicaZones []string) error {
	if disk, ok := cloud.disks[volKey.Name]; ok {
		err := cloud.ValidateExistingDisk(ctx, disk, diskType,
			int64(capacityRange.GetRequiredBytes()),
			int64(capacityRange.GetLimitBytes()))
		if err != nil {
			return err
		}
	}

	var diskToCreate *CloudDisk
	switch volKey.Type() {
	case meta.Zonal:
		diskToCreateGA := &compute.Disk{
			Name:        volKey.Name,
			SizeGb:      common.BytesToGb(capBytes),
			Description: "Disk created by GCE-PD CSI Driver",
			Type:        cloud.GetDiskTypeURI(volKey, diskType),
			SelfLink:    fmt.Sprintf("projects/%s/zones/%s/disks/%s", cloud.project, volKey.Zone, volKey.Name),
		}
		diskToCreate = ZonalCloudDisk(diskToCreateGA)
	case meta.Regional:
		diskToCreateBeta := &computebeta.Disk{
			Name:        volKey.Name,
			SizeGb:      common.BytesToGb(capBytes),
			Description: "Regional disk created by GCE-PD CSI Driver",
			Type:        cloud.GetDiskTypeURI(volKey, diskType),
			SelfLink:    fmt.Sprintf("projects/%s/regions/%s/disks/%s", cloud.project, volKey.Region, volKey.Name),
		}
		diskToCreate = RegionalCloudDisk(diskToCreateBeta)
	default:
		return fmt.Errorf("could not create disk, key was neither zonal nor regional, instead got: %v", volKey.String())
	}

	cloud.disks[volKey.Name] = diskToCreate
	return nil
}

func (cloud *FakeCloudProvider) DeleteDisk(ctx context.Context, volKey *meta.Key) error {
	delete(cloud.disks, volKey.Name)
	return nil
}

func (cloud *FakeCloudProvider) AttachDisk(ctx context.Context, disk *CloudDisk, volKey *meta.Key, readWrite, diskType, instanceZone, instanceName string) error {
	source := cloud.GetDiskSourceURI(disk, volKey)

	attachedDiskV1 := &compute.AttachedDisk{
		DeviceName: disk.GetName(),
		Kind:       disk.GetKind(),
		Mode:       readWrite,
		Source:     source,
		Type:       diskType,
	}
	instance, ok := cloud.instances[instanceName]
	if !ok {
		return fmt.Errorf("Failed to get instance %v", instanceName)
	}
	instance.Disks = append(instance.Disks, attachedDiskV1)
	return nil
}

func (cloud *FakeCloudProvider) DetachDisk(ctx context.Context, volKey *meta.Key, instanceZone, instanceName string) error {
	instance, ok := cloud.instances[instanceName]
	if !ok {
		return fmt.Errorf("Failed to get instance %v", instanceName)
	}
	found := -1
	for i, disk := range instance.Disks {
		if disk.DeviceName == volKey.Name {
			found = i
			break
		}
	}
	instance.Disks[found] = instance.Disks[len(instance.Disks)-1]
	instance.Disks = instance.Disks[:len(instance.Disks)-1]
	return nil
}

func (cloud *FakeCloudProvider) GetDiskSourceURI(disk *CloudDisk, volKey *meta.Key) string {
	return ""
}

func (cloud *FakeCloudProvider) GetDiskTypeURI(volKey *meta.Key, diskType string) string {
	switch volKey.Type() {
	case meta.Zonal:
		return cloud.getZonalDiskTypeURI(volKey.Zone, diskType)
	case meta.Regional:
		return cloud.getRegionalDiskTypeURI(volKey.Region, diskType)
	default:
		return fmt.Sprintf("could not get disk type uri, key was neither zonal nor regional, instead got: %v", volKey.String())
	}
}

func (cloud *FakeCloudProvider) getZonalDiskTypeURI(zone, diskType string) string {
	return fmt.Sprintf(diskTypeURITemplateSingleZone, cloud.project, zone, diskType)
}

func (cloud *FakeCloudProvider) getRegionalDiskTypeURI(region, diskType string) string {
	return fmt.Sprintf(diskTypeURITemplateRegional, cloud.project, region, diskType)
}

func (cloud *FakeCloudProvider) WaitForAttach(ctx context.Context, volKey *meta.Key, instanceZone, instanceName string) error {
	return nil
}

// Regional Disk Methods
func (cloud *FakeCloudProvider) GetReplicaZoneURI(zone string) string {
	return ""
}

// Instance Methods
func (cloud *FakeCloudProvider) InsertInstance(instance *compute.Instance, instanceZone, instanceName string) {
	cloud.instances[instanceName] = instance
	return
}

func (cloud *FakeCloudProvider) GetInstanceOrError(ctx context.Context, instanceZone, instanceName string) (*compute.Instance, error) {
	instance, ok := cloud.instances[instanceName]
	if !ok {
		return nil, notFoundError()
	}
	return instance, nil
}

func notFoundError() *googleapi.Error {
	return &googleapi.Error{
		Errors: []googleapi.ErrorItem{
			{
				Reason: "notFound",
			},
		},
	}
}
