/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/test-infra/boskos/client"
	gce "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/test/remote/remote"

	"github.com/golang/glog"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/cloudresourcemanager/v1"
	compute "google.golang.org/api/compute/v0.beta"
)

var testArgs = flag.String("test_args", "", "Space-separated list of arguments to pass to Ginkgo test runner.")
var zone = flag.String("zone", "", "gce zone the hosts live in")
var project = flag.String("project", "", "gce project the hosts live in")
var cleanup = flag.Bool("cleanup", true, "If true remove files from remote hosts and delete temporary instances")
var deleteInstances = flag.Bool("delete-instances", true, "If true, delete any instances created")
var buildOnly = flag.Bool("build-only", false, "If true, build e2e_gce_pd_test.tar.gz and exit.")
var ginkgoFlags = flag.String("ginkgo-flags", "", "Passed to ginkgo to specify additional flags such as --skip=.")
var serviceAccount = flag.String("service-account", "", "GCP Service Account to start the test instance under")
var runInProw = flag.Bool("run-in-prow", false, "If true, use a Boskos loaned project and special CI service accounts and ssh keys")

// envs is the type used to collect all node envs. The key is the env name,
// and the value is the env value
type envs map[string]string

// String function of flag.Value
func (e *envs) String() string {
	return fmt.Sprint(*e)
}

// Set function of flag.Value
func (e *envs) Set(value string) error {
	kv := strings.SplitN(value, "=", 2)
	if len(kv) != 2 {
		return fmt.Errorf("invalid env string")
	}
	emap := *e
	emap[kv[0]] = kv[1]
	return nil
}

// nodeEnvs is the node envs from the flag `node-env`.
var nodeEnvs = make(envs)

func init() {
	flag.Var(&nodeEnvs, "node-env", "An environment variable passed to instance as metadata, e.g. when '--node-env=PATH=/usr/bin' is specified, there will be an extra instance metadata 'PATH=/usr/bin'.")
}

const (
	defaultMachine      = "n1-standard-1"
	defaultFirewallRule = "default-allow-ssh"
)

var (
	computeService *compute.Service
	arc            Archive
	suite          remote.TestSuite

	boskos = client.NewClient(os.Getenv("JOB_NAME"), "http://boskos")
)

// Archive contains information about the test tar
type Archive struct {
	sync.Once
	path string
	err  error
}

// TestResult contains info about results of test
type TestResult struct {
	output string
	err    error
	host   string
	exitOk bool
}

func main() {
	flag.Parse()
	suite = remote.InitE2ERemote()

	if *runInProw {
		// Try to get a Boskos project
		glog.V(4).Infof("Running in PROW")
		glog.V(4).Infof("Fetching a Boskos loaned project")

		p, err := boskos.Acquire("gce-project", "free", "busy")
		if err != nil {
			glog.Fatal("boskos failed to acquire project: %v", err)
		}

		if p == nil {
			glog.Fatal("boskos does not have a free gce-project at the moment")
		}

		glog.Infof("Overwriting supplied project %v with project from Boskos: %v", *project, p.GetName())

		*project = p.GetName()

		go func(c *client.Client, proj string) {
			for range time.Tick(time.Minute * 5) {
				if err := c.UpdateOne(p.Name, "busy", nil); err != nil {
					glog.Warningf("[Boskos] Update %s failed with %v", p, err)
				}
			}
		}(boskos, p.Name)

		// If we're on CI overwrite the service account
		glog.V(4).Infof("Fetching the default compute service account")

		c, err := google.DefaultClient(context.TODO(), cloudresourcemanager.CloudPlatformScope)
		if err != nil {
			glog.Fatalf("Failed to get Google Default Client: %v", err)
		}

		cloudresourcemanagerService, err := cloudresourcemanager.New(c)
		if err != nil {
			glog.Fatalf("Failed to create new cloudresourcemanager: %v", err)
		}

		resp, err := cloudresourcemanagerService.Projects.Get(*project).Do()
		if err != nil {
			glog.Fatal("Failed to get project %v from Cloud Resource Manager: %v", *project, err)
		}

		// Default Compute Engine service account
		// [PROJECT_NUMBER]-compute@developer.gserviceaccount.com
		sa := fmt.Sprintf("%v-compute@developer.gserviceaccount.com", resp.ProjectNumber)
		glog.Infof("Overwriting supplied service account %v with PROW service account %v", *serviceAccount, sa)

		*serviceAccount = sa
	}

	if *project == "" {
		glog.Fatal("Project must be speficied")
	}

	if *zone == "" {
		glog.Fatal("Zone must be specified")
	}

	if *serviceAccount == "" {
		glog.Fatal("You must specify a service account to create an instance under that has at least OWNERS permissions on disks and READER on instances.")
	}

	rand.Seed(time.Now().UTC().UnixNano())
	if *buildOnly {
		// Build the archive and exit
		remote.CreateTestArchive(suite)
		return
	}

	var err error
	computeService, err = getComputeClient()
	if err != nil {
		glog.Fatalf("Unable to create gcloud compute service using defaults.  Make sure you are authenticated. %v", err)
	}

	// Setup coloring
	stat, _ := os.Stdout.Stat()
	useColor := (stat.Mode() & os.ModeCharDevice) != 0
	blue := ""
	noColour := ""
	if useColor {
		blue = "\033[0;34m"
		noColour = "\033[0m"
	}

	go arc.getArchive()
	defer arc.deleteArchive()

	fmt.Printf("Initializing e2e tests")
	results := test([]string{"TODO tests"})
	// Wait for all tests to complete and emit the results
	errCount := 0
	host := results.host
	fmt.Println() // Print an empty line
	fmt.Printf("%s>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>%s\n", blue, noColour)
	fmt.Printf("%s>                              START TEST                                >%s\n", blue, noColour)
	fmt.Printf("%s>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>%s\n", blue, noColour)
	fmt.Printf("Start Test Suite on Host %s\n", host)
	fmt.Printf("%s\n", results.output)
	if results.err != nil {
		errCount++
		fmt.Printf("Failure Finished Test Suite on Host %s\n%v\n", host, results.err)
	} else {
		fmt.Printf("Success Finished Test Suite on Host %s\n", host)
	}
	fmt.Printf("%s<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<%s\n", blue, noColour)
	fmt.Printf("%s<                              FINISH TEST                               <%s\n", blue, noColour)
	fmt.Printf("%s<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<%s\n", blue, noColour)
	fmt.Println() // Print an empty line

	if boskos.HasResource() {
		if berr := boskos.ReleaseAll("dirty"); berr != nil {
			glog.Fatalf("[Boskos] Fail To Release: %v, kubetest err: %v", berr, err)
		}
	}

	// Set the exit code if there were failures
	if !results.exitOk {
		fmt.Printf("Failure: %d errors encountered.\n", errCount)
		arc.deleteArchive()
		os.Exit(1)
	}
}

