package linkcache

import (
	"os"
	"testing"
	"testing/fstest"

	"github.com/google/go-cmp/cmp"
)

var allowUnexportedLinkCache = cmp.AllowUnexported(linkCache{}, linkCacheEntry{})

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
		expectedCache *linkCache
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
			expectedCache: &linkCache{
				devices: map[string]linkCacheEntry{
					gcpPersistentDiskID: {path: devicePathSDA, brokenSymlink: false},
					gcpPVCID:            {path: devicePathSDB, brokenSymlink: false},
				},
			},
			expectError: false,
		},
		{
			name: "broken symlink not added to cache",
			setupFS: func(m *mockFS) {
				m.MapFS[gcpPersistentDiskID] = &fstest.MapFile{}
				// No symlink target for gcpPersistentDiskID
			},
			expectedCache: &linkCache{
				devices: map[string]linkCacheEntry{},
			},
			expectError: true,
		},
		{
			name: "partition files ignored",
			setupFS: func(m *mockFS) {
				m.MapFS[gcpPersistentDiskPartitionID] = &fstest.MapFile{}
				m.MapFS[gcpPersistentDiskID] = &fstest.MapFile{}
				m.symlinks[gcpPersistentDiskID] = devicePathSDA
				m.symlinks[gcpPersistentDiskPartitionID] = devicePathSDA + "1"
			},
			expectedCache: &linkCache{
				devices: map[string]linkCacheEntry{
					gcpPersistentDiskID: {path: devicePathSDA, brokenSymlink: false},
				},
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

			// Compare the entire cache state
			if diff := cmp.Diff(tt.expectedCache, cache.links, allowUnexportedLinkCache); diff != "" {
				t.Errorf("linkCache mismatch (-expected +got):\n%s", diff)
			}
		})
	}
}

func TestLinkCache(t *testing.T) {
	tests := []struct {
		name       string
		setupCache func(*linkCache)
		expected   *linkCache
	}{
		{
			name: "AddOrUpdateDevice",
			setupCache: func(lc *linkCache) {
				lc.AddOrUpdateDevice("symlink1", "/dev/sda")
				lc.AddOrUpdateDevice("symlink2", "/dev/sdb")
			},
			expected: &linkCache{
				devices: map[string]linkCacheEntry{
					"symlink1": {path: "/dev/sda", brokenSymlink: false},
					"symlink2": {path: "/dev/sdb", brokenSymlink: false},
				},
			},
		},
		{
			name: "BrokenSymlink",
			setupCache: func(lc *linkCache) {
				lc.AddOrUpdateDevice("symlink1", "/dev/sda")
				lc.BrokenSymlink("symlink1")
			},
			expected: &linkCache{
				devices: map[string]linkCacheEntry{
					"symlink1": {path: "/dev/sda", brokenSymlink: true},
				},
			},
		},
		{
			name: "RemoveDevice",
			setupCache: func(lc *linkCache) {
				lc.AddOrUpdateDevice("symlink1", "/dev/sda")
				lc.RemoveDevice("symlink1")
			},
			expected: &linkCache{
				devices: map[string]linkCacheEntry{},
			},
		},
		{
			name: "PartitionIgnored",
			setupCache: func(lc *linkCache) {
				lc.AddOrUpdateDevice(gcpPersistentDiskPartitionID, devicePathSDA+"1")
				lc.AddOrUpdateDevice(gcpPersistentDiskID, devicePathSDA)
				lc.AddOrUpdateDevice(gcpPVCID, devicePathSDB)
			},
			expected: &linkCache{
				devices: map[string]linkCacheEntry{
					gcpPersistentDiskID: {path: devicePathSDA, brokenSymlink: false},
					gcpPVCID:            {path: devicePathSDB, brokenSymlink: false},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := newLinkCache()
			tt.setupCache(cache)

			if diff := cmp.Diff(tt.expected, cache, allowUnexportedLinkCache); diff != "" {
				t.Errorf("linkCache mismatch (-expected +got):\n%s", diff)
			}
		})
	}
}
