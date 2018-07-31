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

package common

import (
	"fmt"
	"strings"
)

func BytesToGb(bytes int64) int64 {
	// TODO: Throw an error when div to 0
	return bytes / (1024 * 1024 * 1024)
}

func GbToBytes(Gb int64) int64 {
	// TODO: Check for overflow
	return Gb * 1024 * 1024 * 1024
}

func SplitZoneNameId(id string) (string, string, error) {
	splitId := strings.Split(id, "/")
	if len(splitId) != 2 {
		return "", "", fmt.Errorf("Failed to get id components. Expected {zone}/{name}. Got: %s", id)
	}
	return splitId[0], splitId[1], nil
}

func CombineVolumeId(zone, name string) string {
	return fmt.Sprintf("%s/%s", zone, name)
}
