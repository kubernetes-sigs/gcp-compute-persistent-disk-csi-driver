package gceGCEDriver

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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
		{
			name:     "invalid name WARNING token",
			vgName:   "WARNING:",
			expected: false,
		},
		{
			name:     "invalid name unknown token",
			vgName:   "[unknown]",
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

const (
	// `mdadm --detail --scan` output of mdadm 4.2 (Debian bookworm) for the
	// Data Cache array.
	mdadm42DetailScan = "ARRAY /dev/md126 metadata=1.2 name=csi-driver-data-cache UUID=eaf34ad0:278efa4c:5b2e2376:62011509\n"
	// `mdadm --detail --scan` output of mdadm 4.4 (Debian trixie, driver image)
	// for the same array on a COS node without a /dev/md/csi-driver-data-cache
	// symlink, captured on a COS VM.
	mdadm44DetailScan = "ARRAY /dev/md127 metadata=1.2 UUID=eaf34ad0:278efa4c:5b2e2376:62011509\n"
	// `mdadm --detail --export /dev/md127` output of mdadm 4.4 for the Data
	// Cache array, captured on a COS VM.
	mdadm44DetailExport = `MD_LEVEL=raid0
MD_DEVICES=2
MD_METADATA=1.2
MD_UUID=eaf34ad0:278efa4c:5b2e2376:62011509
MD_DEVNAME=csi-driver-data-cache_0
MD_NAME=csi-driver-data-cache
MD_DEVICE_dev_nvme0n2_ROLE=1
MD_DEVICE_dev_nvme0n2_DEV=/dev/nvme0n2
MD_DEVICE_dev_nvme0n1_ROLE=0
MD_DEVICE_dev_nvme0n1_DEV=/dev/nvme0n1
`
)

// fakeMdadm returns a replacement for runMdadm that serves canned output,
// keyed by the space separated mdadm arguments.
func fakeMdadm(t *testing.T, outputs map[string]string, errs map[string]error) func(args ...string) ([]byte, error) {
	t.Helper()
	return func(args ...string) ([]byte, error) {
		key := strings.Join(args, " ")
		if err, ok := errs[key]; ok {
			return nil, err
		}
		output, ok := outputs[key]
		if !ok {
			t.Errorf("unexpected call: mdadm %s", key)
			return nil, fmt.Errorf("unexpected call: mdadm %s", key)
		}
		return []byte(output), nil
	}
}

func TestFindRAIDedLocalSsdPath(t *testing.T) {
	testCases := []struct {
		name      string
		outputs   map[string]string
		errs      map[string]error
		wantPath  string
		wantFound bool
		wantErr   bool
	}{
		{
			name:      "mdadm 4.2 prints the array name in the scan output",
			outputs:   map[string]string{"--detail --scan": mdadm42DetailScan},
			wantPath:  "/dev/md126",
			wantFound: true,
		},
		{
			name: "mdadm 4.2 array name with homehost prefix",
			outputs: map[string]string{
				"--detail --scan": "ARRAY /dev/md126 metadata=1.2 name=node-1:csi-driver-data-cache UUID=5d1a2f7c:0b6e3c1a:9f4d2e8b:1c7a6b3d\n",
			},
			wantPath:  "/dev/md126",
			wantFound: true,
		},
		{
			name: "udev symlink path contains the array name",
			outputs: map[string]string{
				"--detail --scan": "ARRAY /dev/md/csi-driver-data-cache metadata=1.2 UUID=5d1a2f7c:0b6e3c1a:9f4d2e8b:1c7a6b3d\n",
			},
			wantPath:  "/dev/md/csi-driver-data-cache",
			wantFound: true,
		},
		{
			// Seen on a COS VM with host mdadm udev rules: the symlink and
			// MD_DEVNAME carry a "_0" suffix, only MD_NAME matches.
			name: "udev symlink with suffix found via MD_NAME",
			outputs: map[string]string{
				"--detail --scan": "ARRAY /dev/md/csi-driver-data-cache_0 metadata=1.2 UUID=eaf34ad0:278efa4c:5b2e2376:62011509\n",
				"--detail --export /dev/md/csi-driver-data-cache_0": mdadm44DetailExport,
			},
			wantPath:  "/dev/md/csi-driver-data-cache_0",
			wantFound: true,
		},
		{
			name: "mdadm 4.4 array found via MD_NAME",
			outputs: map[string]string{
				"--detail --scan":              mdadm44DetailScan,
				"--detail --export /dev/md127": mdadm44DetailExport,
			},
			wantPath:  "/dev/md127",
			wantFound: true,
		},
		{
			name: "mdadm 4.4 MD_NAME with homehost prefix",
			outputs: map[string]string{
				"--detail --scan":              mdadm44DetailScan,
				"--detail --export /dev/md127": "MD_LEVEL=raid0\nMD_DEVICES=2\nMD_NAME=node-1:csi-driver-data-cache\n",
			},
			wantPath:  "/dev/md127",
			wantFound: true,
		},
		{
			name: "mdadm 4.4 array found via MD_DEVNAME",
			outputs: map[string]string{
				"--detail --scan":              mdadm44DetailScan,
				"--detail --export /dev/md127": "MD_LEVEL=raid0\nMD_DEVICES=2\nMD_DEVNAME=csi-driver-data-cache\n",
			},
			wantPath:  "/dev/md127",
			wantFound: true,
		},
		{
			name: "mdadm 4.4 Data Cache array next to another array",
			outputs: map[string]string{
				"--detail --scan":              "ARRAY /dev/md0 metadata=1.2 UUID=11111111:22222222:33333333:44444444\n" + mdadm44DetailScan,
				"--detail --export /dev/md0":   "MD_LEVEL=raid0\nMD_DEVICES=2\nMD_NAME=gke-node:0\n",
				"--detail --export /dev/md127": mdadm44DetailExport,
			},
			wantPath:  "/dev/md127",
			wantFound: true,
		},
		{
			name:      "no arrays",
			outputs:   map[string]string{"--detail --scan": ""},
			wantFound: false,
		},
		{
			name: "array with a different name",
			outputs: map[string]string{
				"--detail --scan":              mdadm44DetailScan,
				"--detail --export /dev/md127": "MD_LEVEL=raid0\nMD_DEVICES=2\nMD_NAME=csi-driver-data-cache-old\n",
			},
			wantFound: false,
		},
		{
			name:    "scan fails",
			errs:    map[string]error{"--detail --scan": errors.New("exit status 1")},
			wantErr: true,
		},
		{
			name: "details of one array fail",
			outputs: map[string]string{
				"--detail --scan":              "ARRAY /dev/md0 metadata=1.2 UUID=11111111:22222222:33333333:44444444\n" + mdadm44DetailScan,
				"--detail --export /dev/md127": mdadm44DetailExport,
			},
			errs:      map[string]error{"--detail --export /dev/md0": errors.New("exit status 1")},
			wantPath:  "/dev/md127",
			wantFound: true,
		},
		{
			name: "warnings mixed into the output are ignored",
			outputs: map[string]string{
				"--detail --scan":              "mdadm: Unknown keyword devices\n" + mdadm44DetailScan,
				"--detail --export /dev/md127": "mdadm: Unknown keyword devices\n" + mdadm44DetailExport,
			},
			wantPath:  "/dev/md127",
			wantFound: true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			origRunMdadm := runMdadm
			t.Cleanup(func() { runMdadm = origRunMdadm })
			runMdadm = fakeMdadm(t, tc.outputs, tc.errs)

			path, found, err := findRAIDedLocalSsdPath()
			if gotErr := err != nil; gotErr != tc.wantErr {
				t.Fatalf("findRAIDedLocalSsdPath() error = %v, wantErr %v", err, tc.wantErr)
			}
			if path != tc.wantPath || found != tc.wantFound {
				t.Errorf("findRAIDedLocalSsdPath() = (%q, %v), want (%q, %v)", path, found, tc.wantPath, tc.wantFound)
			}
		})
	}
}

