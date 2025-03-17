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
	"testing"
	"time"
)

func TestDeviceErrorMap(t *testing.T) {
	timeout := time.Second * 10
	dName := "fake-device"
	eMap := newDeviceErrMap(timeout)
	defer func() { currentTime = time.Now }()

	// Register an error. Checking the timeout right after should return false
	stubCurrentTime(0)
	eMap.markDeviceError(dName)
	isTimedOut := eMap.deviceErrorExpired(dName)
	if isTimedOut {
		t.Errorf("checkDeviceErrorTimeout expected to be false if called immediately after marking an error")
	}

	// Advance time. Checking the timeout should now return true
	stubCurrentTime(int64(timeout.Seconds()) + 1)
	isTimedOut = eMap.deviceErrorExpired(dName)
	if !isTimedOut {
		t.Errorf("checkDeviceErrorTimeout expected to be true after waiting for timeout")
	}

	// Deleting the device and checking the timout should return false
	eMap.deleteDevice(dName)
	isTimedOut = eMap.deviceErrorExpired(dName)
	if isTimedOut {
		t.Errorf("checkDeviceErrorTimeout expected to be false after deleting device from map")
	}
}

func stubCurrentTime(unixTime int64) {
	currentTime = func() time.Time {
		return time.Unix(unixTime, 0)
	}
}
