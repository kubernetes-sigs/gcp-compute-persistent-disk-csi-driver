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
	"fmt"

	volumeapiv1 "github.com/kubernetes-csi/csi-proxy/client/api/volume/v1"
	volumeapiv1beta1 "github.com/kubernetes-csi/csi-proxy/client/api/volume/v1beta1"
	"k8s.io/mount-utils"
)

var _ Statter = &realStatter{}

type realStatter struct {
	mounter *mount.SafeFormatAndMount
}

func NewStatter(mounter *mount.SafeFormatAndMount) *realStatter {
	return &realStatter{mounter: mounter}
}

// IsBlock checks if the given path is a block device
func (r *realStatter) IsBlockDevice(fullPath string) (bool, error) {
	return false, nil
}

// StatFS returns volume usage information
func (r *realStatter) StatFS(path string) (available, capacity, used, inodesFree, inodes, inodesUsed int64, err error) {
	switch r.mounter.Interface.(type) {
	case *CSIProxyMounterV1:
		return r.StatFSV1(path)
	case *CSIProxyMounterV1Beta:
		return r.StatFSV1Beta(path)
	}

	return 0, 0, 0, 0, 0, 0, fmt.Errorf("Invalid interface type=%v", r.mounter.Interface)
}

func (r *realStatter) StatFSV1(path string) (available, capacity, used, inodesFree, inodes, inodesUsed int64, err error) {
	zero := int64(0)

	proxy := r.mounter.Interface.(*CSIProxyMounterV1)
	idRequest := &volumeapiv1.GetVolumeIDFromTargetPathRequest{
		TargetPath: path,
	}
	idResponse, err := proxy.VolumeClient.GetVolumeIDFromTargetPath(context.Background(), idRequest)
	if err != nil {
		return zero, zero, zero, zero, zero, zero, err
	}
	volumeID := idResponse.GetVolumeId()

	request := &volumeapiv1.GetVolumeStatsRequest{
		VolumeId: volumeID,
	}
	response, err := proxy.VolumeClient.GetVolumeStats(context.Background(), request)
	if err != nil {
		return zero, zero, zero, zero, zero, zero, err
	}
	capacity = response.GetTotalBytes()
	used = response.GetUsedBytes()
	available = capacity - used
	return available, capacity, used, zero, zero, zero, nil
}

func (r *realStatter) StatFSV1Beta(path string) (available, capacity, used, inodesFree, inodes, inodesUsed int64, err error) {
	zero := int64(0)

	proxy := r.mounter.Interface.(*CSIProxyMounterV1Beta)

	idRequest := &volumeapiv1beta1.VolumeIDFromMountRequest{
		Mount: path,
	}
	idResponse, err := proxy.VolumeClient.GetVolumeIDFromMount(context.Background(), idRequest)
	if err != nil {
		return zero, zero, zero, zero, zero, zero, err
	}
	volumeID := idResponse.GetVolumeId()

	request := &volumeapiv1beta1.VolumeStatsRequest{
		VolumeId: volumeID,
	}
	response, err := proxy.VolumeClient.VolumeStats(context.Background(), request)
	if err != nil {
		return zero, zero, zero, zero, zero, zero, err
	}
	capacity = response.GetVolumeSize()
	used = response.GetVolumeUsedSize()
	available = capacity - used
	return available, capacity, used, zero, zero, zero, nil
}

type fakeStatter struct{}

func NewFakeStatter(mounter *mount.SafeFormatAndMount) *fakeStatter {
	return &fakeStatter{}
}

func (*fakeStatter) StatFS(path string) (available, capacity, used, inodesFree, inodes, inodesUsed int64, err error) {
	// Assume the file exists and give some dummy values back
	return 1, 1, 1, 1, 1, 1, nil
}

func (*fakeStatter) IsBlockDevice(fullPath string) (bool, error) {
	return false, nil
}