func (a *Archive) getArchive() (string, error) {
	a.Do(func() { a.path, a.err = remote.CreateTestArchive(suite) })
	return a.path, a.err
}

func (a *Archive) deleteArchive() {
	path, err := a.getArchive()
	if err != nil {
		return
	}
	os.Remove(path)
}

// Run tests in archive against host
func testHost(host string, deleteFiles bool, ginkgoFlagsStr string) *TestResult {
	instance, err := computeService.Instances.Get(*project, *zone, host).Do()
	if err != nil {
		return &TestResult{
			err:    err,
			host:   host,
			exitOk: false,
		}
	}
	if strings.ToUpper(instance.Status) != "RUNNING" {
		err = fmt.Errorf("instance %s not in state RUNNING, was %s", host, instance.Status)
		return &TestResult{
			err:    err,
			host:   host,
			exitOk: false,
		}
	}
	externalIP := getexternalIP(instance)
	if len(externalIP) > 0 {
		remote.AddHostnameIP(host, externalIP)
	}

	path, err := arc.getArchive()
	if err != nil {
		// Don't log fatal because we need to do any needed cleanup contained in "defer" statements
		return &TestResult{
			err: fmt.Errorf("unable to create test archive: %v", err),
		}
	}

	output, exitOk, err := remote.RunRemote(suite, path, host, deleteFiles, *testArgs, ginkgoFlagsStr)
	return &TestResult{
		output: output,
		err:    err,
		host:   host,
		exitOk: exitOk,
	}
}

// Provision a gce instance using image and run the tests in archive against the instance.
// Delete the instance afterward.
func test(tests []string) *TestResult {
	ginkgoFlagsStr := *ginkgoFlags
	// Check whether the test is for benchmark.
	if len(tests) > 0 {
		// Use the Ginkgo focus in benchmark config.
		ginkgoFlagsStr += (" " + testsToGinkgoFocus(tests))
	}

	host, err := createInstance(*serviceAccount)
	if *deleteInstances {
		defer deleteInstance(host)
	}
	if err != nil {
		return &TestResult{
			err: fmt.Errorf("unable to create gce instance with running docker daemon for image.  %v", err),
		}
	}

	// Only delete the files if we are keeping the instance and want it cleaned up.
	// If we are going to delete the instance, don't bother with cleaning up the files
	deleteFiles := !*deleteInstances && *cleanup

	result := testHost(host, deleteFiles, ginkgoFlagsStr)
	// This is a temporary solution to collect serial node serial log. Only port 1 contains useful information.
	// TODO(random-liu): Extract out and unify log collection logic with cluste e2e.
	serialPortOutput, err := computeService.Instances.GetSerialPortOutput(*project, *zone, host).Port(1).Do()
	if err != nil {
		glog.Errorf("Failed to collect serial output from node %q: %v", host, err)
	} else {
		logFilename := "serial-1.log"
		err := remote.WriteLog(host, logFilename, serialPortOutput.Contents)
		if err != nil {
			glog.Errorf("Failed to write serial output from node %q to %q: %v", host, logFilename, err)
		}
	}
	return result
}

