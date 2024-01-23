package mountmanager

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestParseNvmeSerial(t *testing.T) {
	testCases := []struct {
		name   string
		output string
		serial string
		expErr bool
	}{
		{
			name:   "valid google_nvme_id response",
			output: "line 58: warning: command substitution: ignored null byte in input\nID_SERIAL_SHORT=pvc-8ee0cf44-6acd-456e-9f3b-95ccd65065b9\nID_SERIAL=Google_PersistentDisk_pvc-8ee0cf44-6acd-456e-9f3b-95ccd65065b9",
			serial: "pvc-8ee0cf44-6acd-456e-9f3b-95ccd65065b9",
			expErr: false,
		},
		{
			name:   "valid google_nvme_id boot disk response",
			output: "line 58: warning: command substitution: ignored null byte in input\nID_SERIAL_SHORT=persistent-disk-0\nID_SERIAL=Google_PersistentDisk_persistent-disk-0",
			serial: "persistent-disk-0",
			expErr: false,
		},
		{
			name:   "invalid google_nvme_id response",
			output: "Error: requesting namespace-id from non-block device\nNVMe Status:INVALID_NS: The namespace or the format of that namespace is invalid(b) NSID:0\nxxd: sorry cannot seek.\n[2022-03-12T04:17:17+0000]: NVMe Vendor Extension disk information not present",
			serial: "",
			expErr: true,
		},
	}

	for _, tc := range testCases {
		t.Logf("Running test: %v", tc.name)
		actualSerial, err := parseNvmeSerial(tc.output)
		if tc.expErr && err == nil {
			t.Fatalf("Expected error but didn't get any")
		}
		if !tc.expErr && err != nil {
			t.Fatalf("Got unexpected error: %s", err)
		}
		if actualSerial != tc.serial {
			t.Fatalf("Expected '%s' but got '%s'", tc.serial, actualSerial)
		}
	}
}

func less(a, b fmt.Stringer) bool {
	return a.String() < b.String()
}

// Test that the NVMe Regex matches expected paths
// Note that this only tests the regex, not the actual path finding codepath.
// The real codepath uses filepath.Glob(), which doesn't have an easy way to mock out local
// directory paths (eg: using a subdirectory prefix).
// We could use a recursive child process, setting a root UID, and use Chroot (which requires superuser).
// This is done in upstream golang tests, but this adds additional complexity
// and may prevent our tests from running on all platforms. See the following test for an example:
// https://github.com/golang/go/blob/d33548d178016122726342911f8e15016a691472/src/syscall/exec_linux_test.go#L250
func TestDiskNvmePattern(t *testing.T) {
	nvmeDiskRegex := regexp.MustCompile(diskNvmePattern)

	testCases := []struct {
		paths     []string
		wantPaths []string
	}{
		{
			paths: []string{
				"/dev/nvme0n1p15",
				"/dev/nvme0n1p14",
				"/dev/nvme0n1p1",
				"/dev/nvme0n1",
				"/dev/nvme0n2",
				"/dev/nvme0",
			},
			wantPaths: []string{
				"/dev/nvme0n1",
				"/dev/nvme0n2",
			},
		},
		{
			paths: []string{
				"/dev/nvme1",
				"/dev/nvme0n1p15",
				"/dev/nvme0n1p14",
				"/dev/nvme0n1p1",
				"/dev/nvme2",
			},
			wantPaths: []string{},
		},
	}

	for _, tc := range testCases {
		gotPaths := []string{}
		for _, path := range tc.paths {
			if nvmeDiskRegex.MatchString(path) {
				gotPaths = append(gotPaths, path)
			}
		}
		if diff := cmp.Diff(gotPaths, tc.wantPaths, cmpopts.SortSlices(less)); diff != "" {
			t.Errorf("Unexpected NVMe device paths (-got, +want):\n%s", diff)
		}
	}
}
