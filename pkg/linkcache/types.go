package linkcache

import (
	"sync"
	"time"

	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/deviceutils"
)

type deviceMapping struct {
	volumeID string
	realPath string
}

type DeviceCache struct {
	mutex    sync.Mutex
	symlinks map[string]deviceMapping
	period   time.Duration
	// dir is the directory to look for device symlinks
	dir         string
	deviceUtils deviceutils.DeviceUtils
}
