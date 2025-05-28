package linkcache

import "time"

type deviceMapping struct {
	symlink  string
	realPath string
}

type DeviceCache struct {
	volumes map[string]deviceMapping
	period  time.Duration
	// dir is the directory to look for device symlinks
	dir string
}