func TestIsRaidedAndFetchRAIDedLocalSsdPath(t *testing.T) {
	origRunMdadm := runMdadm
	t.Cleanup(func() { runMdadm = origRunMdadm })

	t.Run("mdadm 4.4 array is detected", func(t *testing.T) {
		runMdadm = fakeMdadm(t, map[string]string{
			"--detail --scan":              mdadm44DetailScan,
			"--detail --export /dev/md127": mdadm44DetailExport,
		}, nil)
		raided, err := IsRaided()
		if err != nil || !raided {
			t.Errorf("IsRaided() = (%v, %v), want (true, nil)", raided, err)
		}
		path, err := fetchRAIDedLocalSsdPath()
		if err != nil || path != "/dev/md127" {
			t.Errorf("fetchRAIDedLocalSsdPath() = (%q, %v), want (%q, nil)", path, err, "/dev/md127")
		}
	})

	t.Run("no Data Cache array", func(t *testing.T) {
		runMdadm = fakeMdadm(t, map[string]string{"--detail --scan": ""}, nil)
		raided, err := IsRaided()
		if err != nil || raided {
			t.Errorf("IsRaided() = (%v, %v), want (false, nil)", raided, err)
		}
		if path, err := fetchRAIDedLocalSsdPath(); err == nil {
			t.Errorf("fetchRAIDedLocalSsdPath() = (%q, nil), want an error", path)
		}
	})

	t.Run("scan error is returned", func(t *testing.T) {
		runMdadm = fakeMdadm(t, nil, map[string]error{"--detail --scan": errors.New("exit status 1")})
		if _, err := IsRaided(); err == nil {
			t.Error("IsRaided() returned no error, want an error")
		}
		if _, err := fetchRAIDedLocalSsdPath(); err == nil {
			t.Error("fetchRAIDedLocalSsdPath() returned no error, want an error")
		}
	})
}

