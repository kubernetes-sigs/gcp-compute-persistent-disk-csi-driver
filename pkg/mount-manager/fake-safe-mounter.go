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

import "k8s.io/kubernetes/pkg/util/mount"

func NewFakeSafeMounter() *mount.SafeFormatAndMount {
	execCallback := func(cmd string, args ...string) ([]byte, error) {
		return nil, nil
		// TODO: Fill out exec callback for errors
		/*
			if len(test.execScripts) <= execCallCount {
				t.Errorf("Unexpected command: %s %v", cmd, args)
				return nil, nil
			}
			script := test.execScripts[execCallCount]
			execCallCount++
			if script.command != cmd {
				t.Errorf("Unexpected command %s. Expecting %s", cmd, script.command)
			}
			for j := range args {
				if args[j] != script.args[j] {
					t.Errorf("Unexpected args %v. Expecting %v", args, script.args)
				}
			}
			return []byte(script.output), script.err
		*/
	}
	fakeMounter := &mount.FakeMounter{MountPoints: []mount.MountPoint{}, Log: []mount.FakeAction{}}
	fakeExec := mount.NewFakeExec(execCallback)
	return &mount.SafeFormatAndMount{
		Interface: fakeMounter,
		Exec:      fakeExec,
	}
}
