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

func TestParseVgsUuid(t *testing.T) {
	testCases := []struct {
		name     string
		output   string
		wantUuid string
	}{
		{
			name:     "clean vgs output",
			output:   "  e1L2m3-N4o5-P6q7-R8s9-T0u1-V2w3-X4y5z6\n",
			wantUuid: "e1L2m3-N4o5-P6q7-R8s9-T0u1-V2w3-X4y5z6",
		},
		{
			name: "WARNING lines before and after vg_uuid",
			output: "  WARNING: VG csi-vg-x7hl7lfz is missing PV a1b2c3 (last written to [unknown]).\n" +
				"  WARNING: PV /dev/sdb is marked in use but no VG was found using it.\n" +
				"  e1L2m3-N4o5-P6q7-R8s9-T0u1-V2w3-X4y5z6\n" +
				"  WARNING: PV /dev/sdb might need repairing.\n",
			wantUuid: "e1L2m3-N4o5-P6q7-R8s9-T0u1-V2w3-X4y5z6",
		},
		{
			name:     "placeholder dash is ignored",
			output:   "  -\n",
			wantUuid: "",
		},
		{
			name:     "invalid token is ignored",
			output:   "  [unknown]\n",
			wantUuid: "",
		},
		{
			name:     "empty output",
			output:   "",
			wantUuid: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseVgsUuid(tc.output); got != tc.wantUuid {
				t.Errorf("parseVgsUuid() = %q, want %q", got, tc.wantUuid)
			}
		})
	}
}

func TestGetVgUuid(t *testing.T) {
	origRunCommand := runCommand
	t.Cleanup(func() { runCommand = origRunCommand })

	t.Run("strips WARNING lines from vgs output", func(t *testing.T) {
		runCommand = func(pipeCmd string, pipeCmdArg []string, cmd1 string, execCmdArgs ...string) ([]byte, error) {
			if cmd1 != "vgs" {
				t.Fatalf("unexpected command %q", cmd1)
			}
			out := "  WARNING: VG csi-vg-x7hl7lfz is missing PV a1b2c3 (last written to [unknown]).\n" +
				"  e1L2m3-N4o5-P6q7-R8s9-T0u1-V2w3-X4y5z6\n"
			return []byte(out), nil
		}
		got, err := getVgUuid("csi-vg-x7hl7lfz")
		if err != nil {
			t.Fatalf("getVgUuid() unexpected error: %v", err)
		}
		want := "e1L2m3-N4o5-P6q7-R8s9-T0u1-V2w3-X4y5z6"
		if got != want {
			t.Errorf("getVgUuid() = %q, want %q", got, want)
		}
	})

	t.Run("passes device filter when excludedPvPaths is provided", func(t *testing.T) {
		var gotArgs []string
		runCommand = func(pipeCmd string, pipeCmdArg []string, cmd1 string, execCmdArgs ...string) ([]byte, error) {
			gotArgs = execCmdArgs
			return []byte("  e1L2m3-N4o5-P6q7-R8s9-T0u1-V2w3-X4y5z6\n"), nil
		}
		got, err := getVgUuid("csi-vg-x7hl7lfz", "/dev/sdb")
		if err != nil {
			t.Fatalf("getVgUuid() unexpected error: %v", err)
		}
		if got != "e1L2m3-N4o5-P6q7-R8s9-T0u1-V2w3-X4y5z6" {
			t.Errorf("getVgUuid() = %q", got)
		}
		wantArgs := []string{
			"--noheadings",
			"-o",
			"vg_uuid",
			"csi-vg-x7hl7lfz",
			"--config",
			`devices { filter = [ "r|^/dev/sdb$|", "a|.*|" ] }`,
		}
		if !reflect.DeepEqual(gotArgs, wantArgs) {
			t.Errorf("vgs args = %v, want %v", gotArgs, wantArgs)
		}
	})

	t.Run("returns error when vgs fails", func(t *testing.T) {
		runCommand = func(pipeCmd string, pipeCmdArg []string, cmd1 string, execCmdArgs ...string) ([]byte, error) {
			return []byte("  Volume group \"csi-vg-x7hl7lfz\" not found\n"), errors.New("exit status 5")
		}
		if _, err := getVgUuid("csi-vg-x7hl7lfz"); err == nil {
			t.Fatal("getVgUuid() expected error, got nil")
		}
	})
}

