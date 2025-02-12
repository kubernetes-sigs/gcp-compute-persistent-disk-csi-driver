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

package remote

import (
	"context"
	"fmt"
	"time"

	csipb "github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc"
	"k8s.io/klog/v2"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"

	"k8s.io/apimachinery/pkg/util/wait"
)

var (
	stdVolCap = &csipb.VolumeCapability{
		AccessType: &csipb.VolumeCapability_Mount{
			Mount: &csipb.VolumeCapability_MountVolume{},
		},
		AccessMode: &csipb.VolumeCapability_AccessMode{
			Mode: csipb.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		},
	}
	blockVolCap = &csipb.VolumeCapability{
		AccessType: &csipb.VolumeCapability_Block{
			Block: &csipb.VolumeCapability_BlockVolume{},
		},
		AccessMode: &csipb.VolumeCapability_AccessMode{
			Mode: csipb.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		},
	}
	stdVolCaps = []*csipb.VolumeCapability{
		stdVolCap,
	}
	stdCapRange = &csipb.CapacityRange{
		RequiredBytes: common.GbToBytes(20),
	}
)

const (
	// Keys in the volume context.
	contextForceAttach   = "force-attach"
	contextDataCacheSize = "data-cache-size"
	contextDataCacheMode = "data-cache-mode"

	// Keys in the publish context
	contexLocalSsdCacheSize = "local-ssd-cache-size"

	defaultLocalSsdCacheSize = "200Gi"
	defaultDataCacheMode     = common.DataCacheModeWriteThrough
)

type CsiClient struct {
	conn       *grpc.ClientConn
	idClient   csipb.IdentityClient
	nodeClient csipb.NodeClient
	ctrlClient csipb.ControllerClient

	endpoint string
}

func CreateCSIClient(endpoint string) *CsiClient {
	return &CsiClient{endpoint: endpoint}
}

func (c *CsiClient) AssertCSIConnection() error {
	var err error

	if err != nil {
		return err
	}
	if c.conn == nil {
		var conn *grpc.ClientConn
		err = wait.Poll(10*time.Second, 3*time.Minute, func() (bool, error) {
			conn, err = grpc.Dial(
				c.endpoint,
				grpc.WithInsecure(),
			)
			if err != nil {
				klog.Warningf("Client failed to dail endpoint %v", c.endpoint)
				return false, nil
			}
			return true, nil
		})
		if err != nil || conn == nil {
			return fmt.Errorf("Failed to get client connection: %v", err.Error())
		}
		c.conn = conn
		c.idClient = csipb.NewIdentityClient(conn)
		c.nodeClient = csipb.NewNodeClient(conn)
		c.ctrlClient = csipb.NewControllerClient(conn)
	}
	return nil
}

func (c *CsiClient) CloseConn() error {
	return c.conn.Close()
}

func (c *CsiClient) CreateVolumeWithCaps(volName string, params map[string]string, sizeInGb int64, topReq *csipb.TopologyRequirement, caps []*csipb.VolumeCapability, volContentSrc *csipb.VolumeContentSource) (*csipb.Volume, error) {
	capRange := &csipb.CapacityRange{
		RequiredBytes: common.GbToBytes(sizeInGb),
	}
	cvr := &csipb.CreateVolumeRequest{
		Name:               volName,
		VolumeCapabilities: caps,
		Parameters:         params,
		CapacityRange:      capRange,
	}
	if topReq != nil {
		cvr.AccessibilityRequirements = topReq
	}
	if volContentSrc != nil {
		cvr.VolumeContentSource = volContentSrc
	}
	cresp, err := c.ctrlClient.CreateVolume(context.Background(), cvr)
	if err != nil {
		return nil, err
	}
	return cresp.Volume, nil
}

func (c *CsiClient) CreateVolume(volName string, params map[string]string, sizeInGb int64, topReq *csipb.TopologyRequirement, volContentSrc *csipb.VolumeContentSource) (*csipb.Volume, error) {
	return c.CreateVolumeWithCaps(volName, params, sizeInGb, topReq, stdVolCaps, volContentSrc)
}

