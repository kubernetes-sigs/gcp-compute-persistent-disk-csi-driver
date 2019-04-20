package main

import (
	"bufio"
	"os"
	"path/filepath"
	"text/template"
)

type driverConfig struct {
	StorageClassFile string
}

const (
	testConfigDir      = "test/k8s-integration/config"
	configTemplateFile = "test-config-template.in"
	configFile         = "test-config.yaml"
)

// generateDriverConfigFile loads a testdriver config template and creates a file
// with the test-specific configuration
func generateDriverConfigFile(pkgDir, storageClassFile string) (string, error) {
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

	// Fill in template parameters
	params := driverConfig{
		StorageClassFile: filepath.Join(pkgDir, testConfigDir, storageClassFile),
	}

	// Write config file
	err = t.Execute(w, params)
	if err != nil {
		return "", err
	}

	return configFilePath, nil
}
