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
	computealpha "google.golang.org/api/compute/v0.alpha"
	computev1 "google.golang.org/api/compute/v1"
)

type CloudDisk struct {
	ZonalDisk         *computev1.Disk
	RegionalDisk      *computev1.Disk
	ZonalAlphaDisk    *computealpha.Disk
	RegionalAlphaDisk *computealpha.Disk
}

type CloudDiskType string

const (
	// Zonal key type.
	Zonal = "zonal"
	// Regional key type.
	Regional = "regional"
	// ZonalAlpha key type.
	ZonalAlpha = "zonalAlpha"
	// RegionalAlpha key type.
	RegionalAlpha = "regionalAlpha"
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

func ZonalAlphaCloudDisk(disk *computealpha.Disk) *CloudDisk {
	return &CloudDisk{
		ZonalAlphaDisk: disk,
	}
}

func RegionalAlphaCloudDisk(disk *computealpha.Disk) *CloudDisk {
	return &CloudDisk{
		RegionalAlphaDisk: disk,
	}
}

func (d *CloudDisk) Type() CloudDiskType {
	switch {
	case d.ZonalDisk != nil:
		return Zonal
	case d.RegionalDisk != nil:
		return Regional
	case d.ZonalAlphaDisk != nil:
		return ZonalAlpha
	case d.RegionalAlphaDisk != nil:
		return RegionalAlpha
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
	case ZonalAlpha:
		return d.ZonalAlphaDisk.Users
	case RegionalAlpha:
		return d.RegionalAlphaDisk.Users
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
	case ZonalAlpha:
		return d.ZonalAlphaDisk.Name
	case RegionalAlpha:
		return d.RegionalAlphaDisk.Name
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
	case ZonalAlpha:
		return d.ZonalAlphaDisk.Kind
	case RegionalAlpha:
		return d.RegionalAlphaDisk.Kind
	default:
		return ""
	}
}

func (d *CloudDisk) GetType() string {
	switch d.Type() {
	case Zonal:
		return d.ZonalDisk.Type
	case Regional:
		return d.RegionalDisk.Type
	case ZonalAlpha:
		return d.ZonalAlphaDisk.Type
	case RegionalAlpha:
		return d.RegionalAlphaDisk.Type
	default:
		return ""
	}
}

func (d *CloudDisk) GetSelfLink() string {
	switch d.Type() {
	case Zonal:
		return d.ZonalDisk.SelfLink
	case Regional:
		return d.RegionalDisk.SelfLink
	case ZonalAlpha:
		return d.ZonalAlphaDisk.SelfLink
	case RegionalAlpha:
		return d.RegionalAlphaDisk.SelfLink
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
	case ZonalAlpha:
		return d.ZonalAlphaDisk.SizeGb
	case RegionalAlpha:
		return d.RegionalAlphaDisk.SizeGb
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
	case ZonalAlpha:
		d.ZonalAlphaDisk.SizeGb = size
	case RegionalAlpha:
		d.RegionalAlphaDisk.SizeGb = size
	}
}

func (d *CloudDisk) GetZone() string {
	switch d.Type() {
	case Zonal:
		return d.ZonalDisk.Zone
	case Regional:
		return d.RegionalDisk.Zone
	case ZonalAlpha:
		return d.ZonalAlphaDisk.Zone
	case RegionalAlpha:
		return d.RegionalAlphaDisk.Zone
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
	case ZonalAlpha:
		return d.ZonalAlphaDisk.SourceSnapshotId
	case RegionalAlpha:
		return d.RegionalAlphaDisk.SourceSnapshotId
	default:
		return ""
	}
}

func (d *CloudDisk) GetMultiWriter() bool {
	switch d.Type() {
	case Zonal:
		return false
	case Regional:
		return false
	case ZonalAlpha:
		return d.ZonalAlphaDisk.MultiWriter
	case RegionalAlpha:
		return d.RegionalAlphaDisk.MultiWriter
	default:
		return false
	}
}
