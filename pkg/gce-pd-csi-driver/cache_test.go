package gceGCEDriver

import (
	"os"
	"path/filepath"
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

func TestIsValidVGName(t *testing.T) {
	testCases := []struct {
		name     string
		vgName   string
		expected bool
	}{
		{
			name:     "valid simple name",
			vgName:   "csi-vg-nndw98vv",
			expected: true,
		},
		{
			name:     "valid name with all characters",
			vgName:   "a-z_A-Z_0-9_._-_+",
			expected: true,
		},
		{
			name:     "empty name",
			vgName:   "",
			expected: false,
		},
		{
			name:     "invalid name with spaces",
			vgName:   "csi vg nndw98vv",
			expected: false,
		},
		{
			name:     "invalid name with warning prefix",
			vgName:   "WARNING: VG csi-vg-nndw98vv is missing PV",
			expected: false,
		},
		{
			name:     "invalid name with slash",
			vgName:   "/dev/md127",
			expected: false,
		},
		{
			name:     "invalid name with parenthesis",
			vgName:   "md127)",
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := isValidVGName(tc.vgName)
			if actual != tc.expected {
				t.Errorf("isValidVGName(%q) = %v; want %v", tc.vgName, actual, tc.expected)
			}
		})
	}
}

func TestEnsureMdadmRunDir(t *testing.T) {
	origDir := mdadmRunDir
	t.Cleanup(func() { mdadmRunDir = origDir })

	t.Run("creates missing nested directory", func(t *testing.T) {
		mdadmRunDir = filepath.Join(t.TempDir(), "run", "mdadm")
		if err := ensureMdadmRunDir(); err != nil {
			t.Fatalf("ensureMdadmRunDir() returned error: %v", err)
		}
		info, err := os.Stat(mdadmRunDir)
		if err != nil {
			t.Fatalf("expected %q to exist: %v", mdadmRunDir, err)
		}
		if !info.IsDir() {
			t.Fatalf("expected %q to be a directory", mdadmRunDir)
		}
	})

	t.Run("is a no-op when directory already exists", func(t *testing.T) {
		mdadmRunDir = t.TempDir()
		if err := ensureMdadmRunDir(); err != nil {
			t.Fatalf("ensureMdadmRunDir() returned error: %v", err)
		}
	})

	t.Run("fails when path is a regular file", func(t *testing.T) {
		mdadmRunDir = filepath.Join(t.TempDir(), "not-a-dir")
		if err := os.WriteFile(mdadmRunDir, []byte("x"), 0600); err != nil {
			t.Fatalf("failed to create file: %v", err)
		}
		if err := ensureMdadmRunDir(); err == nil {
			t.Fatalf("ensureMdadmRunDir() expected error when %q is a file", mdadmRunDir)
		}
	})
}