func (c *CsiClient) DeleteVolume(volId string) error {
	dvr := &csipb.DeleteVolumeRequest{
		VolumeId: volId,
	}
	_, err := c.ctrlClient.DeleteVolume(context.Background(), dvr)
	return err
}

func (c *CsiClient) ControllerPublishVolumeReadOnly(volId, nodeId string) error {
	return c.ControllerPublishVolume(volId, nodeId, false /* forceAttach */, true /* readOnly */)
}

func (c *CsiClient) ControllerPublishVolumeReadWrite(volId, nodeId string, forceAttach bool) error {
	return c.ControllerPublishVolume(volId, nodeId, forceAttach, false /* readOnly */)
}

func (c *CsiClient) ControllerPublishVolume(volId, nodeId string, forceAttach bool, readOnly bool) error {
	cpreq := &csipb.ControllerPublishVolumeRequest{
		VolumeId:         volId,
		NodeId:           nodeId,
		VolumeCapability: stdVolCap,
		Readonly:         readOnly,
	}
	if forceAttach {
		cpreq.VolumeContext = map[string]string{
			"force-attach": "true",
		}
	}
	_, err := c.ctrlClient.ControllerPublishVolume(context.Background(), cpreq)
	return err
}

func (c *CsiClient) ListVolumes() (map[string]([]string), error) {
	resp, err := c.ctrlClient.ListVolumes(context.Background(), &csipb.ListVolumesRequest{})
	if err != nil {
		return nil, err
	}
	vols := map[string]([]string){}
	for _, e := range resp.Entries {
		vols[e.Volume.VolumeId] = e.Status.PublishedNodeIds
	}
	return vols, nil
}

func (c *CsiClient) ControllerUnpublishVolume(volId, nodeId string) error {
	cupreq := &csipb.ControllerUnpublishVolumeRequest{
		VolumeId: volId,
		NodeId:   nodeId,
	}
	_, err := c.ctrlClient.ControllerUnpublishVolume(context.Background(), cupreq)
	return err
}

func (c *CsiClient) NodeStageExt4Volume(volId, stageDir string, setupDataCache bool) error {
	return c.NodeStageVolume(volId, stageDir, stdVolCap, setupDataCache)
}

func (c *CsiClient) NodeStageBlockVolume(volId, stageDir string, setupDataCache bool) error {
	return c.NodeStageVolume(volId, stageDir, blockVolCap, setupDataCache)
}

func (c *CsiClient) NodeStageVolume(volId string, stageDir string, volumeCap *csipb.VolumeCapability, setupDataCache bool) error {
	publishContext := map[string]string{}
	if setupDataCache {
		publishContext[contexLocalSsdCacheSize] = defaultLocalSsdCacheSize
		publishContext[contextDataCacheMode] = defaultDataCacheMode
	}
	nodeStageReq := &csipb.NodeStageVolumeRequest{
		VolumeId:          volId,
		StagingTargetPath: stageDir,
		VolumeCapability:  volumeCap,
		PublishContext:    publishContext,
	}
	_, err := c.nodeClient.NodeStageVolume(context.Background(), nodeStageReq)
	return err
}

func (c *CsiClient) NodeUnstageVolume(volId, stageDir string) error {
	nodeUnstageReq := &csipb.NodeUnstageVolumeRequest{
		VolumeId:          volId,
		StagingTargetPath: stageDir,
	}
	_, err := c.nodeClient.NodeUnstageVolume(context.Background(), nodeUnstageReq)
	return err
}

func (c *CsiClient) NodeUnpublishVolume(volumeID, publishDir string) error {
	nodeUnpublishReq := &csipb.NodeUnpublishVolumeRequest{
		VolumeId:   volumeID,
		TargetPath: publishDir,
	}
	_, err := c.nodeClient.NodeUnpublishVolume(context.Background(), nodeUnpublishReq)
	return err
}

