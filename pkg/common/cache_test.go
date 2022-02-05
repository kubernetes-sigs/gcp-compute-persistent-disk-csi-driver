/*
Copyright 2022 The Kubernetes Authors.

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

package common

import (
	"testing"
)

type InstanceInfo struct {
	Project      string
	Location     string
	InstanceName string
}

const (
	testproject      = "myproject"
	testinstance     = "myinstance"
	testdisk         = "mydisk"
	testlocation     = "us-central1-c"
	testop           = "op1"
	attachDiskOpType = "attachDisk"
	detachDiskOpType = "detachDisk"
)

func TestKey(t *testing.T) {
	expectedDiskInstanceKey := testproject + "_" + testlocation + "_" + testdisk + "_" + testinstance
	if CreateDiskInstanceKey(testproject, testlocation, testdisk, testinstance) != expectedDiskInstanceKey {
		t.Errorf("mismatch in key")
	}

	expectedInstanceKey := testproject + "_" + testlocation + "_" + testinstance
	if CreateInstanceKey(testproject, testlocation, testinstance) != expectedInstanceKey {
		t.Errorf("mismatch in key")
	}
}

func TestInstanceOpMap(t *testing.T) {
	// Add operation tests
	tests := []struct {
		name        string
		inputOps    []OpInfo
		expectedOps map[string]bool
	}{
		{
			name: "add single op",
			inputOps: []OpInfo{
				{Name: testop, Type: attachDiskOpType},
			},
			expectedOps: map[string]bool{
				testop: true,
			},
		},
		{
			name: "add unique ops",
			inputOps: []OpInfo{
				{Name: "op-1", Type: attachDiskOpType},
				{Name: "op-2", Type: attachDiskOpType},
				{Name: "op-3", Type: attachDiskOpType},
			},
			expectedOps: map[string]bool{
				"op-1": true,
				"op-2": true,
				"op-3": true,
			},
		},
		{
			name: "add duplicate ops, items should be deduped in map",
			inputOps: []OpInfo{
				{Name: "op-1", Type: attachDiskOpType},
				{Name: "op-1", Type: attachDiskOpType},
				{Name: "op-2", Type: attachDiskOpType},
				{Name: "op-2", Type: attachDiskOpType},
			},
			expectedOps: map[string]bool{
				"op-1": true,
				"op-2": true,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := NewOpsCache()
			dummykey := "dummykey"
			for _, op := range tc.inputOps {
				c.InstanceOps.AddOp(dummykey, op)
			}

			if len(c.InstanceOps.GetOps(dummykey)) != len(tc.expectedOps) {
				t.Errorf("mismatch in expected number of ops in map")
			}

			for _, op := range c.InstanceOps.GetOps(dummykey) {
				if _, ok := tc.expectedOps[op.Name]; !ok {
					t.Errorf("unexpected op %q", op.Name)
				}
			}
		})
	}

	// Clear operation tests
	tests1 := []struct {
		name            string
		inputOps        []OpInfo
		clearOps        []string
		expectedOps     map[string]bool
		expectKeyRemove bool
	}{
		{
			name:        "empty input map, clear item is a no-op",
			inputOps:    []OpInfo{},
			clearOps:    []string{"op-1", "op-2"},
			expectedOps: map[string]bool{},
		},
		{
			name: "non-empty input map, clear item for non-existent key is no-op",
			inputOps: []OpInfo{
				{Name: "op-1", Type: attachDiskOpType},
				{Name: "op-2", Type: attachDiskOpType},
			},
			clearOps: []string{"op-3"},
			expectedOps: map[string]bool{
				"op-1": true,
				"op-2": true,
			},
		},
		{
			name: "non-empty input map, clear item for existent key, 0 remaining values",
			inputOps: []OpInfo{
				{Name: "op-1", Type: attachDiskOpType},
				{Name: "op-2", Type: attachDiskOpType},
			},
			clearOps:        []string{"op-1", "op-2"},
			expectedOps:     map[string]bool{},
			expectKeyRemove: true,
		},
		{
			name: "non-empty input map, clear item for existent key, non 0 remaining values",
			inputOps: []OpInfo{
				{Name: "op-1", Type: attachDiskOpType},
				{Name: "op-2", Type: attachDiskOpType},
			},
			clearOps: []string{"op-1"},
			expectedOps: map[string]bool{
				"op-2": true,
			},
		},
	}
	for _, tc := range tests1 {
		t.Run(tc.name, func(t *testing.T) {
			c := NewOpsCache()
			dummykey := "dummykey"
			for _, op := range tc.inputOps {
				c.InstanceOps.AddOp(dummykey, op)
			}

			for _, op := range tc.clearOps {
				c.InstanceOps.ClearOp(dummykey, op)
			}

			if tc.expectKeyRemove {
				_, ok := c.InstanceOps.ops[dummykey]
				if ok {
					t.Errorf("unexpected key found")
				}
			}

			if len(c.InstanceOps.GetOps(dummykey)) != len(tc.expectedOps) {
				t.Errorf("mismatch in expected number of ops in map")
			}
			for _, op := range c.InstanceOps.GetOps(dummykey) {
				if _, ok := tc.expectedOps[op.Name]; !ok {
					t.Errorf("unexpected op %q", op.Name)
				}
			}
		})
	}
}

func TestDiskInstanceOpMap(t *testing.T) {
	// Add operation tests
	type CacheEntry struct {
		Key string
		Op  OpInfo
	}
	tests := []struct {
		name             string
		inputEntries     []CacheEntry
		clearEntry       CacheEntry
		expectedEntries  map[string]OpInfo
		expectClearOpErr bool
	}{
		{
			name: "clear called on empty map",
			clearEntry: CacheEntry{
				Key: "key1",
				Op: OpInfo{
					Name: "op-1",
					Type: attachDiskOpType,
				},
			},
		},
		{
			name: "clear called on non-empty map",
			inputEntries: []CacheEntry{
				{
					Key: "key1",
					Op: OpInfo{
						Name: "op-1",
						Type: attachDiskOpType,
					},
				},
				{
					Key: "key2",
					Op: OpInfo{
						Name: "op-2",
						Type: attachDiskOpType,
					},
				},
			},
			clearEntry: CacheEntry{
				Key: "key1",
				Op: OpInfo{
					Name: "op-1",
				},
			},
			expectedEntries: map[string]OpInfo{
				"key2": {
					Name: "op-2",
					Type: attachDiskOpType,
				},
			},
		},
		{
			name: "clear called on a key with different op name, error expected",
			inputEntries: []CacheEntry{
				{
					Key: "key1",
					Op: OpInfo{
						Name: "op-1",
						Type: attachDiskOpType,
					},
				},
				{
					Key: "key2",
					Op: OpInfo{
						Name: "op-2",
						Type: attachDiskOpType,
					},
				},
			},
			clearEntry: CacheEntry{
				Key: "key1",
				Op: OpInfo{
					Name: "op-3",
				},
			},
			expectedEntries: map[string]OpInfo{
				"key1": {
					Name: "op-1",
					Type: attachDiskOpType,
				},
				"key2": {
					Name: "op-2",
					Type: attachDiskOpType,
				},
			},
			expectClearOpErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := NewOpsCache()
			for _, e := range tc.inputEntries {
				c.DiskInstanceOps.AddOp(e.Key, e.Op)
			}

			err := c.DiskInstanceOps.ClearOp(tc.clearEntry.Key, tc.clearEntry.Op.Name)
			if tc.expectClearOpErr && err == nil {
				t.Errorf("Expected error not found")
			}
			if !tc.expectClearOpErr && err != nil {
				t.Errorf("Unexpected error found")
			}

			if len(tc.expectedEntries) != len(c.DiskInstanceOps.ops) {
				t.Errorf("Unexpected number of keys found")
			}

			for k, v := range c.DiskInstanceOps.ops {
				expectedV, ok := tc.expectedEntries[k]
				if !ok {
					t.Errorf("unexpected key %q", k)
				}
				if v.Name != expectedV.Name || v.Type != expectedV.Type {
					t.Errorf("unexpected value %q, %q", v.Name, v.Type)
				}
			}
		})
	}
}
