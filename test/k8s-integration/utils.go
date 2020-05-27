package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"

	"k8s.io/klog"
)

func runCommand(action string, cmd *exec.Cmd) error {
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr

	fmt.Printf("%s\n", action)
	fmt.Printf("%s\n", cmd.Args)

	err := cmd.Start()
	if err != nil {
		return err
	}

	err = cmd.Wait()
	if err != nil {
		return err
	}
	return nil
}

func generateUniqueTmpDir() string {
	dir, err := ioutil.TempDir("", "gcp-pd-driver-tmp")
	if err != nil {
		klog.Fatalf("Error creating temp dir: %v", err)
	}
	return dir
}

func removeDir(dir string) {
	err := os.RemoveAll(dir)
	if err != nil {
		klog.Fatalf("Error removing temp dir: %v", err)
	}
}

func ensureVariable(v *string, set bool, msgOnError string) {
	if set && len(*v) == 0 {
		klog.Fatal(msgOnError)
	} else if !set && len(*v) != 0 {
		klog.Fatal(msgOnError)
	}
}

func ensureFlag(v *bool, setTo bool, msgOnError string) {
	if *v != setTo {
		klog.Fatal(msgOnError)
	}
}

func shredFile(filePath string) {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		klog.V(4).Infof("File %v was not found, skipping shredding", filePath)
		return
	}
	klog.V(4).Infof("Shredding file %v", filePath)
	out, err := exec.Command("shred", "--remove", filePath).CombinedOutput()
	if err != nil {
		klog.V(4).Infof("Failed to shred file %v: %v\nOutput:%v", filePath, err, out)
	}
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		klog.V(4).Infof("File %v successfully shredded", filePath)
		return
	}

	// Shred failed Try to remove the file for good meausure
	err = os.Remove(filePath)
	if err != nil {
		klog.V(4).Infof("Failed to remove service account file %s: %v", filePath, err)
	}
}

func ensureVariableVal(v *string, val string, msgOnError string) {
	if *v != val {
		klog.Fatal(msgOnError)
	}
}
