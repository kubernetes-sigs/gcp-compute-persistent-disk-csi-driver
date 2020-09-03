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

package main

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"k8s.io/klog"
)

/*
 * TestSuite represents a JUnit file. Due to how encoding/xml works, we have
 * represent all fields that we want to be passed through. It's therefore
 * not a complete solution, but good enough for Ginkgo + Spyglass.
 */
type TestSuite struct {
	XMLName   string     `xml:"testsuite"`
	TestCases []TestCase `xml:"testcase"`
}

type TestCase struct {
	Name      string     `xml:"name,attr"`
	Time      string     `xml:"time,attr"`
	SystemOut string     `xml:"system-out,omitempty"`
	Failure   string     `xml:"failure,omitempty"`
	Skipped   SkipReason `xml:"skipped,omitempty"`
}

// SkipReason deals with the special <skipped></skipped>:
// if present, we must re-encode it, even if empty.
type SkipReason string

func (s *SkipReason) UnmarshalText(text []byte) error {
	*s = SkipReason(text)
	if *s == "" {
		*s = " "
	}
	return nil
}

func (s SkipReason) MarshalText() ([]byte, error) {
	if s == " " {
		return []byte{}, nil
	}
	return []byte(s), nil
}

// MergeJUnit merges all junit xml files found in sourceDirectories into a single xml file at destination, using the filter.
// The merging removes duplicate skipped tests. The original files are deleted.
func MergeJUnit(testFilter string, sourceDirectories []string, destination string) error {
	var junit TestSuite
	var data []byte

	re := regexp.MustCompile(testFilter)

	var mergeErrors []string
	var filesToDelete []string
	for _, dir := range sourceDirectories {
		files, err := ioutil.ReadDir(dir)
		if err != nil {
			klog.Errorf("Failed to read juint directory %s: %v", dir, err)
			mergeErrors = append(mergeErrors, err.Error())
			continue
		}
		for _, file := range files {
			if !strings.HasSuffix(file.Name(), ".xml") {
				continue
			}
			fullFilename := filepath.Join(dir, file.Name())
			filesToDelete = append(filesToDelete, fullFilename)
			data, err := ioutil.ReadFile(fullFilename)
			if err != nil {
				return err
			}
			if err = xml.Unmarshal(data, &junit); err != nil {
				return err
			}
		}
	}

	// Keep only matching testcases. Testcases skipped in all test runs are only stored once.
	filtered := map[string]TestCase{}
	for _, testcase := range junit.TestCases {
		if !re.MatchString(testcase.Name) {
			continue
		}
		entry, ok := filtered[testcase.Name]
		if !ok || // not present yet
			entry.Skipped != "" && testcase.Skipped == "" { // replaced skipped test with real test run
			filtered[testcase.Name] = testcase
		}
	}
	junit.TestCases = nil
	for _, testcase := range filtered {
		junit.TestCases = append(junit.TestCases, testcase)
	}

	// Re-encode.
	data, err := xml.MarshalIndent(junit, "", "  ")
	if err != nil {
		return err
	}

	if err = ioutil.WriteFile(destination, data, 0644); err != nil {
		return err
	}

	if mergeErrors != nil {
		return fmt.Errorf("Problems reading junit files; partial merge has been performed: %s", strings.Join(mergeErrors, " "))
	} else {
		// Only delete original files if everything went well.
		var removeErrors []string
		for _, filename := range filesToDelete {
			if err := os.Remove(filename); err != nil {
				removeErrors = append(removeErrors, err.Error())
			}
		}
		if removeErrors != nil {
			return fmt.Errorf("Problem removing original junit results: %s", strings.Join(removeErrors, " "))
		}
	}
	return nil
}