func TestDetectPreemptionVgConflict(t *testing.T) {
	origRunCommand := runCommand
	t.Cleanup(func() { runCommand = origRunCommand })

	const (
		vgName   = "csi-vg-x7hl7lfz"
		pvPath   = "/dev/sdb"
		pdUuid   = "111111-2222-3333-4444-5555-6666-777777"
		hostUuid = "aaaaaa-bbbb-cccc-dddd-eeee-ffff-000000"
	)

	t.Run("detects conflict when host VG has different UUID", func(t *testing.T) {
		runCommand = func(pipeCmd string, pipeCmdArg []string, cmd1 string, execCmdArgs ...string) ([]byte, error) {
			switch cmd1 {
			case "pvs":
				return []byte(fmt.Sprintf("  %s %s %s\n", vgName, pdUuid, pvPath)), nil
			case "vgs":
				return []byte(fmt.Sprintf("  %s\n", hostUuid)), nil
			default:
				t.Fatalf("unexpected command %q", cmd1)
				return nil, nil
			}
		}
		conflict, staleUuid, err := detectPreemptionVgConflict(vgName, pvPath)
		if err != nil {
			t.Fatalf("detectPreemptionVgConflict() unexpected error: %v", err)
		}
		if !conflict || staleUuid != pdUuid {
			t.Errorf("detectPreemptionVgConflict() = (%v, %q), want (true, %q)", conflict, staleUuid, pdUuid)
		}
	})

	t.Run("detects conflict when host VG does not exist on host devices", func(t *testing.T) {
		runCommand = func(pipeCmd string, pipeCmdArg []string, cmd1 string, execCmdArgs ...string) ([]byte, error) {
			switch cmd1 {
			case "pvs":
				return []byte(fmt.Sprintf("  %s %s %s\n", vgName, pdUuid, pvPath)), nil
			case "vgs":
				return []byte("  Volume group not found\n"), errors.New("exit status 5")
			default:
				t.Fatalf("unexpected command %q", cmd1)
				return nil, nil
			}
		}
		conflict, staleUuid, err := detectPreemptionVgConflict(vgName, pvPath)
		if err != nil {
			t.Fatalf("detectPreemptionVgConflict() unexpected error: %v", err)
		}
		if !conflict || staleUuid != pdUuid {
			t.Errorf("detectPreemptionVgConflict() = (%v, %q), want (true, %q)", conflict, staleUuid, pdUuid)
		}
	})

	t.Run("no conflict when host VG has same UUID even with WARNING output", func(t *testing.T) {
		runCommand = func(pipeCmd string, pipeCmdArg []string, cmd1 string, execCmdArgs ...string) ([]byte, error) {
			switch cmd1 {
			case "pvs":
				return []byte(fmt.Sprintf("  %s %s %s\n", vgName, pdUuid, pvPath)), nil
			case "vgs":
				out := fmt.Sprintf("  WARNING: VG %s is missing PV a1b2c3 (last written to %s).\n  %s\n", vgName, pvPath, pdUuid)
				return []byte(out), nil
			default:
				t.Fatalf("unexpected command %q", cmd1)
				return nil, nil
			}
		}
		conflict, staleUuid, err := detectPreemptionVgConflict(vgName, pvPath)
		if err != nil {
			t.Fatalf("detectPreemptionVgConflict() unexpected error: %v", err)
		}
		if conflict || staleUuid != "" {
			t.Errorf("detectPreemptionVgConflict() = (%v, %q), want (false, \"\")", conflict, staleUuid)
		}
	})
}

func TestGetUnstagedVolumeGroupName(t *testing.T) {
	fullVolID := "projects/test-proj/zones/us-central1-a/disks/pvc-8b643106-31d1-4a48-90b5-d000605514e0"
	shortVolID := "pvc-8b643106-31d1-4a48-90b5-d000605514e0"

	vg1 := getUnstagedVolumeGroupName(fullVolID)
	vg2 := getUnstagedVolumeGroupName(shortVolID)
	if vg1 != vg2 {
		t.Errorf("getUnstagedVolumeGroupName(%q) = %q != getUnstagedVolumeGroupName(%q) = %q", fullVolID, vg1, shortVolID, vg2)
	}
	if !strings.HasPrefix(vg1, "csi-vg-unstaged-") {
		t.Errorf("getUnstagedVolumeGroupName(%q) = %q, want prefix %q", fullVolID, vg1, "csi-vg-unstaged-")
	}
	if !isValidVGName(vg1) {
		t.Errorf("getUnstagedVolumeGroupName(%q) = %q is not a valid VG name", fullVolID, vg1)
	}
	nodeVg := getVolumeGroupName("gke-cluster-pool-12345678-abcd")
	if vg1 == nodeVg {
		t.Errorf("unstaged VG name %q should not collide with node VG name %q", vg1, nodeVg)
	}
}

