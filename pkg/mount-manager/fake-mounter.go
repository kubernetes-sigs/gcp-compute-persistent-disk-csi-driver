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

package mountmanager

import "os"

type FakeMounter struct {
}

func CreateFakeMounter() (*FakeMounter, error) {
	return &FakeMounter{}, nil
}

func (m *FakeMounter) DoMount(source string, target string, fstype string, options []string) error {
	//TODO(dyzz): Unimplemented
	return nil
}

func (m *FakeMounter) FormatAndMount(source string, target string, fstype string, options []string) error {
	//TODO(dyzz): Unimplemented
	return nil
}

func (m *FakeMounter) IsLikelyNotMountPoint(file string) (bool, error) {
	//TODO(dyzz): Unimplemented
	return true, nil
}

func (m *FakeMounter) UnmountVolume(targetPath string) (string, error) {
	//TODO(dyzz): Unimplemented
	return "TODO", nil
}

func (m *FakeMounter) GetDiskByIdPaths(pdName string, partition string) []string {
	//TODO(dyzz): Unimplemented
	return []string{}
}

func (m *FakeMounter) VerifyDevicePath(devicePaths []string) (string, error) {
	//TODO(dyzz): Unimplemented
	return "TODO", nil
}

func (m *FakeMounter) MkdirAll(path string, perm os.FileMode) error {
	//TODO(dyzz): Unimplemented
	return nil
}

func (m *FakeMounter) Remove(name string) error {
	//TODO(dyzz): Unimplemented
	return nil
}
