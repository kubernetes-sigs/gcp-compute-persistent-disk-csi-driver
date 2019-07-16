package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"

	"github.com/golang/glog"
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
		glog.Fatalf("Error creating temp dir: %v", err)
	}
	return dir
}

func removeDir(dir string) {
	err := os.RemoveAll(dir)
	if err != nil {
		glog.Fatalf("Error removing temp dir: %v", err)
	}
}

func ensureVariable(v *string, set bool, msgOnError string) {
	if set && len(*v) == 0 {
		glog.Fatal(msgOnError)
	} else if !set && len(*v) != 0 {
		glog.Fatal(msgOnError)
	}
}

func ensureFlag(v *bool, setTo bool, msgOnError string) {
	if *v != setTo {
		glog.Fatal(msgOnError)
	}
}

func shredFile(filePath string) {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		glog.V(4).Infof("File %v was not found, skipping shredding", filePath)
		return
	}
	glog.V(4).Infof("Shredding file %v", filePath)
	out, err := exec.Command("shred", "--remove", filePath).CombinedOutput()
	if err != nil {
		glog.V(4).Infof("Failed to shred file %v: %v\nOutput:%v", filePath, err, out)
	}
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		glog.V(4).Infof("File %v successfully shredded", filePath)
		return
	}

	// Shred failed Try to remove the file for good meausure
	err = os.Remove(filePath)
	if err != nil {
		glog.V(4).Infof("Failed to remove service account file %s: %v", filePath, err)
	}
}
