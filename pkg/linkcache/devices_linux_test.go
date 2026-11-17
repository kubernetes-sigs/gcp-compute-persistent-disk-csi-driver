package linkcache

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/deviceutils"
)

type fakeDeviceUtils struct {
	deviceutils.DeviceUtils
	fakePath string
}

func (f *fakeDeviceUtils) GetDiskByIdPaths(deviceName string, partition string) []string {
	return []string{f.fakePath}
}

func TestDeviceCache_Concurrent(t *testing.T) {
	// Create a temp file to act as the symlink path so EvalSymlinks succeeds
	tmpDir, err := os.MkdirTemp("", "device-cache-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	fakeSymlinkPath := filepath.Join(tmpDir, "fake-symlink")
	if err := os.WriteFile(fakeSymlinkPath, []byte(""), 0644); err != nil {
		t.Fatalf("failed to create fake symlink file: %v", err)
	}

	node := NewTestNodeWithVolumes(nil)
	d := newDeviceCacheForNode(10*time.Millisecond, node, "pd.csi.storage.gke.io", &fakeDeviceUtils{fakePath: fakeSymlinkPath}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the watcher loop in the background (runs listAndUpdate)
	go d.Run(ctx)

	concurrency := 10
	var wg sync.WaitGroup
	wg.Add(concurrency * 2)

	// Concurrent AddVolume
	for i := 0; i < concurrency; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				volID := fmt.Sprintf("projects/p/zones/z/disks/vol-%d-%d", id, j)
				_ = d.AddVolume(volID)
			}
		}(i)
	}

	// Concurrent RemoveVolume
	for i := 0; i < concurrency; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				volID := fmt.Sprintf("projects/p/zones/z/disks/vol-%d-%d", id, j)
				d.RemoveVolume(volID)
			}
		}(i)
	}

	wg.Wait()
}

func TestShouldWarnForSymlinkEvaluationError(t *testing.T) {
	tests := []struct {
		name            string
		device          deviceMapping
		hasResolvedPath bool
		want            bool
	}{
		{
			name:            "unresolved alternate alias with another working path",
			device:          deviceMapping{volumeID: "volume-1"},
			hasResolvedPath: true,
			want:            false,
		},
		{
			name:            "previously resolved alias disappeared while another path works",
			device:          deviceMapping{volumeID: "volume-1", realPath: "/dev/nvme0n1"},
			hasResolvedPath: true,
			want:            true,
		},
		{
			name:            "all aliases are unavailable",
			device:          deviceMapping{volumeID: "volume-1"},
			hasResolvedPath: false,
			want:            true,
		},
		{
			name:            "single unresolved alias",
			device:          deviceMapping{volumeID: "volume-1"},
			hasResolvedPath: false,
			want:            true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldWarnForSymlinkEvaluationError(tt.device, tt.hasResolvedPath); got != tt.want {
				t.Errorf("shouldWarnForSymlinkEvaluationError() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestDeviceCache_ListAndUpdateWithUnavailableAlternateSymlink(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "device")
	if err := os.WriteFile(target, []byte("device"), 0600); err != nil {
		t.Fatalf("failed to create target device file: %v", err)
	}

	workingSymlink := filepath.Join(tmpDir, "scsi-alias")
	if err := os.Symlink(target, workingSymlink); err != nil {
		t.Fatalf("failed to create working symlink: %v", err)
	}
	missingSymlink := filepath.Join(tmpDir, "nvme-alias")

	d := &DeviceCache{
		symlinks: map[string]deviceMapping{
			workingSymlink: {volumeID: "volume-1"},
			missingSymlink: {volumeID: "volume-1"},
		},
	}

	d.listAndUpdate()

	if got := d.symlinks[workingSymlink].realPath; got != target {
		t.Errorf("working symlink realPath = %q, want %q", got, target)
	}
	if got := d.symlinks[missingSymlink].realPath; got != "" {
		t.Errorf("unavailable alternate symlink realPath = %q, want empty", got)
	}
}

func TestDeviceCache_ListAndUpdatePreservesPreviouslyResolvedPathOnError(t *testing.T) {
	tmpDir := t.TempDir()
	missingSymlink := filepath.Join(tmpDir, "removed-alias")
	d := &DeviceCache{
		symlinks: map[string]deviceMapping{
			missingSymlink: {
				volumeID: "volume-1",
				realPath: "/dev/nvme0n1",
			},
		},
	}

	d.listAndUpdate()

	if got := d.symlinks[missingSymlink].realPath; got != "/dev/nvme0n1" {
		t.Errorf("realPath after symlink evaluation failure = %q, want previous path %q", got, "/dev/nvme0n1")
	}
}
