/*
Copyright 2026 The Kubernetes Authors.

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
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/k8s-cloud-provider/pkg/cloud/meta"
	csi "github.com/container-storage-interface/spec/lib/go/csi"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	storagev1beta1 "k8s.io/api/storage/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/constants"
	gce "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/compute"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/k8sclient"
)

// sizeAwareConversionCloudProvider enforces GCE's real hyperdisk-balanced
// IOPS-per-GiB ceiling (500 IOPS/GiB) based on the disk's current size at the
// moment ConvertDiskType is called.
type sizeAwareConversionCloudProvider struct {
	*gce.FakeCloudProvider
	convertCalls int
}

func (p *sizeAwareConversionCloudProvider) ConvertDiskType(
	ctx context.Context,
	proj string,
	volKey *meta.Key,
	targetDiskType, requestID string,
	provisionedIops, provisionedThroughput *int64,
) (string, error) {
	p.convertCalls++
	disk, err := p.GetDisk(ctx, proj, volKey)
	if err != nil {
		return "", err
	}
	if targetDiskType == "hyperdisk-balanced" && provisionedIops != nil && *provisionedIops > disk.GetSizeGb()*500 {
		apiErr := conversionAPIError(400, "invalidParameter",
			fmt.Sprintf("provisionedIops %d exceeds max %d for %dGiB hyperdisk-balanced",
				*provisionedIops, disk.GetSizeGb()*500, disk.GetSizeGb()))
		return "", gce.NewConversionStatusError(codes.InvalidArgument, apiErr)
	}
	return p.FakeCloudProvider.ConvertDiskType(ctx, proj, volKey, targetDiskType, requestID, provisionedIops, provisionedThroughput)
}

// TestSimultaneousResizeAndConversionRace_SizeGatedTargetLatchesInfeasible
// proves that patching PVC storage (50Gi -> 200Gi) and VAC
// (type=hyperdisk-balanced, iops=50000) in a single update is order-dependent:
//   - Order A (ExpandVolume first, ModifyVolume second): converges cleanly.
//   - Order B (ModifyVolume first, ExpandVolume second, ModifyVolume retried):
//     permanently latches Infeasible (InvalidArgument) even AFTER the disk is
//     resized to 200Gi because conversionIntent omits disk sizeGb.
func TestSimultaneousResizeAndConversionRace_SizeGatedTargetLatchesInfeasible(t *testing.T) {
	className := "vac-hdb-50k"
	vac := &storagev1beta1.VolumeAttributesClass{
		ObjectMeta: metav1.ObjectMeta{Name: className},
		DriverName: driver,
		Parameters: map[string]string{"type": "hyperdisk-balanced", "iops": "50000"},
	}
	modifyReq := &csi.ControllerModifyVolumeRequest{
		VolumeId: testVolumeID,
		MutableParameters: map[string]string{
			"type": "hyperdisk-balanced",
			"iops": "50000",
		},
	}
	expandReq := &csi.ControllerExpandVolumeRequest{
		VolumeId:      testVolumeID,
		CapacityRange: &csi.CapacityRange{RequiredBytes: common.GbToBytes(200)},
	}

	setupEnv := func(t *testing.T) (*GCEDriver, kubernetes.Interface, *sizeAwareConversionCloudProvider, func()) {
		t.Helper()
		oldGetKubeClient := k8sclient.GetClient
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "test-pvc", Namespace: "default"},
			Spec:       corev1.PersistentVolumeClaimSpec{VolumeAttributesClassName: &className},
		}
		pv := &corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec: corev1.PersistentVolumeSpec{
				ClaimRef: &corev1.ObjectReference{Name: pvc.Name, Namespace: pvc.Namespace},
			},
		}
		kubeClient := newConversionKubeClient(pv, pvc, vac)
		k8sclient.GetClient = func() (kubernetes.Interface, error) { return kubeClient, nil }

		disk := &compute.Disk{Name: name, Zone: zone, SelfLink: testVolumeID, Type: "pd-balanced", SizeGb: 50}
		baseFCP, err := gce.CreateFakeCloudProvider(project, zone, []*gce.CloudDisk{gce.CloudDiskFromV1(disk)})
		if err != nil {
			t.Fatalf("CreateFakeCloudProvider failed: %v", err)
		}
		cp := &sizeAwareConversionCloudProvider{FakeCloudProvider: baseFCP}
		driver := initGCEDriverWithCloudProvider(t, cp, &GCEControllerServerArgs{EnableDiskTypeConversion: true})
		driver.cs.conversionPollBackoffOverride = wait.Backoff{Duration: time.Millisecond, Factor: 1.0, Steps: 1000}

		cleanup := func() {
			driver.cs.WaitForConversionWorkers()
			k8sclient.GetClient = oldGetKubeClient
		}
		return driver, kubeClient, cp, cleanup
	}

	t.Run("OrderA_ExpandFirst_ThenModify_Converges", func(t *testing.T) {
		driver, kubeClient, _, cleanup := setupEnv(t)
		defer cleanup()

		// 1. ExpandVolume (50Gi -> 200Gi) wins the race.
		if _, err := driver.cs.ControllerExpandVolume(context.Background(), expandReq); err != nil {
			t.Fatalf("ExpandVolume failed: %v", err)
		}

		// 2. ModifyVolume arrives second on the 200Gi disk -> conversion starts (Unavailable while in progress).
		_, err := driver.cs.ControllerModifyVolume(context.Background(), modifyReq)
		if got := status.Code(err); got != codes.Unavailable {
			t.Fatalf("ModifyVolume after expand: got %v (%v), want Unavailable (conversion started)", got, err)
		}

		// Wait for background watcher to finish reconciling the conversion.
		err = wait.PollUntilContextTimeout(context.Background(), 5*time.Millisecond, 5*time.Second, true, func(ctx context.Context) (bool, error) {
			pv, getErr := kubeClient.CoreV1().PersistentVolumes().Get(ctx, name, metav1.GetOptions{})
			if getErr != nil {
				return false, nil
			}
			return pv.Annotations[constants.DiskTypeConversionOperationKey] == "" &&
				pv.Annotations[constants.DiskTypeConvertedToKey] == "hyperdisk-balanced", nil
		})
		if err != nil {
			t.Fatalf("conversion did not finish in Order A: %v", err)
		}

		// 3. Subsequent ModifyVolume retry succeeds with OK!
		if _, err := driver.cs.ControllerModifyVolume(context.Background(), modifyReq); err != nil {
			t.Fatalf("final ModifyVolume retry in Order A failed: %v", err)
		}
	})

	t.Run("OrderB_ModifyFirst_ThenExpand_ThenModifyRetry_LatchesInfeasible", func(t *testing.T) {
		driver, kubeClient, cp, cleanup := setupEnv(t)
		defer cleanup()

		// 1. ModifyVolume arrives FIRST while disk is still 50Gi -> GCE rejects 50k IOPS on 50Gi.
		_, err := driver.cs.ControllerModifyVolume(context.Background(), modifyReq)
		if got := status.Code(err); got != codes.InvalidArgument {
			t.Fatalf("initial ModifyVolume on 50Gi disk: got %v (%v), want InvalidArgument", got, err)
		}
		if cp.convertCalls != 1 {
			t.Fatalf("convertCalls = %d, want 1", cp.convertCalls)
		}

		// 2. ExpandVolume (50Gi -> 200Gi) arrives SECOND and succeeds! Disk is now 200Gi.
		if _, err := driver.cs.ControllerExpandVolume(context.Background(), expandReq); err != nil {
			t.Fatalf("ExpandVolume after rejected ModifyVolume failed: %v", err)
		}

		// 3. external-resizer retries ModifyVolume now that disk is 200Gi (where 50k IOPS IS valid).
		// BUG: Because conversionIntent("hyperdisk-balanced", 50000, nil) omits sizeGb,
		// convertDiskType returns InvalidArgument from the cached PV annotation WITHOUT calling GCE!
		_, retryErr := driver.cs.ControllerModifyVolume(context.Background(), modifyReq)
		if status.Code(retryErr) == codes.InvalidArgument {
			pv, _ := kubeClient.CoreV1().PersistentVolumes().Get(context.Background(), name, metav1.GetOptions{})
			t.Fatalf("RACE BUG REPRODUCED: ModifyVolume retry on resized 200Gi disk stayed permanently stuck in InvalidArgument (Infeasible) without calling GCE again (convertCalls=%d, cached intent=%q, err=%v)",
				cp.convertCalls,
				pv.Annotations[constants.DiskTypeConversionInfeasibleIntentKey],
				retryErr)
		}
	})
}

// TestSimultaneousResizeAndConversionRace_PendingConversionBlocksOnlineExpand
// proves that on an attached (online) volume, patching storage + conversion VAC
// together blocks online expansion if ModifyVolume happens to arrive first.
func TestSimultaneousResizeAndConversionRace_PendingConversionBlocksOnlineExpand(t *testing.T) {
	className := "vac-hdb"
	vac := &storagev1beta1.VolumeAttributesClass{
		ObjectMeta: metav1.ObjectMeta{Name: className},
		DriverName: driver,
		Parameters: map[string]string{"type": "hyperdisk-balanced"},
	}
	modifyReq := &csi.ControllerModifyVolumeRequest{
		VolumeId:          testVolumeID,
		MutableParameters: map[string]string{"type": "hyperdisk-balanced"},
	}
	expandReq := &csi.ControllerExpandVolumeRequest{
		VolumeId:      testVolumeID,
		CapacityRange: &csi.CapacityRange{RequiredBytes: common.GbToBytes(200)},
	}

	setupAttachedEnv := func(t *testing.T) (*GCEDriver, func()) {
		t.Helper()
		oldGetKubeClient := k8sclient.GetClient
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "test-pvc", Namespace: "default"},
			Spec:       corev1.PersistentVolumeClaimSpec{VolumeAttributesClassName: &className},
		}
		pv := &corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec: corev1.PersistentVolumeSpec{
				ClaimRef: &corev1.ObjectReference{Name: pvc.Name, Namespace: pvc.Namespace},
			},
		}
		kubeClient := newConversionKubeClient(pv, pvc, vac)
		k8sclient.GetClient = func() (kubernetes.Interface, error) { return kubeClient, nil }

		attachedDisk := &compute.Disk{
			Name:     name,
			Zone:     zone,
			SelfLink: testVolumeID,
			Type:     "pd-balanced",
			SizeGb:   50,
			Users:    []string{fmt.Sprintf("projects/%s/zones/%s/instances/test-node", project, zone)},
		}
		fcp, err := gce.CreateFakeCloudProvider(project, zone, []*gce.CloudDisk{gce.CloudDiskFromV1(attachedDisk)})
		if err != nil {
			t.Fatalf("CreateFakeCloudProvider failed: %v", err)
		}
		driver := initGCEDriverWithCloudProvider(t, fcp, &GCEControllerServerArgs{EnableDiskTypeConversion: true})
		cleanup := func() {
			driver.cs.WaitForConversionWorkers()
			k8sclient.GetClient = oldGetKubeClient
		}
		return driver, cleanup
	}

	t.Run("OrderA_ExpandFirst_ThenModify_OnlineResizeSucceeds", func(t *testing.T) {
		driver, cleanup := setupAttachedEnv(t)
		defer cleanup()

		// 1. ExpandVolume arrives first -> online resize succeeds while attached!
		if _, err := driver.cs.ControllerExpandVolume(context.Background(), expandReq); err != nil {
			t.Fatalf("online ExpandVolume failed in Order A: %v", err)
		}

		// 2. ModifyVolume arrives second -> queues conversion as Pending for future detach.
		_, err := driver.cs.ControllerModifyVolume(context.Background(), modifyReq)
		if got := status.Code(err); got != codes.FailedPrecondition {
			t.Fatalf("ModifyVolume on attached disk: got %v (%v), want FailedPrecondition", got, err)
		}
	})

	t.Run("OrderB_ModifyFirst_ThenExpand_OnlineResizeBlockedByPendingConversion", func(t *testing.T) {
		driver, cleanup := setupAttachedEnv(t)
		defer cleanup()

		// 1. ModifyVolume arrives first -> writes Pending annotation on PV (no GCE op started).
		_, err := driver.cs.ControllerModifyVolume(context.Background(), modifyReq)
		if got := status.Code(err); got != codes.FailedPrecondition {
			t.Fatalf("ModifyVolume on attached disk: got %v (%v), want FailedPrecondition", got, err)
		}

		// 2. ExpandVolume arrives second -> online resize is blocked even though no GCE conversion is running!
		if _, expandErr := driver.cs.ControllerExpandVolume(context.Background(), expandReq); expandErr != nil {
			t.Fatalf("RACE BUG REPRODUCED: online ControllerExpandVolume was blocked on an attached disk solely because ModifyVolume arrived first and queued a Pending conversion: %v", expandErr)
		}
	})
}