func TestParseMdadmDetailScan(t *testing.T) {
	output := "mdadm: Unknown keyword devices\n" +
		"ARRAY /dev/md0 metadata=1.2 UUID=11111111:22222222:33333333:44444444\n" +
		"ARRAY /dev/md126 metadata=1.2 name=csi-driver-data-cache UUID=5d1a2f7c:0b6e3c1a:9f4d2e8b:1c7a6b3d\n" +
		"ARRAY\n" +
		"\n"
	want := []mdArray{
		{devicePath: "/dev/md0"},
		{devicePath: "/dev/md126", name: "csi-driver-data-cache"},
	}
	if got := parseMdadmDetailScan(output); !reflect.DeepEqual(got, want) {
		t.Errorf("parseMdadmDetailScan() = %+v, want %+v", got, want)
	}
}

func TestParseMdadmExport(t *testing.T) {
	got := parseMdadmExport("mdadm: Unknown keyword devices\n" + mdadm44DetailExport + "NOT_MD=1\nnot a key value line\n")
	want := map[string]string{
		"MD_LEVEL":                   "raid0",
		"MD_DEVICES":                 "2",
		"MD_METADATA":                "1.2",
		"MD_UUID":                    "eaf34ad0:278efa4c:5b2e2376:62011509",
		"MD_DEVNAME":                 "csi-driver-data-cache_0",
		"MD_NAME":                    "csi-driver-data-cache",
		"MD_DEVICE_dev_nvme0n1_ROLE": "0",
		"MD_DEVICE_dev_nvme0n1_DEV":  "/dev/nvme0n1",
		"MD_DEVICE_dev_nvme0n2_ROLE": "1",
		"MD_DEVICE_dev_nvme0n2_DEV":  "/dev/nvme0n2",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseMdadmExport() = %v, want %v", got, want)
	}
}

