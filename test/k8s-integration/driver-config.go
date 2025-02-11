package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

type driverConfig struct {
	StorageClassFile          string
	StorageClass              string
	SnapshotClassFile         string
	SnapshotClass             string
	VolumeAttributesClassFile string
	VolumeAttributesClass     string
	Capabilities              []string
	SupportedFsType           []string
	MinimumVolumeSize         string
	NumAllowedTopologies      int
	Timeouts                  map[string]string
}

const (
	testConfigDir      = "test/k8s-integration/config"
	configTemplateFile = "test-config-template.in"
	configFile         = "test-config.yaml"
	// configurable timeouts for the k8s e2e testsuites
	dataSourceProvisionTimeout = "480s"

	// These are keys for the configurable timeout map.
	dataSourceProvisionTimeoutKey = "DataSourceProvision"
)

// generateDriverConfigFile loads a testdriver config template and creates a file
// with the test-specific configuration
func generateDriverConfigFile(testParams *testParameters) (string, error) {
	// Load template
	t, err := template.ParseFiles(filepath.Join(testParams.pkgDir, testConfigDir, configTemplateFile))
	if err != nil {
		return "", err
	}

	// Create destination
	configFilePath := filepath.Join(testParams.pkgDir, testConfigDir, configFile)
	f, err := os.Create(configFilePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	defer w.Flush()

	// Fill in template parameters. Capabilities can be found here:
	// https://github.com/kubernetes/kubernetes/blob/b717be8269a4f381ab6c23e711e8924bc1f64c93/test/e2e/storage/testsuites/testdriver.go#L136

	caps := []string{
		"persistence",
		"block",
		"fsGroup",
		"exec",
		"multipods",
		"topology",
		"controllerExpansion",
		"nodeExpansion",
	}
	var fsTypes []string
	if testParams.platform == "windows" {
		fsTypes = []string{"ntfs"}
		caps = []string{
			"persistence",
			"exec",
			"multipods",
			"topology",
		}
	} else {
		fsTypes = []string{
			"ext2",
			"ext3",
			"ext4",
		}
	}

	/* Unsupported Capabilities:
	   RWX
	   volumeLimits # PD Supports volume limits but test is very slow
	   singleNodeVolume
	   dataSource
	*/

	switch testParams.deploymentStrategy {
	case "gke":
		if testParams.imageType == "cos" {
			var gkeVer *version
			// The node version is what matters for XFS support. If the node version is not given, we assume
			// it's the same as the cluster master version.
			if testParams.nodeVersion != "" {
				gkeVer = mustParseVersion(testParams.nodeVersion)
			} else {
				gkeVer = mustParseVersion(testParams.clusterVersion)
			}
			if gkeVer.lessThan(mustParseVersion("1.18.0")) {
				// XFS is not supported on COS before 1.18.0
			} else {
				fsTypes = append(fsTypes, "xfs")
			}
		} else {
			// XFS is supported on all non-COS images.
			fsTypes = append(fsTypes, "xfs")
		}
	case "gce":
		fsTypes = append(fsTypes, "xfs")
	default:
		return "", fmt.Errorf("got unknown deployment strat %s, expected gce or gke", testParams.deploymentStrategy)
	}

	var absSnapshotClassFilePath string
	var snapshotClassName string
	// If snapshot class is passed in as argument, include snapshot specific driver capabiltiites.
	if testParams.snapshotClassFile != "" {
		caps = append(caps, "snapshotDataSource")
		// Update the absolute file path pointing to the snapshot class file, if it is provided as an argument.
		absSnapshotClassFilePath = filepath.Join(testParams.pkgDir, testConfigDir, testParams.snapshotClassFile)
		snapshotClassName = testParams.snapshotClassFile[:strings.LastIndex(testParams.snapshotClassFile, ".")]
	} else {
		snapshotClassName = "no-vsc"
	}

	var absVacFilePath string
	var vacName string
	if testParams.volumeAttributesClassFile != "" {
		absVacFilePath = filepath.Join(testParams.pkgDir, testConfigDir, testParams.volumeAttributesClassFile)
		vacName = testParams.volumeAttributesClassFile[:strings.LastIndex(testParams.volumeAttributesClassFile, ".")]
	} else {
		vacName = "no-vac"
	}

	if !strings.Contains(testParams.storageClassFile, "sc-extreme") {
		caps = append(caps, "pvcDataSource")
	}
	minimumVolumeSize := "10Gi"
	numAllowedTopologies := 1
	if testParams.storageClassFile == regionalPDStorageClass {
		minimumVolumeSize = "200Gi"
		numAllowedTopologies = 2
	} else if len(testParams.volumeAttributesClassFile) > 0 {
		minimumVolumeSize = "100Gi"
	}
	timeouts := map[string]string{
		dataSourceProvisionTimeoutKey: dataSourceProvisionTimeout,
	}
	extLoc := strings.LastIndex(testParams.storageClassFile, ".")
	scName := testParams.storageClassFile[:extLoc]
	params := driverConfig{
		StorageClassFile:          filepath.Join(testParams.pkgDir, testConfigDir, testParams.storageClassFile),
		StorageClass:              scName,
		SnapshotClassFile:         absSnapshotClassFilePath,
		SnapshotClass:             snapshotClassName,
		VolumeAttributesClassFile: absVacFilePath,
		VolumeAttributesClass:     vacName,
		SupportedFsType:           fsTypes,
		Capabilities:              caps,
		MinimumVolumeSize:         minimumVolumeSize,
		NumAllowedTopologies:      numAllowedTopologies,
		Timeouts:                  timeouts,
	}

	// Write config file
	err = t.Execute(w, params)
	if err != nil {
		return "", err
	}

	return configFilePath, nil
}
