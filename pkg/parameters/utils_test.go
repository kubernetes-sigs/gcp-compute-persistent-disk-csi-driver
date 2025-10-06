package parameters

import (
	"reflect"
	"testing"
)

func TestSnapshotStorageLocations(t *testing.T) {
	tests := []struct {
		desc                        string
		locationString              string
		expectedNormalizedLocations []string
		expectError                 bool
	}{
		{
			"valid multi-region",
			"   uS ",
			[]string{"us"},
			false,
		},
		{
			"valid region",
			"  US-EAST1",
			[]string{"us-east1"},
			false,
		},
		{
			"valid region in large continent",
			"europe-west12",
			[]string{"europe-west12"},
			false,
		},
		{
			// Zones are not valid bucket/snapshot locations.
			"single zone",
			"us-east1-a",
			[]string{},
			true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			normalizedLocations, err := ProcessStorageLocations(tc.locationString)
			if err != nil && !tc.expectError {
				t.Errorf("Got error %v processing storage locations %q; expect no error", err, tc.locationString)
			}
			if err == nil && tc.expectError {
				t.Errorf("Got no error processing storage locations %q; expect an error", tc.locationString)
			}
			if err == nil && !reflect.DeepEqual(normalizedLocations, tc.expectedNormalizedLocations) {
				t.Errorf("Got %v for normalized storage locations; expect %v", normalizedLocations, tc.expectedNormalizedLocations)
			}
		})
	}
}
