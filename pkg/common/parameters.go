/*
Copyright 2020 The Kubernetes Authors.

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

package common

import (
	"fmt"
	"strings"
)

const (
	ParameterKeyType                 = "type"
	ParameterKeyReplicationType      = "replication-type"
	ParameterKeyDiskEncryptionKmsKey = "disk-encryption-kms-key"

	replicationTypeNone = "none"

	// Keys for PV and PVC parameters as reported by external-provisioner
	ParameterKeyPVCName      = "csi.storage.k8s.io/pvc/name"
	ParameterKeyPVCNamespace = "csi.storage.k8s.io/pvc/namespace"
	ParameterKeyPVName       = "csi.storage.k8s.io/pv/name"

	// Keys for tags to attach to the provisioned disk.
	tagKeyCreatedForClaimNamespace = "kubernetes.io/created-for/pvc/namespace"
	tagKeyCreatedForClaimName      = "kubernetes.io/created-for/pvc/name"
	tagKeyCreatedForVolumeName     = "kubernetes.io/created-for/pv/name"
	tagKeyCreatedBy                = "storage.gke.io/created-by"
)

// DiskParameters contains normalized and defaulted disk parameters
type DiskParameters struct {
	// Values: pd-standard, pd-balanced, pd-ssd, or any other PD disk type. Not validated.
	// Default: pd-standard
	DiskType string
	// Values: "none", regional-pd
	// Default: "none"
	ReplicationType string
	// Values: {string}
	// Default: ""
	DiskEncryptionKMSKey string
	// Values: {map[string]string}
	// Default: ""
	Tags map[string]string
	// Values: {map[string]string}
	// Default: ""
	Labels map[string]string
}

// ExtractAndDefaultParameters will take the relevant parameters from a map and
// put them into a well defined struct making sure to default unspecified fields
func ExtractAndDefaultParameters(parameters map[string]string, driverName string, extraVolumeLabels map[string]string) (DiskParameters, error) {
	p := DiskParameters{
		DiskType:             "pd-standard",           // Default
		ReplicationType:      replicationTypeNone,     // Default
		DiskEncryptionKMSKey: "",                      // Default
		Tags:                 make(map[string]string), // Default
		Labels:               make(map[string]string), // Default
	}

	for k, v := range parameters {
		if k == "csiProvisionerSecretName" || k == "csiProvisionerSecretNamespace" {
			// These are hardcoded secrets keys required to function but not needed by GCE PD
			continue
		}
		switch strings.ToLower(k) {
		case ParameterKeyType:
			if v != "" {
				p.DiskType = strings.ToLower(v)
			}
		case ParameterKeyReplicationType:
			if v != "" {
				p.ReplicationType = strings.ToLower(v)
			}
		case ParameterKeyDiskEncryptionKmsKey:
			// Resource names (e.g. "keyRings", "cryptoKeys", etc.) are case sensitive, so do not change case
			p.DiskEncryptionKMSKey = v
		case ParameterKeyPVCName:
			p.Tags[tagKeyCreatedForClaimName] = v
		case ParameterKeyPVCNamespace:
			p.Tags[tagKeyCreatedForClaimNamespace] = v
		case ParameterKeyPVName:
			p.Tags[tagKeyCreatedForVolumeName] = v
		default:
			return p, fmt.Errorf("parameters contains invalid option %q", k)
		}
	}
	if len(p.Tags) > 0 {
		p.Tags[tagKeyCreatedBy] = driverName
	}
	for k, v := range extraVolumeLabels {
		p.Labels[k] = v
	}
	return p, nil
}
