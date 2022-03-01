/*
Copyright 2022 The Kubernetes Authors.

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

package gceGCEDriver

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/GoogleCloudPlatform/k8s-cloud-provider/pkg/cloud/meta"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
	gce "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/compute"
)

var (
	backoff = wait.Backoff{
		Steps:    300,
		Duration: 1 * time.Second,
		Factor:   1.0,
	}
)

type AttachDiskOpts struct {
	Volumekey    *meta.Key
	ReadWrite    string
	DiskType     string
	Project      string
	Location     string
	DeviceName   string
	InstanceName string
}

type DetachDiskOpts struct {
	Project      string
	Location     string
	DeviceName   string
	InstanceName string
}

type OpsManager struct {
	sync.Mutex         // lock to perfom CRUD operations on the cache in a thread safe manner.
	ready         bool // Ops manager is ready when the cache is ready
	opsCache      *common.OpsCache
	cloudProvider gce.GCECompute
}

func NewOpsManager(c gce.GCECompute) *OpsManager {
	return &OpsManager{
		opsCache:      common.NewOpsCache(),
		cloudProvider: c,
	}
}

// ExecuteAttachDisk verifies the cache content before starting an attach disk operation, and then clears the cache if the operation completes successfully.
func (o *OpsManager) ExecuteAttachDisk(ctx context.Context, opts *AttachDiskOpts) error {
	// Acquire the ops cache lock, verify ops and proceed to start attach disk operation.
	op, err := o.checkCacheAndStartAttachDiskOp(ctx, opts.Volumekey, opts.ReadWrite, opts.DiskType, opts.Project, opts.Location, opts.DeviceName, opts.InstanceName)
	if err != nil {
		return err
	}

	// Lock released, wait for the op to complete.
	err = o.cloudProvider.WaitForZonalOp(ctx, opts.Project, op.Name, opts.Location)
	if err != nil {
		return status.Error(codes.Internal, fmt.Sprintf("%v", err))
	}

	// Op succeeded, acquire cache lock and clear op.
	o.clearDiskInstanceOpSafe(opts.Project, opts.Location, opts.DeviceName, opts.InstanceName, op.Name)
	return nil
}

// ExecuteDetachDisk verifies the cache content before starting a detach disk operation, and then clears the cache if the operation completes successfully.
func (o *OpsManager) ExecuteDetachDisk(ctx context.Context, opts *DetachDiskOpts) error {
	// Acquire the ops cache lock, verify ops and proceed to start attach disk operation.
	op, err := o.checkCacheAndStartDetachDiskOp(ctx, opts.Project, opts.Location, opts.DeviceName, opts.InstanceName)
	if err != nil {
		return err
	}

	// Lock released, wait for the op to complete.
	err = o.cloudProvider.WaitForZonalOp(ctx, opts.Project, op.Name, opts.Location)
	if err != nil {
		return status.Error(codes.Internal, fmt.Sprintf("%v", err))
	}

	// Op succeeded, acquire cache lock and clear op.
	o.clearDiskInstanceOpSafe(opts.Project, opts.Location, opts.DeviceName, opts.InstanceName, op.Name)
	return nil
}

func (o *OpsManager) checkAndUpdateLastKnownInstanceOps(ctx context.Context, project, location, instanceName string) error {
	ops := o.opsCache.InstanceOps.GetOps(common.CreateInstanceKey(project, location, instanceName))
	if ops == nil {
		return nil
	}

	for _, op := range ops {
		klog.V(5).Infof("Found last known op name %q (type %q) for instance %q", op.Name, op.Type, instanceName)
		done, err := o.cloudProvider.CheckZonalOpDoneStatus(ctx, project, location, op.Name)
		if err != nil {
			return status.Error(codes.Internal, fmt.Sprintf("%v", err))
		}

		if !done {
			return status.Error(codes.Aborted, fmt.Sprintf("operation %q (type %q) is still in progress", op.Name, op.Type))
		}

		o.opsCache.InstanceOps.ClearOp(common.CreateInstanceKey(project, location, instanceName), op.Name)
	}

	return nil
}

func (o *OpsManager) checkAndUpdateLastKnownDiskInstanceOp(ctx context.Context, project, location, deviceName, instanceName string) error {
	opInfo := o.opsCache.DiskInstanceOps.GetOp(common.CreateDiskInstanceKey(project, location, deviceName, instanceName))
	if opInfo == nil {
		return nil
	}

	klog.V(5).Infof("Found last known op name %q (type %q) for instance %q", opInfo.Name, opInfo.Type, instanceName)
	done, err := o.cloudProvider.CheckZonalOpDoneStatus(ctx, project, location, opInfo.Name)
	if err != nil {
		return status.Error(codes.Internal, fmt.Sprintf("%v", err))
	}

	if !done {
		return status.Error(codes.Aborted, fmt.Sprintf("operation %q (type %q) is still in progress", opInfo.Name, opInfo.Type))
	}

	o.opsCache.DiskInstanceOps.ClearOp(common.CreateDiskInstanceKey(project, location, deviceName, instanceName), opInfo.Name)
	return nil
}

func (o *OpsManager) checkCacheAndStartAttachDiskOp(ctx context.Context, volKey *meta.Key, readWrite, diskType, project, instanceZone, deviceName, instanceName string) (*compute.Operation, error) {
	o.Lock()
	defer o.Unlock()

	err := o.checkAndUpdateLastKnownDiskInstanceOp(ctx, project, instanceZone, deviceName, instanceName)
	if err != nil {
		return nil, err
	}

	err = o.checkAndUpdateLastKnownInstanceOps(ctx, project, instanceZone, instanceName)
	if err != nil {
		return nil, err
	}

	op, err := o.cloudProvider.StartAttachDiskOp(ctx, volKey, readWrite, diskType, project, instanceZone, instanceName)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("%v", err))
	}

	klog.V(5).Infof("AttachDisk operation %q for disk %q started on instance %q", op.Name, deviceName, instanceName)
	o.opsCache.DiskInstanceOps.AddOp(common.CreateDiskInstanceKey(project, instanceZone, deviceName, instanceName), common.OpInfo{Name: op.Name, Type: op.OperationType})
	return op, nil
}

// CheckCacheAndStartDetachDiskOp first verifies that there is no ongoing attach/detach operation for the given disk + instance combination, before triggering a new detach operation.
func (o *OpsManager) checkCacheAndStartDetachDiskOp(ctx context.Context, project, instanceZone, deviceName, instanceName string) (*compute.Operation, error) {
	o.Lock()
	defer o.Unlock()

	err := o.checkAndUpdateLastKnownDiskInstanceOp(ctx, project, instanceZone, deviceName, instanceName)
	if err != nil {
		return nil, err
	}

	err = o.checkAndUpdateLastKnownInstanceOps(ctx, project, instanceZone, instanceName)
	if err != nil {
		return nil, err
	}

	op, err := o.cloudProvider.StartDetachDiskOp(ctx, project, instanceZone, deviceName, instanceName)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("%v", err))
	}

	klog.V(5).Infof("DetachDisk operation %q for disk %q started on instance %q", op.Name, deviceName, instanceName)
	o.opsCache.DiskInstanceOps.AddOp(common.CreateDiskInstanceKey(project, instanceZone, deviceName, instanceName), common.OpInfo{Name: op.Name, Type: op.OperationType})
	return op, nil
}
func (o *OpsManager) HydrateOpsCache() {
	var ops []*compute.Operation
	retry.OnError(backoff, func(err error) bool { return true }, func() error {
		var err error
		ops, err = o.cloudProvider.ListZonalOps(context.Background(), map[string]bool{
			"attachDisk": true,
			"detachDisk": true})
		return err
	})

	klog.V(5).Infof("Found %d zonal ops", len(ops))
	o.Lock()
	defer o.Unlock()

	for _, op := range ops {
		if done, _ := o.cloudProvider.OpIsDone(op); done {
			continue
		}
		project, zone, instanceName, err := common.ParseOpTargetLinkUrl(op.TargetLink)
		if err != nil {
			klog.Errorf("Failed to parse operation target link, err: %s", err)
			continue
		}

		klog.V(5).Infof("Adding op %q (type %q) for project %q, zone %q, instance %q to cache", op.Name, op.OperationType, project, zone, instanceName)
		o.opsCache.InstanceOps.AddOp(common.CreateInstanceKey(project, zone, instanceName), common.OpInfo{Name: op.Name, Type: op.OperationType})
	}

	o.ready = true
}

func (o *OpsManager) clearDiskInstanceOpSafe(project, location, diskName, instanceName, opName string) error {
	o.Lock()
	defer o.Unlock()
	return o.opsCache.DiskInstanceOps.ClearOp(common.CreateDiskInstanceKey(project, location, diskName, instanceName), opName)
}

func (o *OpsManager) IsReady() bool {
	return o.ready
}
