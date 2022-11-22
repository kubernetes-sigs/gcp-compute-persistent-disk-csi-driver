package deviceutils

import (
	"testing"
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
