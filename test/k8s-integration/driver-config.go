package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
)

type driverConfig struct {
	StorageClassFile string
	Capabilities     []string
}

const (
	testConfigDir      = "test/k8s-integration/config"
	configTemplateFile = "test-config-template.in"
	configFile         = "test-config.yaml"
)

// generateDriverConfigFile loads a testdriver config template and creates a file
// with the test-specific configuration
func generateDriverConfigFile(pkgDir, storageClassFile, deploymentStrat string) (string, error) {
	// Load template
	t, err := template.ParseFiles(filepath.Join(pkgDir, testConfigDir, configTemplateFile))
	if err != nil {
		return "", err
	}

	// Create destination
	configFilePath := filepath.Join(pkgDir, testConfigDir, configFile)
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
	}

	/* Unsupported Capabilities:
	   snapshotDataSource
	   pvcDataSource
	   RWX
	   volumeLimits # PD Supports volume limits but test is very slow
	   singleNodeVolume
	   dataSource
	*/

	// TODO: Support adding/removing capabilities based on Kubernetes version.
	switch deploymentStrat {
	case "gke":
	case "gce":
		// TODO: OSS K8S supports volume expansion for CSI by default in 1.16+;
		// however, at time of writing GKE does not support K8S 1.16+. Add these
		// capabilities for both deployment strategies when GKE Supports CSI
		// Expansion by default.
		caps = append(caps, "controllerExpansion", "nodeExpansion")
	default:
		return "", fmt.Errorf("got unknown deployment strat %s, expected gce or gke", deploymentStrat)
	}

	params := driverConfig{
		StorageClassFile: filepath.Join(pkgDir, testConfigDir, storageClassFile),
		Capabilities:     caps,
	}

	// Write config file
	err = t.Execute(w, params)
	if err != nil {
		return "", err
	}

	return configFilePath, nil
}