func TestCleanupCache(t *testing.T) {
	const (
		nodeID   = "gke-cluster-pool-12345678-abcd"
		volumeID = "projects/test-proj/zones/us-central1-a/disks/pvc-8b643106-31d1-4a48-90b5-d000605514e0"
	)
	nodeVg := getVolumeGroupName(nodeID)
	unstagedVg := getUnstagedVolumeGroupName(volumeID)
	mainLv := getLvName(mainLvSuffix, volumeID)

	t.Run("deactivates, uncaches, and splits LV into unstaged VG", func(t *testing.T) {
		origRunCommand := runCommand
		t.Cleanup(func() { runCommand = origRunCommand })

		var calls []string
		runCommand = func(pipeCmd string, pipeCmdArg []string, cmd1 string, execCmdArgs ...string) ([]byte, error) {
			call := strings.TrimSpace(cmd1 + " " + strings.Join(execCmdArgs, " "))
			calls = append(calls, call)
			switch cmd1 {
			case "vgscan":
				return []byte(fmt.Sprintf("  Found volume group %q using metadata type lvm2\n", nodeVg)), nil
			case "vgreduce":
				return []byte(fmt.Sprintf("  Volume group %q is already consistent.\n", nodeVg)), nil
			case "lvscan":
				return []byte(fmt.Sprintf("  ACTIVE            '/dev/%s/%s' [4.00 GiB] inherit\n", nodeVg, mainLv)), nil
			case "lvchange":
				return nil, nil
			case "lvs":
				return []byte("  [csi-fast-pvc-8b643106-31d1-4a48-90b5-d000605514e0_cvol]\n"), nil
			case "lvconvert":
				return []byte(fmt.Sprintf("  Logical volume %s/%s is not cached and csi-fast-... is removed.\n", nodeVg, mainLv)), nil
			case "vgsplit":
				return []byte(fmt.Sprintf("  New volume group %q successfully split from %q\n", unstagedVg, nodeVg)), nil
			default:
				t.Fatalf("unexpected command: %s", call)
				return nil, nil
			}
		}

		if err := cleanupCache(volumeID, nodeID); err != nil {
			t.Fatalf("cleanupCache() unexpected error: %v", err)
		}

		wantCalls := []string{
			"vgscan --config backup { backup = 0 archive = 0 }",
			"vgscan --config backup { backup = 0 archive = 0 }",
			fmt.Sprintf("vgreduce --removemissing --autobackup n %s --force", nodeVg),
			"lvscan",
			fmt.Sprintf("lvchange -an --autobackup n /dev/%s/%s", nodeVg, mainLv),
			fmt.Sprintf("lvs --select lv_name=%s -o pool_lv", mainLv),
			fmt.Sprintf("lvconvert --uncache %s/%s -y", nodeVg, mainLv),
			fmt.Sprintf("vgsplit --autobackup n -n %s %s %s", mainLv, nodeVg, unstagedVg),
		}
		if !reflect.DeepEqual(calls, wantCalls) {
			t.Errorf("cleanupCache() calls =\n  %v\nwant\n  %v", calls, wantCalls)
		}
	})

	t.Run("skips lvconvert uncache if LV is already uncached and splits into unstaged VG", func(t *testing.T) {
		origRunCommand := runCommand
		t.Cleanup(func() { runCommand = origRunCommand })

		var calls []string
		runCommand = func(pipeCmd string, pipeCmdArg []string, cmd1 string, execCmdArgs ...string) ([]byte, error) {
			call := strings.TrimSpace(cmd1 + " " + strings.Join(execCmdArgs, " "))
			calls = append(calls, call)
			switch cmd1 {
			case "vgscan":
				return []byte(fmt.Sprintf("  Found volume group %q using metadata type lvm2\n", nodeVg)), nil
			case "vgreduce":
				return nil, nil
			case "lvscan":
				return []byte(fmt.Sprintf("  inactive          '/dev/%s/%s' [4.00 GiB] inherit\n", nodeVg, mainLv)), nil
			case "lvchange":
				return nil, nil
			case "lvs":
				return []byte("  Pool\n      \n"), nil
			case "vgsplit":
				return nil, nil
			default:
				t.Fatalf("unexpected command: %s", call)
				return nil, nil
			}
		}

		if err := cleanupCache(volumeID, nodeID); err != nil {
			t.Fatalf("cleanupCache() unexpected error: %v", err)
		}

		wantCalls := []string{
			"vgscan --config backup { backup = 0 archive = 0 }",
			"vgscan --config backup { backup = 0 archive = 0 }",
			fmt.Sprintf("vgreduce --removemissing --autobackup n %s --force", nodeVg),
			"lvscan",
			fmt.Sprintf("lvchange -an --autobackup n /dev/%s/%s", nodeVg, mainLv),
			fmt.Sprintf("lvs --select lv_name=%s -o pool_lv", mainLv),
			fmt.Sprintf("vgsplit --autobackup n -n %s %s %s", mainLv, nodeVg, unstagedVg),
		}
		if !reflect.DeepEqual(calls, wantCalls) {
			t.Errorf("cleanupCache() calls =\n  %v\nwant\n  %v", calls, wantCalls)
		}
	})

	t.Run("is idempotent when LV was already split into unstaged VG", func(t *testing.T) {
		origRunCommand := runCommand
		t.Cleanup(func() { runCommand = origRunCommand })

		var calls []string
		runCommand = func(pipeCmd string, pipeCmdArg []string, cmd1 string, execCmdArgs ...string) ([]byte, error) {
			call := strings.TrimSpace(cmd1 + " " + strings.Join(execCmdArgs, " "))
			calls = append(calls, call)
			switch cmd1 {
			case "vgscan":
				return []byte(fmt.Sprintf("  Found volume group %q using metadata type lvm2\n  Found volume group %q using metadata type lvm2\n", nodeVg, unstagedVg)), nil
			case "vgreduce":
				return nil, nil
			case "lvscan":
				// LV is in unstagedVg, not nodeVg.
				return []byte(fmt.Sprintf("  inactive          '/dev/%s/%s' [4.00 GiB] inherit\n", unstagedVg, mainLv)), nil
			default:
				t.Fatalf("unexpected command: %s", call)
				return nil, nil
			}
		}

		if err := cleanupCache(volumeID, nodeID); err != nil {
			t.Fatalf("cleanupCache() unexpected error: %v", err)
		}

		wantCalls := []string{
			"vgscan --config backup { backup = 0 archive = 0 }",
			"vgscan --config backup { backup = 0 archive = 0 }",
			fmt.Sprintf("vgreduce --removemissing --autobackup n %s --force", nodeVg),
			"lvscan",
		}
		if !reflect.DeepEqual(calls, wantCalls) {
			t.Errorf("cleanupCache() calls =\n  %v\nwant\n  %v", calls, wantCalls)
		}
	})

	t.Run("returns error when vgsplit fails", func(t *testing.T) {
		origRunCommand := runCommand
		t.Cleanup(func() { runCommand = origRunCommand })

		runCommand = func(pipeCmd string, pipeCmdArg []string, cmd1 string, execCmdArgs ...string) ([]byte, error) {
			switch cmd1 {
			case "vgscan":
				return []byte(fmt.Sprintf("  Found volume group %q using metadata type lvm2\n", nodeVg)), nil
			case "vgreduce":
				return nil, nil
			case "lvscan":
				return []byte(fmt.Sprintf("  ACTIVE            '/dev/%s/%s' [4.00 GiB] inherit\n", nodeVg, mainLv)), nil
			case "lvchange":
				return nil, nil
			case "lvs":
				return []byte("  [csi-fast-pvc_cvol]\n"), nil
			case "lvconvert":
				return nil, nil
			case "vgsplit":
				return []byte("vgsplit error"), errors.New("exit status 5")
			default:
				return nil, nil
			}
		}

		if err := cleanupCache(volumeID, nodeID); err == nil {
			t.Fatal("cleanupCache() expected error when vgsplit fails, got nil")
		}
	})
}