// Provision a gce instance using image
func createInstance(serviceAccount string) (string, error) {
	var err error

	name := "gce-pd-csi-e2e"
	myuuid := string(uuid.NewUUID())

	// Create default filewall rule if it does not exist
	if _, err = computeService.Firewalls.Get(*project, defaultFirewallRule).Do(); err != nil {
		glog.Infof("Default firewall rule %v does not exist, creating", defaultFirewallRule)
		f := &compute.Firewall{
			Name: defaultFirewallRule,
			Allowed: []*compute.FirewallAllowed{
				&compute.FirewallAllowed{
					IPProtocol: "tcp",
					Ports:      []string{"22"},
				},
			},
		}
		_, err = computeService.Firewalls.Insert(*project, f).Do()
		if err != nil {
			return "", fmt.Errorf("Failed to insert required default SSH Firewall Rule %v: %v", defaultFirewallRule, err)
		}
	} else {
		glog.Infof("Default firewall rule %v already exists, skipping creation", defaultFirewallRule)
	}

	glog.V(4).Infof("Creating instance: %v", name)

	// TODO: Pick a better boot disk image
	imageURL := "projects/ml-images/global/images/family/tf-1-9"
	i := &compute.Instance{
		Name:        name,
		MachineType: machineType(""),
		NetworkInterfaces: []*compute.NetworkInterface{
			{
				AccessConfigs: []*compute.AccessConfig{
					{
						Type: "ONE_TO_ONE_NAT",
						Name: "External NAT",
					},
				}},
		},
		Disks: []*compute.AttachedDisk{
			{
				AutoDelete: true,
				Boot:       true,
				Type:       "PERSISTENT",
				InitializeParams: &compute.AttachedDiskInitializeParams{
					DiskName:    "my-root-pd-" + myuuid,
					SourceImage: imageURL,
				},
			},
		},
	}

	saObj := &compute.ServiceAccount{
		Email:  serviceAccount,
		Scopes: []string{"https://www.googleapis.com/auth/cloud-platform"},
	}
	i.ServiceAccounts = []*compute.ServiceAccount{saObj}

	if pubkey, ok := os.LookupEnv("JENKINS_GCE_SSH_PUBLIC_KEY_FILE"); ok {
		glog.V(4).Infof("JENKINS_GCE_SSH_PUBLIC_KEY_FILE set to %v, adding public key to Instance", pubkey)
		meta, err := generateMetadataWithPublicKey(pubkey)
		if err != nil {
			return "", err
		}
		i.Metadata = meta
		/*glog.V(4).Infof("JENKINS_GCE_SSH_PUBLIC_KEY_FILE set to %v, adding public key to Instance", pubkey)
		// If we're on CI add public SSH keys to the instance
		i.Metadata =
		err = addPubKeyToInstance(*project, *zone, i.Name, pubkey)
		if err != nil {
			return "", fmt.Errorf("could not add Jenkins public key %v to instance %v: %v", pubkey, i.Name, err)
		}*/
	}

	if _, err := computeService.Instances.Get(*project, *zone, i.Name).Do(); err != nil {
		op, err := computeService.Instances.Insert(*project, *zone, i).Do()
		glog.V(4).Infof("Inserted instance %v in project %v, zone %v", i.Name, *project, *zone)
		if err != nil {
			ret := fmt.Sprintf("could not create instance %s: API error: %v", name, err)
			if op != nil {
				ret = fmt.Sprintf("%s: %v", ret, op.Error)
			}
			return "", fmt.Errorf(ret)
		} else if op.Error != nil {
			return "", fmt.Errorf("could not create instance %s: %+v", name, op.Error)
		}
	} else {
		glog.V(4).Infof("Compute service GOT instance %v, skipping instance creation", i.Name)
	}

	then := time.Now()
	err = wait.Poll(15*time.Second, 5*time.Minute, func() (bool, error) {
		glog.V(2).Infof("Waiting for instance %v to come up. %v elapsed", name, time.Since(then))
		var instance *compute.Instance
		instance, err = computeService.Instances.Get(*project, *zone, name).Do()
		if err != nil {
			glog.Errorf("Failed to get instance %v: %v", name, err)
			return false, nil
		}

		if strings.ToUpper(instance.Status) != "RUNNING" {
			glog.Warningf("instance %s not in state RUNNING, was %s", name, instance.Status)
			return false, nil
		}

		externalIP := getexternalIP(instance)
		if len(externalIP) > 0 {
			remote.AddHostnameIP(name, externalIP)
		}

		if sshOut, err := remote.SSHCheckAlive(name); err != nil {
			err = fmt.Errorf("Instance %v in state RUNNING but not available by SSH: %v", name, err)
			glog.Warningf("SSH encountered an error: %v, output: %v", err, sshOut)
			return false, nil
		}
		glog.Infof("Instance %v in state RUNNING and vailable by SSH", name)
		return true, nil
	})

	// If instance didn't reach running state in time, return with error now.
	if err != nil {
		return name, err
	}
	// Instance reached running state in time, make sure that cloud-init is complete
	glog.V(2).Infof("Instance %v has been created successfully", name)
	return name, nil
}

