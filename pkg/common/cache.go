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
	"sync"
)

type OpInfo struct {
	Name string
	Type string
}

type DiskInstanceOpsMap struct {
	ops map[string]OpInfo
}

func NewDiskInstanceOpsMap() *DiskInstanceOpsMap {
	return &DiskInstanceOpsMap{
		ops: make(map[string]OpInfo),
	}
}

func (c *DiskInstanceOpsMap) GetOp(key string) *OpInfo {
	op, ok := c.ops[key]
	if !ok {
		return nil
	}
	return &op
}

func (c *DiskInstanceOpsMap) ClearOp(key string, targetopName string) error {
	op, ok := c.ops[key]
	if !ok {
		return nil
	}

	if op.Name != targetopName {
		return fmt.Errorf("For key %q, cannot clear op %q, cache already contains op %q", key, targetopName, op.Name)
	}

	delete(c.ops, key)
	return nil
}

func (c *DiskInstanceOpsMap) AddOp(key string, targetop OpInfo) {
	c.ops[key] = targetop
}

type InstanceOpsMap struct {
	ops map[string][]OpInfo
}

func NewInstanceOpsMap() *InstanceOpsMap {
	return &InstanceOpsMap{
		ops: make(map[string][]OpInfo),
	}
}

func (c *InstanceOpsMap) GetOps(key string) []OpInfo {
	v, ok := c.ops[key]
	if !ok {
		return nil
	}
	return v
}

func (c *InstanceOpsMap) ClearOp(key string, targetOpName string) {
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

func (c *InstanceOpsMap) AddOp(key string, targetop OpInfo) {
	ops, ok := c.ops[key]
	if !ok {
		c.ops[key] = []OpInfo{targetop}
		return
	}

	for _, o := range ops {
		if o.Name == targetop.Name {
			return
		}
	}

	ops = append(ops, targetop)
	c.ops[key] = ops
}

type OpsCache struct {
	sync.Mutex
	InstanceOps     *InstanceOpsMap
	DiskInstanceOps *DiskInstanceOpsMap
	Ready           bool
}

func NewOpsCache() *OpsCache {
	return &OpsCache{
		InstanceOps:     NewInstanceOpsMap(),
		DiskInstanceOps: NewDiskInstanceOpsMap(),
	}
}

func (c *OpsCache) ClearDiskInstanceOpSafe(key, opName string) error {
	c.Lock()
	defer c.Unlock()
	return c.DiskInstanceOps.ClearOp(key, opName)
}

func (c *OpsCache) IsReady() bool {
	c.Lock()
	defer c.Unlock()
	return c.Ready
}

func CreateDiskInstanceKey(project, location, disk, instance string) string {
	return fmt.Sprintf("%s_%s_%s_%s", project, location, disk, instance)
}

func CreateInstanceKey(project, location, instance string) string {
	return fmt.Sprintf("%s_%s_%s", project, location, instance)
}
