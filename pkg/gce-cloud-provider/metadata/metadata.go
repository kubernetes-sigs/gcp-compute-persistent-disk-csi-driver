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

package metadata

import (
	"fmt"
	"strings"

	"cloud.google.com/go/compute/metadata"
)

// MetadataService is a fakeable interface exposing necessary data
// from the GCE Metadata service
type MetadataService interface {
	GetZone() string
	GetProject() string
	GetName() string
	GetMachineType() string
}

type metadataServiceManager struct {
	// Current zone the driver is running in
	zone        string
	project     string
	name        string
	machineType string
}

var _ MetadataService = &metadataServiceManager{}

func NewMetadataService() (MetadataService, error) {
	zone, err := metadata.Zone()
	if err != nil {
		return nil, fmt.Errorf("failed to get current zone: %v", err)
	}
	projectID, err := metadata.ProjectID()
	if err != nil {
		return nil, fmt.Errorf("failed to get project: %v", err)
	}
	name, err := metadata.InstanceName()
	if err != nil {
		return nil, fmt.Errorf("failed to get instance name: %v", err)
	}
	fullMachineType, err := metadata.Get("instance/machine-type")
	if err != nil {
		return nil, fmt.Errorf("failed to get machine-type: %v", err)
	}
	// Response format: "projects/[NUMERIC_PROJECT_ID]/machineTypes/[MACHINE_TYPE]"
	splits := strings.Split(fullMachineType, "/")
	machineType := splits[len(splits)-1]

	return &metadataServiceManager{
		project:     projectID,
		zone:        zone,
		name:        name,
		machineType: machineType,
	}, nil
}

func (manager *metadataServiceManager) GetZone() string {
	return manager.zone
}

func (manager *metadataServiceManager) GetProject() string {
	return manager.project
}

func (manager *metadataServiceManager) GetName() string {
	return manager.name
}

func (manager *metadataServiceManager) GetMachineType() string {
	return manager.machineType
}
