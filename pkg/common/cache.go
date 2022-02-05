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
	"fmt"
)

type DiskInstanceKey string
type InstanceKey string

type OpInfo struct {
	Name string
	Type string
}

type DiskInstanceOpsMap struct {
	ops map[DiskInstanceKey]OpInfo
}

func NewDiskInstanceOpsMap() *DiskInstanceOpsMap {
	return &DiskInstanceOpsMap{
		ops: make(map[DiskInstanceKey]OpInfo),
	}
}

func (c *DiskInstanceOpsMap) GetOp(key DiskInstanceKey) *OpInfo {
	op, ok := c.ops[key]
	if !ok {
		return nil
	}
	return &op
}

func (c *DiskInstanceOpsMap) ClearOp(key DiskInstanceKey, targetOpName string) error {
	op, ok := c.ops[key]
	if !ok {
		return nil
	}

	if op.Name != targetOpName {
		return fmt.Errorf("For key %q, cannot clear op %q, cache already contains op %q", key, targetOpName, op.Name)
	}

	delete(c.ops, key)
	return nil
}

func (c *DiskInstanceOpsMap) AddOp(key DiskInstanceKey, targetOp OpInfo) {
	c.ops[key] = targetOp
}

type InstanceOpsMap struct {
	ops map[InstanceKey][]OpInfo
}

func NewInstanceOpsMap() *InstanceOpsMap {
	return &InstanceOpsMap{
		ops: make(map[InstanceKey][]OpInfo),
	}
}

func (c *InstanceOpsMap) GetOps(key InstanceKey) []OpInfo {
	v, ok := c.ops[key]
	if !ok {
		return nil
	}
	return v
}

func (c *InstanceOpsMap) ClearOp(key InstanceKey, targetOpName string) {
	ops, ok := c.ops[key]
	if !ok {
		return
	}

	var filteredOps []OpInfo
	for _, o := range ops {
		if o.Name != targetOpName {
			filteredOps = append(filteredOps, o)
		}
	}
	if len(filteredOps) == 0 {
		delete(c.ops, key)
		return
	}

	c.ops[key] = filteredOps
}

func (c *InstanceOpsMap) AddOp(key InstanceKey, targetOp OpInfo) {
	ops, ok := c.ops[key]
	if !ok {
		c.ops[key] = []OpInfo{targetOp}
		return
	}

	for _, o := range ops {
		if o.Name == targetOp.Name {
			return
		}
	}

	ops = append(ops, targetOp)
	c.ops[key] = ops
}

type OpsCache struct {
	InstanceOps     *InstanceOpsMap
	DiskInstanceOps *DiskInstanceOpsMap
}

func NewOpsCache() *OpsCache {
	return &OpsCache{
		InstanceOps:     NewInstanceOpsMap(),
		DiskInstanceOps: NewDiskInstanceOpsMap(),
	}
}

func CreateDiskInstanceKey(project, location, disk, instance string) DiskInstanceKey {
	return DiskInstanceKey(fmt.Sprintf("%s_%s_%s_%s", project, location, disk, instance))
}

func CreateInstanceKey(project, location, instance string) InstanceKey {
	return InstanceKey(fmt.Sprintf("%s_%s_%s", project, location, instance))
}