func TestRecoverPreemptionVgConflict(t *testing.T) {
	origRunCommand := runCommand
	t.Cleanup(func() { runCommand = origRunCommand })

	const (
		vgName      = "csi-vg-x7hl7lfz"
		mainLvName  = "csi-main-pvc-1234"
		devicePath  = "/dev/sdb"
		staleVgUuid = "11111122-3333-4444-5555-666677778888"
		tempVgName  = "csi-vg-stale-11111122"
	)
	configFilter := fmt.Sprintf("devices { filter = [ \"a|%s|\", \"r|.*|\" ] }", devicePath)

	var calls []string
	runCommand = func(pipeCmd string, pipeCmdArg []string, cmd1 string, execCmdArgs ...string) ([]byte, error) {
		call := strings.TrimSpace(cmd1 + " " + strings.Join(execCmdArgs, " "))
		calls = append(calls, call)
		switch cmd1 {
		case "vgchange", "lvconvert", "vgreduce", "vgrename":
			return nil, nil
		case "lvs":
			return []byte("  [csi-fast-pvc-1234_cvol]\n"), nil
		default:
			t.Fatalf("unexpected command: %s", call)
			return nil, nil
		}
	}

	if err := recoverPreemptionVgConflict(vgName, mainLvName, devicePath, staleVgUuid); err != nil {
		t.Fatalf("recoverPreemptionVgConflict() unexpected error: %v", err)
	}

	wantCalls := []string{
		fmt.Sprintf("vgchange -an --autobackup n %s --config %s", vgName, configFilter),
		fmt.Sprintf("lvs --noheadings --select lv_name=%s && vg_name=%s -o pool_lv --config %s", mainLvName, vgName, configFilter),
		fmt.Sprintf("lvconvert --uncache %s/%s --force -y --config %s", vgName, mainLvName, configFilter),
		fmt.Sprintf("vgreduce --removemissing --autobackup n %s --force --config %s", vgName, configFilter),
		fmt.Sprintf("vgrename --autobackup n %s %s --config %s", vgName, tempVgName, configFilter),
		fmt.Sprintf("vgchange -u --autobackup n %s --config %s", tempVgName, configFilter),
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Errorf("recoverPreemptionVgConflict() calls =\n  %v\nwant\n  %v", calls, wantCalls)
	}
}

