/*
Copyright 2025 The Kubernetes Authors.

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

package nodelabeler

import (
	"context"
	"encoding/json"
	"maps"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/constants"
)

const (
	instanceTypeLabel         = "node.kubernetes.io/instance-type"
	machinePDCompatibilityKey = "machine-pd-compatibility.json"
)

// Reconciler reconciles Node objects to add disk type labels based on machine type.
type Reconciler struct {
	Client             client.Client
	ConfigMapName      string
	ConfigMapNamespace string
	mux                sync.RWMutex
	compatibility      map[string][]string
}

func (r *Reconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	node := &corev1.Node{}
	err := r.Client.Get(ctx, request.NamespacedName, node)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	instanceType, ok := node.Labels[instanceTypeLabel]
	if !ok {
		klog.V(4).Infof("Node %q does not have label %q, skipping", node.Name, instanceTypeLabel)
		return reconcile.Result{}, nil
	}

	pds := r.compatiblePDs(instanceType)
	if len(pds) == 0 {
		klog.V(4).Infof("No compatible PDs found for instance type %q on node %q", instanceType, node.Name)
		return reconcile.Result{}, nil
	}

	newLabels := make(map[string]string)
	maps.Copy(newLabels, node.Labels)

	diskTypeLabelKeys := make(map[string]struct{})
	for _, diskType := range pds {
		key := common.DiskTypeLabelKey(diskType)
		diskTypeLabelKeys[key] = struct{}{}
	}

	labelsChanged := false
	for key := range diskTypeLabelKeys {
		if val, ok := newLabels[key]; !ok || val != "true" {
			newLabels[key] = "true"
			labelsChanged = true
		}
	}

	for key := range newLabels {
		if strings.HasPrefix(key, constants.DiskTypeKeyPrefix) {
			if _, isExpected := diskTypeLabelKeys[key]; !isExpected {
				delete(newLabels, key)
				labelsChanged = true
			}
		}
	}

	if !labelsChanged {
		klog.V(4).Infof("Node %q already has the correct PD compatibility labels", node.Name)
		return reconcile.Result{}, nil
	}

	klog.Infof("Updating labels for node %q", node.Name)
	patch := client.MergeFrom(node.DeepCopy())
	node.Labels = newLabels
	if err := r.Client.Patch(ctx, node, patch); err != nil {
		klog.Errorf("Failed to patch node %q: %v", node.Name, err)
		return reconcile.Result{}, err
	}

	klog.Infof("Successfully updated labels for node %q", node.Name)
	return reconcile.Result{}, nil
}

func (r *Reconciler) compatiblePDs(instanceType string) []string {
	r.mux.RLock()
	defer r.mux.RUnlock()

	mf := machineFamily(instanceType)
	if disks, ok := r.compatibility[mf]; ok {
		return disks
	}

	return nil
}

// machineFamily extracts the machine family from the instance type
// e.g. "n2-standard-4" -> "n2", "e2-medium" -> "e2".
func machineFamily(instanceType string) string {
	mf, _, _ := strings.Cut(instanceType, "-")
	return mf
}

// UpdateMachinePDCompatibility updates the controller's machine PD compatibility map from a ConfigMap.
func (r *Reconciler) UpdateMachinePDCompatibility(ctx context.Context) error {
	cm := &corev1.ConfigMap{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: r.ConfigMapName, Namespace: r.ConfigMapNamespace}, cm)
	if err != nil {
		return err
	}
	return r.updateMap(cm)
}

func (r *Reconciler) updateMap(cm *corev1.ConfigMap) error {
	klog.Infof("Updating disk compatibility map from ConfigMap %s/%s", cm.Namespace, cm.Name)
	jsonData, ok := cm.Data[machinePDCompatibilityKey]
	if !ok {
		klog.Warningf("ConfigMap %s/%s does not contain key '%s'", cm.Namespace, cm.Name, machinePDCompatibilityKey)
		return nil
	}

	var parsedData map[string]map[string]bool
	if err := json.Unmarshal([]byte(jsonData), &parsedData); err != nil {
		klog.Errorf("Failed to unmarshal JSON from ConfigMap %s/%s: %v", cm.Namespace, cm.Name, err)
		return err
	}

	newMap := make(map[string][]string)
	for machineFamily, disks := range parsedData {
		var compatibleDisks []string
		for disk, supported := range disks {
			if supported {
				compatibleDisks = append(compatibleDisks, disk)
			}
		}
		newMap[machineFamily] = compatibleDisks
	}

	r.mux.Lock()
	defer r.mux.Unlock()
	r.compatibility = newMap
	klog.Infof("Machine PD compatibility map updated successfully")
	return nil
}
