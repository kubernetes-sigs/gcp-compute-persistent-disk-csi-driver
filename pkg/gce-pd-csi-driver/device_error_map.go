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

package gceGCEDriver

import (
	"sync"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	"k8s.io/klog/v2"
)

// maxDeviceCacheSize specifies the maximum number if in-use devices to cache.
// 256 was selected since it is twice the number of max PDs per VM (128)
const maxDeviceCacheSize = 256

// currentTime is used to stub time.Now in unit tests
var currentTime = time.Now

// deviceErrMap is an atomic data datastructure for recording deviceInUseError times
// for specified devices
type deviceErrMap struct {
	timeout time.Duration
	mux     sync.Mutex
	cache   *expirable.LRU[string, time.Time]
}

func newDeviceErrMap(timeout time.Duration) *deviceErrMap {
	c := expirable.NewLRU[string, time.Time](maxDeviceCacheSize, nil, timeout*2)

	return &deviceErrMap{
		cache:   c,
		timeout: timeout,
	}
}

// deviceErrorExpired returns true if an error for the specified device is expired,
// where the expiration is specified by `--device-in-use-timeout`
func (devErrMap *deviceErrMap) deviceErrorExpired(deviceName string) bool {
	devErrMap.mux.Lock()
	defer devErrMap.mux.Unlock()

	firstEncounteredErrTime, exists := devErrMap.cache.Get(deviceName)
	if !exists {
		// If the deviceName does not exist in the map, then this is the first time
		// an error was encountered for that device. We return false since it cannot be
		// expired yet.
		return false
	}
	expirationTime := firstEncounteredErrTime.Add(devErrMap.timeout)
	return currentTime().After(expirationTime)
}

// markDeviceError updates the internal `cache` map to denote an error was encounted
// for the specified deviceName at the current time. If an error had previously been recorded, the
// time will not be updated.
func (devErrMap *deviceErrMap) markDeviceError(deviceName string) {
	devErrMap.mux.Lock()
	defer devErrMap.mux.Unlock()

	// If an earlier error has already been recorded, do not overwrite it
	if _, exists := devErrMap.cache.Get(deviceName); !exists {
		now := currentTime()
		klog.V(4).Infof("Recording in-use error for device %s at time %s", deviceName, now)
		devErrMap.cache.Add(deviceName, now)
	}
}

// deleteDevice removes a specified device name from the map
func (devErrMap *deviceErrMap) deleteDevice(deviceName string) {
	devErrMap.mux.Lock()
	defer devErrMap.mux.Unlock()
	devErrMap.cache.Remove(deviceName)
}
