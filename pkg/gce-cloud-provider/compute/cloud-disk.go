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
	"strings"

	computev1 "google.golang.org/api/compute/v1"
)

type CloudDisk struct {
	ZonalDisk    *computev1.Disk
	RegionalDisk *computev1.Disk
}

type CloudDiskType string

const (
	// Zonal key type.
	Zonal = "zonal"
	// Regional key type.
	Regional = "regional"
	// Global key type.
	Global = "global"
)

func ZonalCloudDisk(disk *computev1.Disk) *CloudDisk {
	return &CloudDisk{
		ZonalDisk: disk,
	}
}

func RegionalCloudDisk(disk *computev1.Disk) *CloudDisk {
	return &CloudDisk{
		RegionalDisk: disk,
	}
}

func (d *CloudDisk) Type() CloudDiskType {
	switch {
	case d.ZonalDisk != nil:
		return Zonal
	case d.RegionalDisk != nil:
		return Regional
	default:
		return Global
	}
}

func (d *CloudDisk) GetUsers() []string {
	switch d.Type() {
	case Zonal:
		return d.ZonalDisk.Users
	case Regional:
		return d.RegionalDisk.Users
	default:
		return nil
	}
}

func (d *CloudDisk) GetName() string {
	switch d.Type() {
	case Zonal:
		return d.ZonalDisk.Name
	case Regional:
		return d.RegionalDisk.Name
	default:
		return ""
	}
}

func (d *CloudDisk) GetKind() string {
	switch d.Type() {
	case Zonal:
		return d.ZonalDisk.Kind
	case Regional:
		return d.RegionalDisk.Kind
	default:
		return ""
	}
}

func (d *CloudDisk) GetStatus() string {
	switch d.Type() {
	case Zonal:
		return d.ZonalDisk.Status
	case Regional:
		return d.RegionalDisk.Status
	default:
		return "Unknown"
	}
}

// GetPDType returns the type of the PD as either 'pd-standard' or 'pd-ssd' The
// "Type" field on the compute disk is stored as a url like
// projects/project/zones/zone/diskTypes/pd-standard
func (d *CloudDisk) GetPDType() string {
	var pdType string
	switch d.Type() {
	case Zonal:
		pdType = d.ZonalDisk.Type
	case Regional:
		pdType = d.RegionalDisk.Type
	default:
		return ""
	}
	respType := strings.Split(pdType, "/")
	return strings.TrimSpace(respType[len(respType)-1])
}

func (d *CloudDisk) GetSelfLink() string {
	switch d.Type() {
	case Zonal:
		return d.ZonalDisk.SelfLink
	case Regional:
		return d.RegionalDisk.SelfLink
	default:
		return ""
	}
}

func (d *CloudDisk) GetSizeGb() int64 {
	switch d.Type() {
	case Zonal:
		return d.ZonalDisk.SizeGb
	case Regional:
		return d.RegionalDisk.SizeGb
	default:
		return -1
	}
}

// setSizeGb sets the size of the disk used ONLY
// for testing purposes.
func (d *CloudDisk) setSizeGb(size int64) {
	switch d.Type() {
	case Zonal:
		d.ZonalDisk.SizeGb = size
	case Regional:
		d.RegionalDisk.SizeGb = size
	}
}

func (d *CloudDisk) GetZone() string {
	switch d.Type() {
	case Zonal:
		return d.ZonalDisk.Zone
	case Regional:
		return d.RegionalDisk.Zone
	default:
		return ""
	}
}

func (d *CloudDisk) GetSnapshotId() string {
	switch d.Type() {
	case Zonal:
		return d.ZonalDisk.SourceSnapshotId
	case Regional:
		return d.RegionalDisk.SourceSnapshotId
	default:
		return ""
	}
}

func (d *CloudDisk) GetKMSKeyName() string {
	var dek *computev1.CustomerEncryptionKey
	switch d.Type() {
	case Zonal:
		dek = d.ZonalDisk.DiskEncryptionKey
	case Regional:
		dek = d.RegionalDisk.DiskEncryptionKey
	default:
		return ""
	}
	if dek == nil {
		return ""
	}
	return dek.KmsKeyName
}
