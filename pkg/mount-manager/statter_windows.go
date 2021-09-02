//go:build windows
// +build windows

/*
Copyright 2019 The Kubernetes Authors.

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

package mountmanager

import (
	"fmt"
)

var _ Statter = realStatter{}

type realStatter struct {
}

func NewStatter() realStatter {
	return realStatter{}
}

// IsBlock checks if the given path is a block device
func (realStatter) IsBlockDevice(fullPath string) (bool, error) {
	return false, nil
}

//TODO (jinxu): implement StatFS to get metrics
func (realStatter) StatFS(path string) (available, capacity, used, inodesFree, inodes, inodesUsed int64, err error) {
	zero := int64(0)
	return zero, zero, zero, zero, zero, zero, fmt.Errorf("Not implemented")
}

type fakeStatter struct{}

func NewFakeStatter() fakeStatter {
	return fakeStatter{}
}

func (fakeStatter) StatFS(path string) (available, capacity, used, inodesFree, inodes, inodesUsed int64, err error) {
	// Assume the file exists and give some dummy values back
	return 1, 1, 1, 1, 1, 1, nil
}

func (fakeStatter) IsBlockDevice(fullPath string) (bool, error) {
	return false, nil
}
