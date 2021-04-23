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

import (
	"k8s.io/mount-utils"
	"k8s.io/utils/exec"
	testingexec "k8s.io/utils/exec/testing"
)

var (
	fakeMounter = &mount.FakeMounter{MountPoints: []mount.MountPoint{}}
	fakeExec    = &testingexec.FakeExec{DisableScripts: true}
)

func NewFakeSafeMounter() *mount.SafeFormatAndMount {
	return NewCustomFakeSafeMounter(fakeMounter, fakeExec)
}

func NewFakeSafeMounterWithCustomExec(exec exec.Interface) *mount.SafeFormatAndMount {
	fakeMounter := &mount.FakeMounter{MountPoints: []mount.MountPoint{}}
	return NewCustomFakeSafeMounter(fakeMounter, exec)
}

func NewCustomFakeSafeMounter(mounter mount.Interface, exec exec.Interface) *mount.SafeFormatAndMount {
	return &mount.SafeFormatAndMount{
		Interface: mounter,
		Exec:      exec,
	}
}

type FakeBlockingMounter struct {
	*mount.FakeMounter
	ReadyToExecute chan chan struct{}
}

// Mount is overridden and adds functionality to finely control the order of execution of FakeMounter's Mount calls.
// Upon starting a Mount, it passes a chan 'executeMount' into readyToExecute, then blocks on executeMount.
// The test calling this function can block on readyToExecute to ensure that the operation has started and
// allowed the Mount to continue by passing a struct into executeMount.
func (mounter *FakeBlockingMounter) Mount(source string, target string, fstype string, options []string) error {
	executeMount := make(chan struct{})
	mounter.ReadyToExecute <- executeMount
	<-executeMount
	return mounter.FakeMounter.Mount(source, target, fstype, options)
}

func NewFakeSafeBlockingMounter(readyToExecute chan chan struct{}) *mount.SafeFormatAndMount {
	fakeBlockingMounter := &FakeBlockingMounter{
		FakeMounter:    fakeMounter,
		ReadyToExecute: readyToExecute,
	}
	return &mount.SafeFormatAndMount{
		Interface: fakeBlockingMounter,
	}
}