func TestIsDataCacheArrayName(t *testing.T) {
	testCases := []struct {
		name string
		want bool
	}{
		{name: "csi-driver-data-cache", want: true},
		{name: "node-1:csi-driver-data-cache", want: true},
		{name: "", want: false},
		{name: "md127", want: false},
		{name: "csi-driver-data-cache-old", want: false},
		{name: "old-csi-driver-data-cache", want: false},
		{name: "csi-driver-data-cache:node-1", want: false},
	}
	for _, tc := range testCases {
		if got := isDataCacheArrayName(tc.name); got != tc.want {
			t.Errorf("isDataCacheArrayName(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestParsePvsVgInfo(t *testing.T) {
	testCases := []struct {
		name       string
		output     string
		pvPath     string
		wantVgName string
		wantVgUuid string
	}{
		{
			name:       "clean pvs output with VG",
			output:     "  csi-vg-x7hl7lfz e1L2m3-N4o5-P6q7-R8s9-T0u1-V2w3-X4y5z6 /dev/sdb\n",
			pvPath:     "/dev/sdb",
			wantVgName: "csi-vg-x7hl7lfz",
			wantVgUuid: "e1L2m3-N4o5-P6q7-R8s9-T0u1-V2w3-X4y5z6",
		},
		{
			name: "WARNING PV line with device path as 3rd token before VG row",
			output: "  WARNING: PV /dev/sdb in VG csi-vg-x7hl7lfz is using an old PV header, modify the VG to update.\n" +
				"  WARNING: VG csi-vg-x7hl7lfz is missing PV a1b2c3 (last written to [unknown]).\n" +
				"  csi-vg-x7hl7lfz e1L2m3-N4o5-P6q7-R8s9-T0u1-V2w3-X4y5z6 /dev/sdb\n" +
				"  csi-vg-x7hl7lfz e1L2m3-N4o5-P6q7-R8s9-T0u1-V2w3-X4y5z6 [unknown]\n",
			pvPath:     "/dev/sdb",
			wantVgName: "csi-vg-x7hl7lfz",
			wantVgUuid: "e1L2m3-N4o5-P6q7-R8s9-T0u1-V2w3-X4y5z6",
		},
		{
			name: "WARNING PV marked in use on orphan PV without VG",
			output: "  WARNING: PV /dev/sdb is marked in use but no VG was found using it.\n" +
				"  WARNING: PV /dev/sdb might need repairing.\n" +
				"            /dev/sdb\n",
			pvPath:     "/dev/sdb",
			wantVgName: "",
			wantVgUuid: "",
		},
		{
			name: "WARNING PV marked in use after orphan PV row",
			output: "            /dev/sdb\n" +
				"  WARNING: PV /dev/sdb is marked in use but no VG was found using it.\n",
			pvPath:     "/dev/sdb",
			wantVgName: "",
			wantVgUuid: "",
		},
		{
			name:       "orphan PV with placeholder dashes",
			output:     "  - - /dev/sdb\n",
			pvPath:     "/dev/sdb",
			wantVgName: "",
			wantVgUuid: "",
		},
		{
			name: "invalid VG name tokens like [unknown] or WARNING: in 3-field line are rejected",
			output: "  WARNING: PV /dev/sdb\n" +
				"  [unknown] e1L2m3-N4o5-P6q7-R8s9-T0u1-V2w3-X4y5z6 /dev/sdb\n",
			pvPath:     "/dev/sdb",
			wantVgName: "",
			wantVgUuid: "",
		},
		{
			name:       "empty output",
			output:     "",
			pvPath:     "/dev/sdb",
			wantVgName: "",
			wantVgUuid: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotVgName, gotVgUuid := parsePvsVgInfo(tc.output, tc.pvPath)
			if gotVgName != tc.wantVgName || gotVgUuid != tc.wantVgUuid {
				t.Errorf("parsePvsVgInfo() = (%q, %q), want (%q, %q)", gotVgName, gotVgUuid, tc.wantVgName, tc.wantVgUuid)
			}
		})
	}
}