func TestMergeStaleVgIntoHost(t *testing.T) {
	origRunCommand := runCommand
	t.Cleanup(func() { runCommand = origRunCommand })

	const (
		staleVg    = "csi-vg-unstaged-abcd1234"
		hostVg     = "csi-vg-x7hl7lfz"
		mainLvName = "csi-main-pvc-1234"
	)

	var calls []string
	runCommand = func(pipeCmd string, pipeCmdArg []string, cmd1 string, execCmdArgs ...string) ([]byte, error) {
		call := strings.TrimSpace(cmd1 + " " + strings.Join(execCmdArgs, " "))
		calls = append(calls, call)
		switch cmd1 {
		case "vgscan":
			return []byte(fmt.Sprintf("  Found volume group %q using metadata type lvm2\n", staleVg)), nil
		case "vgchange", "lvconvert", "vgreduce", "vgmerge":
			return nil, nil
		case "lvs":
			return []byte("  [csi-fast-pvc-1234_cvol]\n"), nil
		default:
			t.Fatalf("unexpected command: %s", call)
			return nil, nil
		}
	}

	if err := mergeStaleVgIntoHost(staleVg, hostVg, mainLvName); err != nil {
		t.Fatalf("mergeStaleVgIntoHost() unexpected error: %v", err)
	}

	wantCalls := []string{
		fmt.Sprintf("vgchange -ay --autobackup n %s", staleVg),
		fmt.Sprintf("lvs --select lv_name=%s -o pool_lv", mainLvName),
		fmt.Sprintf("lvconvert --uncache %s/%s --force -y --config backup { backup = 0 archive = 0 }", staleVg, mainLvName),
		"vgscan --config backup { backup = 0 archive = 0 }",
		fmt.Sprintf("vgreduce --removemissing --autobackup n %s --force", staleVg),
		fmt.Sprintf("vgchange -an --autobackup n %s", staleVg),
		fmt.Sprintf("vgmerge --autobackup n %s %s", hostVg, staleVg),
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Errorf("mergeStaleVgIntoHost() calls =\n  %v\nwant\n  %v", calls, wantCalls)
	}
}
