package gceGCEDriver

import (
	"testing"
)

func TestFetchChunkSizeKiB(t *testing.T) {
	testCases := []struct {
		name         string
		cacheSize    string
		expChunkSize string
		expErr       bool
	}{
		{
			name:         "chunk size is in the allowed range",
			cacheSize:    "500GiB",
			expChunkSize: "512KiB", //range defined in fetchChunkSizeKiB
		},
		{
			name:         "chunk size is set to the range ceil",
			cacheSize:    "30000000GiB",
			expChunkSize: "1048576KiB", //range defined in fetchChunkSizeKiB - max 1GiB
		},
		{
			name:         "chunk size is set to the allowed range floor",
			cacheSize:    "100GiB",
			expChunkSize: "160KiB", //range defined in fetchChunkSizeKiB - min 160 KiB
		},
		{
			name:         "cacheSize set to KiB also sets the chunk size to range floor",
			cacheSize:    "1GiB",
			expChunkSize: "160KiB", //range defined in fetchChunkSizeKiB - min 160 KiB
		},
		{
			name:         "chunk size with GiB string parses correctly",
			cacheSize:    "375GiB",
			expChunkSize: "384KiB",
		},
		{
			name:         "invalid cacheSize",
			cacheSize:    "fdfsdKi",
			expChunkSize: "160KiB", //range defined in fetchChunkSizeKiB - min 160 KiB
			expErr:       true,
		},
		// cacheSize is validated in storage class parameter so assuming invalid cacheSize (like negative, 0) would not be passed to the function
	}

	for _, tc := range testCases {
		chunkSize, err := fetchChunkSizeKiB(tc.cacheSize)
		if err != nil {
			if !tc.expErr {
				t.Errorf("Errored %s", err)
			}
			continue
		}
		if chunkSize != tc.expChunkSize {
			t.Errorf("Got %s want %s", chunkSize, tc.expChunkSize)
		}

	}

}

func TestFetchNumberGiB(t *testing.T) {
	testCases := []struct {
		name        string
		stringInput []string
		expOutput   string // Outputs value in GiB
		expErr      bool
	}{
		{
			name:        "valid input 1",
			stringInput: []string{"5000000000B"},
			expOutput:   "5GiB", //range defined in fetchChunkSizeKiB
		},
		{
			name:        "valid input 2",
			stringInput: []string{"375000000000B"}, // 1 LSSD attached
			expOutput:   "350GiB",                  //range defined in fetchChunkSizeKiB
		},
		{
			name:        "valid input 3",
			stringInput: []string{"9000000000000B"}, // 24 LSSD attached
			expOutput:   "8382GiB",                  //range defined in fetchChunkSizeKiB
		},
		{
			name:        "valid input 4",
			stringInput: []string{"Some text before ", "9000000000000B", "Some text after"}, // 24 LSSD attached
			expOutput:   "8382GiB",                                                          //range defined in fetchChunkSizeKiB
		},
		{
			name:        "invalid input 1",
			stringInput: []string{"9000000000000"},
			expErr:      true,
		},
		{
			name:        "invalid input 2",
			stringInput: []string{"A9000000000000B"},
			expErr:      true,
		},
		{
			name:        "valid input 5",
			stringInput: []string{"900000B"}, // <1GiB gets rounded off to 0GiB
			expOutput:   "1GiB",
		},
	}

	for _, tc := range testCases {
		v, err := fetchNumberGiB(tc.stringInput)
		if err != nil {
			if !tc.expErr {
				t.Errorf("Errored %s", err)
			}
			continue
		}
		if v != tc.expOutput {
			t.Errorf("Got %s want %s", v, tc.expOutput)
		}

	}

}
