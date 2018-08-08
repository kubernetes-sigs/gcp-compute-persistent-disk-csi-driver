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
	computebeta "google.golang.org/api/compute/v0.beta"
	compute "google.golang.org/api/compute/v1"
)

type CloudDisk struct {
	ZonalDisk    *compute.Disk
	RegionalDisk *computebeta.Disk
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

func ZonalCloudDisk(disk *compute.Disk) *CloudDisk {
	return &CloudDisk{
		ZonalDisk: disk,
	}
}

func RegionalCloudDisk(disk *computebeta.Disk) *CloudDisk {
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

func (d *CloudDisk) GetType() string {
	switch d.Type() {
	case Zonal:
		return d.ZonalDisk.Type
	case Regional:
		return d.RegionalDisk.Type
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
