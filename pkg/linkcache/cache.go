package linkcache

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"k8s.io/klog/v2"
)

var partitionNameRegex = regexp.MustCompile(`-part[0-9]+$`)

// fsInterface defines the filesystem operations needed by ListingCache
type fsInterface interface {
	ReadDir(name string) ([]os.DirEntry, error)
	EvalSymlinks(path string) (string, error)
}

// realFS implements fsInterface using the real filesystem
type realFS struct{}

func (f *realFS) ReadDir(name string) ([]os.DirEntry, error) {
	return os.ReadDir(name)
}

func (f *realFS) EvalSymlinks(path string) (string, error) {
	return filepath.EvalSymlinks(path)
}

// ListingCache polls the filesystem at the specified directory once per
// period and checks each non-directory entry for a symlink. The results are
// cached. Changes to the cache are logged, as well as the full contents of the
// cache. The cache's Run() method is expected to be called in a goroutine. Its
// cancellation is controlled via the context argument.
type ListingCache struct {
	period time.Duration
	dir    string
	links  *linkCache
	fs     fsInterface
}

func NewListingCache(period time.Duration, dir string) *ListingCache {
	return &ListingCache{
		period: period,
		dir:    dir,
		links:  newLinkCache(),
		fs:     &realFS{},
	}
}

// Run starts the cache's background loop. The filesystem is listed and the cache
// updated according to the frequency specified by the period. It will run until
// the context is cancelled.
func (l *ListingCache) Run(ctx context.Context) {
	// Start the loop that runs every minute
	ticker := time.NewTicker(l.period)
	defer ticker.Stop()

	// Initial list and update so we don't wait for the first tick.
	err := l.listAndUpdate()
	if err != nil {
		klog.Warningf("Error listing and updating symlinks: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			klog.Infof("Context done, stopping watcher")
			return
		case <-ticker.C:
			err := l.listAndUpdate()
			if err != nil {
				klog.Warningf("Error listing and updating symlinks: %v", err)
				continue
			}

			klog.Infof("periodic symlink cache read: %s", l.links.String())
		}
	}
}

func (l *ListingCache) listAndUpdate() error {
	visited := make(map[string]struct{})

	entries, err := l.fs.ReadDir(l.dir)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %w", l.dir, err)
	}

	var errs []error
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		diskByIdPath := filepath.Join(l.dir, entry.Name())

		if partitionNameRegex.MatchString(entry.Name()) {
			continue
		}

		// Add the device to the map regardless of successful symlink eval.
		// Otherwise, a broken symlink will lead us to remove it from the cache.
		visited[diskByIdPath] = struct{}{}

		realFSPath, err := l.fs.EvalSymlinks(diskByIdPath)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to evaluate symlink for %s: %w", diskByIdPath, err))
			l.links.BrokenSymlink(diskByIdPath)
			continue
		}

		l.links.AddOrUpdateDevice(diskByIdPath, realFSPath)
	}

	for _, id := range l.links.DeviceIDs() {
		if _, found := visited[id]; !found {
			l.links.RemoveDevice(id)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to evaluate symlinks for %d devices: %v", len(errs), errs)
	}
	return nil
}

// linkCache is a structure that maintains a cache of symlinks between
// /dev/disk/by-id and /dev/sd* paths. It provides methods to add/update,
// retrieve, and remove device symlinks from the cache.
type linkCache struct {
	devices map[string]linkCacheEntry
}

type linkCacheEntry struct {
	path string
	// If true, the symlink is known to be broken.
	brokenSymlink bool
}

func newLinkCache() *linkCache {
	return &linkCache{
		devices: make(map[string]linkCacheEntry),
	}
}

func (d *linkCache) AddOrUpdateDevice(symlink, realPath string) {
	// Ignore partitions, which are noise as far as our logging is concerned.
	// Expression: -part[0-9]+$
	if partitionNameRegex.MatchString(symlink) {
		// TODO(juliankatz): To have certainty this works for all edge cases, we
		// need to test this with a manually partitioned disk.
		return
	}

	prevEntry, exists := d.devices[symlink]
	if !exists || prevEntry.path != realPath {
		klog.Infof("Symlink updated for link %s, previous value: %s, new value: %s", symlink, prevEntry.path, realPath)
	}
	d.devices[symlink] = linkCacheEntry{path: realPath, brokenSymlink: false}
}

// BrokenSymlink marks a symlink as broken.  If the symlink is not in the cache,
// it is ignored.
func (d *linkCache) BrokenSymlink(symlink string) {
	if entry, ok := d.devices[symlink]; ok {
		entry.brokenSymlink = true
		d.devices[symlink] = entry
	}
}

func (d *linkCache) RemoveDevice(symlink string) {
	if entry, ok := d.devices[symlink]; ok {
		klog.Infof("Removing device %s with path %s from cache, brokenSymlink: %t", symlink, entry.path, entry.brokenSymlink)
		delete(d.devices, symlink)
	}
}

func (d *linkCache) DeviceIDs() []string {
	return slices.Collect(maps.Keys(d.devices))
}

func (d *linkCache) String() string {
	var sb strings.Builder
	for symlink, entry := range d.devices {
		if entry.brokenSymlink {
			sb.WriteString(fmt.Sprintf("%s -> broken symlink... last known value: %s; ", symlink, entry.path))
		} else {
			sb.WriteString(fmt.Sprintf("%s -> %s; ", symlink, entry.path))
		}
	}
	return strings.TrimSuffix(sb.String(), "; ")
}