func generateMetadataWithPublicKey(pubKeyFile string) (*compute.Metadata, error) {
	publicKeyByte, err := ioutil.ReadFile(pubKeyFile)
	if err != nil {
		return nil, err
	}

	publicKey := string(publicKeyByte)

	// Take username and prepend it to the public key
	tokens := strings.Split(publicKey, " ")
	if len(tokens) != 3 {
		return nil, fmt.Errorf("Public key not comprised of 3 parts, instead was: %v", publicKey)
	}
	publicKey = strings.TrimSpace(tokens[2]) + ":" + publicKey
	newMeta := &compute.Metadata{
		Items: []*compute.MetadataItems{
			{
				Key:   "ssh-keys",
				Value: &publicKey,
			},
		},
	}
	return newMeta, nil
}

func addPubKeyToInstance(project, zone, name, pubKeyFile string) error {
	newKeys := ""
	i, err := computeService.Instances.Get(project, zone, name).Do()
	if err != nil {
		return err
	}
	fingerprint := i.Metadata.Fingerprint
	items := i.Metadata.Items
	for _, item := range items {
		if item.Key == "ssh-keys" {
			glog.V(2).Infof("Found existing ssh-keys, prepending to new key string")
			newKeys += *item.Value
			break
		}
	}
	publicKeyByte, err := ioutil.ReadFile(pubKeyFile)
	if err != nil {
		return err
	}

	publicKey := string(publicKeyByte)

	// Take username and prepend it to the public key
	tokens := strings.Split(publicKey, " ")
	if len(tokens) != 3 {
		return fmt.Errorf("Public key not comprised of 3 parts, instead was: %v", publicKey)
	}
	publicKey = strings.TrimSpace(tokens[2]) + ":" + publicKey

	newKeys = newKeys + publicKey
	glog.V(4).Infof("New ssh-keys for instance %v: %v", name, newKeys)
	newMeta := &compute.Metadata{
		Fingerprint: fingerprint,
		Items: []*compute.MetadataItems{
			{
				Key:   "ssh-keys",
				Value: &newKeys,
			},
		},
	}
	_, err = computeService.Instances.SetMetadata(project, zone, name, newMeta).Do()
	if err != nil {
		return err
	}
	return nil

}

func getexternalIP(instance *compute.Instance) string {
	for i := range instance.NetworkInterfaces {
		ni := instance.NetworkInterfaces[i]
		for j := range ni.AccessConfigs {
			ac := ni.AccessConfigs[j]
			if len(ac.NatIP) > 0 {
				return ac.NatIP
			}
		}
	}
	return ""
}

func getComputeClient() (*compute.Service, error) {
	const retries = 10
	const backoff = time.Second * 6

	// Setup the gce client for provisioning instances
	// Getting credentials on gce jenkins is flaky, so try a couple times
	var err error
	var cs *compute.Service
	for i := 0; i < retries; i++ {
		if i > 0 {
			time.Sleep(backoff)
		}

		var client *http.Client
		client, err = google.DefaultClient(context.TODO(), compute.ComputeScope)
		if err != nil {
			continue
		}

		cs, err = compute.New(client)
		if err != nil {
			continue
		}
		return cs, nil
	}
	return nil, err
}

func deleteInstance(host string) {
	glog.V(4).Infof("Deleting instance %q", host)
	_, err := computeService.Instances.Delete(*project, *zone, host).Do()
	if err != nil {
		if gce.IsGCEError(err, "notFound") {
			return
		}
		glog.Errorf("Error deleting instance %q: %v", host, err)
	}
}

func machineType(machine string) string {
	if machine == "" {
		machine = defaultMachine
	}
	return fmt.Sprintf("zones/%s/machineTypes/%s", *zone, machine)
}

// testsToGinkgoFocus converts the test string list to Ginkgo focus
func testsToGinkgoFocus(tests []string) string {
	focus := "--focus=\""
	for i, test := range tests {
		if i == 0 {
			focus += test
		} else {
			focus += ("|" + test)
		}
	}
	return focus + "\""
}
