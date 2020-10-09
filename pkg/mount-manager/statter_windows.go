// +build windows
/*
Copyright 2019 The Kubernetes Authors.

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

package mountmanager

import (
	"context"

	volumeapi "github.com/kubernetes-csi/csi-proxy/client/api/volume/v1beta1"
	volumeclient "github.com/kubernetes-csi/csi-proxy/client/groups/volume/v1beta1"
)

var _ Statter = realStatter{}

type realStatter struct {
	VolumeClient *volumeclient.Client
}

func NewStatter() (realStatter, error) {
	volumeClient, err := volumeclient.NewClient()
	if err != nil {
		return realStatter{}, err
	}
	return realStatter{
		VolumeClient: volumeClient,
	}, nil
}

// IsBlock checks if the given path is a block device
func (realStatter) IsBlockDevice(fullPath string) (bool, error) {
	return false, nil
}

// StatFS returns volume usage information
func (r realStatter) StatFS(path string) (available, capacity, used, inodesFree, inodes, inodesUsed int64, err error) {
	zero := int64(0)

	idRequest := &volumeapi.VolumeIDFromMountRequest{
		Mount: path,
	}
	idResponse, err := r.VolumeClient.GetVolumeIDFromMount(context.Background(), idRequest)
	if err != nil {
		return zero, zero, zero, zero, zero, zero, err
	}
	volumeId := idResponse.GetVolumeId()

	request := &volumeapi.VolumeStatsRequest{
		VolumeId: volumeId,
	}
	response, err := r.VolumeClient.VolumeStats(context.Background(), request)
	if err != nil {
		return zero, zero, zero, zero, zero, zero, err
	}
	capacity = response.GetVolumeSize()
	used = response.GetVolumeUsedSize()
	available = capacity - used
	return available, capacity, used, zero, zero, zero, nil
}

type fakeStatter struct{}

func NewFakeStatter() fakeStatter {
	return fakeStatter{}
}

func (fakeStatter) StatFS(path string) (available, capacity, used, inodesFree, inodes, inodesUsed int64, err error) {
	// Assume the file exists and give some dummy values back
	return 1, 1, 1, 1, 1, 1, nil
}

func (fakeStatter) IsBlockDevice(fullPath string) (bool, error) {
	return false, nil
}
