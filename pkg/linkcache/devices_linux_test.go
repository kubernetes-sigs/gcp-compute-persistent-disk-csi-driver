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