func (c *CsiClient) NodePublishVolume(volumeID, stageDir, publishDir string) error {
	nodePublishReq := &csipb.NodePublishVolumeRequest{
		VolumeId:          volumeID,
		StagingTargetPath: stageDir,
		TargetPath:        publishDir,
		VolumeCapability:  stdVolCap,
		Readonly:          false,
	}
	_, err := c.nodeClient.NodePublishVolume(context.Background(), nodePublishReq)
	return err
}

func (c *CsiClient) NodePublishBlockVolume(volumeID, stageDir, publishDir string) error {
	nodePublishReq := &csipb.NodePublishVolumeRequest{
		VolumeId:          volumeID,
		StagingTargetPath: stageDir,
		TargetPath:        publishDir,
		VolumeCapability:  blockVolCap,
		Readonly:          false,
	}
	_, err := c.nodeClient.NodePublishVolume(context.Background(), nodePublishReq)
	return err
}

func (c *CsiClient) ControllerExpandVolume(volumeID string, sizeGb int64) error {
	controllerExpandReq := &csipb.ControllerExpandVolumeRequest{
		VolumeId: volumeID,
		CapacityRange: &csipb.CapacityRange{
			RequiredBytes: common.GbToBytes(sizeGb),
		},
	}
	_, err := c.ctrlClient.ControllerExpandVolume(context.Background(), controllerExpandReq)
	return err
}

func (c *CsiClient) NodeExpandVolume(volumeID, volumePath string, sizeGb int64) (*csipb.NodeExpandVolumeResponse, error) {
	nodeExpandReq := &csipb.NodeExpandVolumeRequest{
		VolumeId:   volumeID,
		VolumePath: volumePath,
		CapacityRange: &csipb.CapacityRange{
			RequiredBytes: common.GbToBytes(sizeGb),
		},
	}
	return c.nodeClient.NodeExpandVolume(context.Background(), nodeExpandReq)
}

func (c *CsiClient) NodeGetInfo() (*csipb.NodeGetInfoResponse, error) {
	resp, err := c.nodeClient.NodeGetInfo(context.Background(), &csipb.NodeGetInfoRequest{})
	return resp, err
}

func (c *CsiClient) NodeGetVolumeStats(volumeID, volumePath string) (available, capacity, used, inodesFree, inodes, inodesUsed int64, err error) {
	resp, err := c.nodeClient.NodeGetVolumeStats(context.Background(), &csipb.NodeGetVolumeStatsRequest{
		VolumeId:   volumeID,
		VolumePath: volumePath,
	})
	if err != nil {
		return
	}
	for _, usage := range resp.Usage {
		if usage == nil {
			continue
		}
		unit := usage.GetUnit()
		switch unit {
		case csipb.VolumeUsage_BYTES:
			available = usage.GetAvailable()
			capacity = usage.GetTotal()
			used = usage.GetUsed()
		case csipb.VolumeUsage_INODES:
			inodesFree = usage.GetAvailable()
			inodes = usage.GetTotal()
			inodesUsed = usage.GetUsed()
		default:
			err = fmt.Errorf("unknown key %s in usage", unit.String())
			return
		}
	}
	return
}

func (c *CsiClient) CreateSnapshot(snapshotName, sourceVolumeId string, params map[string]string) (string, error) {

	csr := &csipb.CreateSnapshotRequest{
		Name:           snapshotName,
		SourceVolumeId: sourceVolumeId,
		Parameters:     params,
	}
	cresp, err := c.ctrlClient.CreateSnapshot(context.Background(), csr)
	if err != nil {
		return "", err
	}
	return cresp.GetSnapshot().GetSnapshotId(), nil
}

func (c *CsiClient) DeleteSnapshot(snapshotID string) error {
	dsr := &csipb.DeleteSnapshotRequest{
		SnapshotId: snapshotID,
	}
	_, err := c.ctrlClient.DeleteSnapshot(context.Background(), dsr)
	return err
}
