package linkcache

import (
	"os"
	"testing"
	"testing/fstest"
)

const (
	// Test disk names in /dev/disk/by-id format
	gcpPersistentDiskID          = "google-persistent-disk-0"
	gcpPVCID                     = "google-pvc-f5418f78-dc07-4d69-9487-6c4a7232dd67"
	gcpPersistentDiskPartitionID = "google-persistent-disk-0-part1"

	// Test device paths in /dev format
	devicePathSDA = "/dev/sda"
	devicePathSDB = "/dev/sdb"
)

// mockFS implements fsInterface for testing
type mockFS struct {
	fstest.MapFS
	symlinks map[string]string
}

func newMockFS() *mockFS {
	return &mockFS{
		MapFS:    make(fstest.MapFS),
		symlinks: make(map[string]string),
	}
}

func (m *mockFS) ReadDir(name string) ([]os.DirEntry, error) {
	entries, err := m.MapFS.ReadDir(name)
	if err != nil {
		return nil, err
	}
	return entries, nil
}

func (m *mockFS) EvalSymlinks(path string) (string, error) {
	if target, ok := m.symlinks[path]; ok {
		return target, nil
	}
	return "", os.ErrNotExist
}

func TestListAndUpdate(t *testing.T) {
	tests := []struct {
		name          string
		setupFS       func(*mockFS)
		expectedLinks map[string]string
		expectError   bool
	}{
		{
			name: "valid symlinks",
			setupFS: func(m *mockFS) {
				// Create some device files
				m.MapFS[gcpPersistentDiskID] = &fstest.MapFile{}
				m.MapFS[gcpPVCID] = &fstest.MapFile{}
				// Create symlinks
				m.symlinks[gcpPersistentDiskID] = devicePathSDA
				m.symlinks[gcpPVCID] = devicePathSDB
			},
			expectedLinks: map[string]string{
				gcpPersistentDiskID: devicePathSDA,
				gcpPVCID:            devicePathSDB,
			},
			expectError: false,
		},
		{
			name: "broken symlink not added to cache",
			setupFS: func(m *mockFS) {
				m.MapFS[gcpPersistentDiskID] = &fstest.MapFile{}
				// No symlink target for gcpPersistentDiskID
			},
			expectedLinks: map[string]string{},
			expectError:   true,
		},
		{
			name: "partition files ignored",
			setupFS: func(m *mockFS) {
				m.MapFS[gcpPersistentDiskPartitionID] = &fstest.MapFile{}
				m.MapFS[gcpPersistentDiskID] = &fstest.MapFile{}
				m.symlinks[gcpPersistentDiskID] = devicePathSDA
			},
			expectedLinks: map[string]string{
				gcpPersistentDiskID: devicePathSDA,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := newMockFS()
			tt.setupFS(mock)

			cache := NewListingCache(0, ".")
			cache.fs = mock // Inject our mock filesystem
			err := cache.listAndUpdate()

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}

			// Verify the cache contents
			for symlink, expectedTarget := range tt.expectedLinks {
				entry, exists := cache.links.devices[symlink]
				if !exists {
					t.Errorf("symlink %s should exist in cache", symlink)
					continue
				}
				if entry.path != expectedTarget {
					t.Errorf("symlink %s should point to %s, got %s", symlink, expectedTarget, entry.path)
				}
				if entry.brokenSymlink {
					t.Errorf("symlink %s should not be marked as broken", symlink)
				}
			}
		})
	}
}

func TestListAndUpdateWithChanges(t *testing.T) {
	mock := newMockFS()
	cache := NewListingCache(0, ".")
	cache.fs = mock

	// Initial state: one disk with a valid symlink
	mock.MapFS[gcpPersistentDiskID] = &fstest.MapFile{}
	mock.symlinks[gcpPersistentDiskID] = devicePathSDA

	// First listAndUpdate should add the disk to cache
	err := cache.listAndUpdate()
	if err != nil {
		t.Fatalf("unexpected error in first listAndUpdate: %v", err)
	}

	// Verify initial state
	entry, exists := cache.links.devices[gcpPersistentDiskID]
	if !exists {
		t.Fatal("gcpPersistentDiskID should exist in cache after first listAndUpdate")
	}
	if entry.path != devicePathSDA {
		t.Errorf("gcpPersistentDiskID should point to %s, got %s", devicePathSDA, entry.path)
	}

	// Add a new disk and update the symlink target
	mock.MapFS[gcpPVCID] = &fstest.MapFile{}
	mock.symlinks[gcpPVCID] = devicePathSDB
	mock.symlinks[gcpPersistentDiskID] = devicePathSDB // Update existing disk's target

	// Second listAndUpdate should update the cache
	err = cache.listAndUpdate()
	if err != nil {
		t.Fatalf("unexpected error in second listAndUpdate: %v", err)
	}

	// Verify both disks are in cache with correct paths
	entry, exists = cache.links.devices[gcpPersistentDiskID]
	if !exists {
		t.Fatal("gcpPersistentDiskID should still exist in cache")
	}
	if entry.path != devicePathSDB {
		t.Errorf("gcpPersistentDiskID should now point to %s, got %s", devicePathSDB, entry.path)
	}

	entry, exists = cache.links.devices[gcpPVCID]
	if !exists {
		t.Fatal("gcpPVCID should exist in cache after second listAndUpdate")
	}
	if entry.path != devicePathSDB {
		t.Errorf("gcpPVCID should point to %s, got %s", devicePathSDB, entry.path)
	}

	// Break the symlink for gcpPersistentDiskID but keep the file
	delete(mock.symlinks, gcpPersistentDiskID)

	// Third listAndUpdate should mark the disk as broken but keep its last known value
	err = cache.listAndUpdate()
	if err == nil {
		t.Error("expected error for broken symlink")
	}

	// Verify gcpPersistentDiskID is marked as broken but maintains its last known value
	entry, exists = cache.links.devices[gcpPersistentDiskID]
	if !exists {
		t.Fatal("gcpPersistentDiskID should still exist in cache")
	}
	if entry.path != devicePathSDB {
		t.Errorf("gcpPersistentDiskID should maintain its last known value %s, got %s", devicePathSDB, entry.path)
	}
	if !entry.brokenSymlink {
		t.Error("gcpPersistentDiskID should be marked as broken")
	}

	// Verify gcpPVCID is still valid
	entry, exists = cache.links.devices[gcpPVCID]
	if !exists {
		t.Fatal("gcpPVCID should still exist in cache")
	}
	if entry.path != devicePathSDB {
		t.Errorf("gcpPVCID should still point to %s, got %s", devicePathSDB, entry.path)
	}
	if entry.brokenSymlink {
		t.Error("gcpPVCID should not be marked as broken")
	}

	// Remove one disk
	delete(mock.MapFS, gcpPersistentDiskID)
	delete(mock.symlinks, gcpPersistentDiskID)

	// Fourth listAndUpdate should remove the deleted disk
	err = cache.listAndUpdate()
	if err != nil {
		t.Fatalf("unexpected error in fourth listAndUpdate: %v", err)
	}

	// Verify only gcpPVCID remains
	if _, exists := cache.links.devices[gcpPersistentDiskID]; exists {
		t.Error("gcpPersistentDiskID should be removed from cache")
	}

	entry, exists = cache.links.devices[gcpPVCID]
	if !exists {
		t.Fatal("gcpPVCID should still exist in cache")
	}
	if entry.path != devicePathSDB {
		t.Errorf("gcpPVCID should still point to %s, got %s", devicePathSDB, entry.path)
	}
}
