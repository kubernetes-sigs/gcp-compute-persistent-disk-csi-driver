/*
Copyright 2018 The Kubernetes Authors.

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

package remote

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/oauth2/google"
	computealpha "google.golang.org/api/compute/v0.alpha"
	computebeta "google.golang.org/api/compute/v0.beta"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
	gce "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/compute"
)

const (
	defaultFirewallRule = "default-allow-ssh"

	// timestampFormat is the timestamp format used in the e2e directory name.
	timestampFormat = "20060102T150405"
)

// InstanceConfig is the common bundle of options used for instance creation.
type InstanceConfig struct {
	Project                   string
	Architecture              string
	Zone                      string
	Name                      string
	MachineType               string
	ServiceAccount            string
	ImageURL                  string
	CloudtopHost              bool
	MinCpuPlatform            string
	ComputeService            *compute.Service
	EnableConfidentialCompute bool
	LocalSSDCount             int64
	EnableDataCache           bool
}

type InstanceInfo struct {
	cfg InstanceConfig
	// External IP is filled in after instance creation
	externalIP string
}

func (i *InstanceInfo) GetIdentity() (string, string, string) {
	return i.cfg.Project, i.cfg.Zone, i.cfg.Name
}

func (i *InstanceInfo) GetName() string {
	return i.cfg.Name
}

func (i *InstanceInfo) GetNodeID() string {
	return common.CreateNodeID(i.cfg.Project, i.cfg.Zone, i.cfg.Name)
}

func (i *InstanceInfo) GetLocalSSD() int64 {
	return i.cfg.LocalSSDCount
}

func machineTypeMismatch(curInst *compute.Instance, newInst *compute.Instance) bool {
	if !strings.Contains(curInst.MachineType, newInst.MachineType) {
		klog.Infof("Machine type mismatch")
		return true
	}
	// Ideally we could compare to see if the new instance has a greater minCpuPlatfor
	// For now we just check it was set and it's different.
	if curInst.MinCpuPlatform != "" && curInst.MinCpuPlatform != newInst.MinCpuPlatform {
		klog.Infof("CPU Platform mismatch")
		return true
	}
	if (curInst.ConfidentialInstanceConfig != nil && newInst.ConfidentialInstanceConfig == nil) ||
		(curInst.ConfidentialInstanceConfig == nil && newInst.ConfidentialInstanceConfig != nil) ||
		(curInst.ConfidentialInstanceConfig != nil && newInst.ConfidentialInstanceConfig != nil && curInst.ConfidentialInstanceConfig.EnableConfidentialCompute != newInst.ConfidentialInstanceConfig.EnableConfidentialCompute) {
		klog.Infof("Confidential compute mismatch")
		return true
	}
	if curInst.SourceMachineImage != newInst.SourceMachineImage {
		klog.Infof("Source Machine Mismatch")
		return true
	}
	return false
}

// Provision a gce instance using image
func (i *InstanceInfo) CreateOrGetInstance(localSSDCount int) error {
	var err error
	var instance *compute.Instance
	klog.V(4).Infof("Creating instance: %v", i.cfg.Name)

	myuuid := string(uuid.NewUUID())

	err = i.createDefaultFirewallRule()
	if err != nil {
		return fmt.Errorf("Failed to create firewall rule: %v", err.Error())
	}

	newInst := &compute.Instance{
		Name:        i.cfg.Name,
		MachineType: fmt.Sprintf("zones/%s/machineTypes/%s", i.cfg.Zone, i.cfg.MachineType),
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
					SourceImage: i.cfg.ImageURL,
				},
			},
		},
		MinCpuPlatform: i.cfg.MinCpuPlatform,
	}

	if i.cfg.EnableConfidentialCompute {
		newInst.ConfidentialInstanceConfig = &compute.ConfidentialInstanceConfig{
			EnableConfidentialCompute: true,
		}
	}

	localSSDConfig := &compute.AttachedDisk{
		Type: "SCRATCH",
		InitializeParams: &compute.AttachedDiskInitializeParams{
			DiskType: fmt.Sprintf("zones/%s/diskTypes/local-ssd", i.cfg.Zone),
		},
		AutoDelete: true,
		Interface:  "NVME",
	}

	for i := 0; i < localSSDCount; i++ {
		newInst.Disks = append(newInst.Disks, localSSDConfig)
	}
	saObj := &compute.ServiceAccount{
		Email:  i.cfg.ServiceAccount,
		Scopes: []string{"https://www.googleapis.com/auth/cloud-platform"},
	}
	newInst.ServiceAccounts = []*compute.ServiceAccount{saObj}

	if pubkey, ok := os.LookupEnv("JENKINS_GCE_SSH_PUBLIC_KEY_FILE"); ok {
		klog.V(4).Infof("JENKINS_GCE_SSH_PUBLIC_KEY_FILE set to %v, adding public key to Instance", pubkey)
		meta, err := generateMetadataWithPublicKey(pubkey)
		if err != nil {
			return err
		}
		newInst.Metadata = meta
	}

	// If instance exists but settings differ, delete instance
	curInst, _ := i.cfg.ComputeService.Instances.Get(i.cfg.Project, i.cfg.Zone, newInst.Name).Do()
	if curInst != nil {
		if machineTypeMismatch(curInst, newInst) {
			klog.V(4).Infof("Instance machine type doesn't match the required one. Delete instance.")
			if _, err := i.cfg.ComputeService.Instances.Delete(i.cfg.Project, i.cfg.Zone, i.cfg.Name).Do(); err != nil {
				return err
			}

			start := time.Now()
			err := wait.Poll(15*time.Second, 5*time.Minute, func() (bool, error) {
				klog.V(2).Infof("Waiting for instance to be deleted. %v elapsed", time.Since(start))
				if curInst, _ = i.cfg.ComputeService.Instances.Get(i.cfg.Project, i.cfg.Zone, i.cfg.Name).Do(); curInst != nil {
					return false, nil
				}
				return true, nil
			})
			if err != nil {
				return err
			}
		}
	}

	if curInst == nil {
		op, err := i.cfg.ComputeService.Instances.Insert(i.cfg.Project, i.cfg.Zone, newInst).Do()
		klog.V(4).Infof("Inserted instance %v in project: %v, zone: %v", newInst.Name, i.cfg.Project, i.cfg.Zone)
		if err != nil {
			ret := fmt.Sprintf("could not create instance %s: API error: %v", i.cfg.Name, err.Error())
			if op != nil {
				ret = fmt.Sprintf("%s. op error: %v", ret, op.Error)
			}
			return errors.New(ret)
		} else if op.Error != nil {
			return fmt.Errorf("could not create instance %s: %+v", i.cfg.Name, op.Error)
		}
	} else {
		klog.V(4).Infof("Compute service GOT instance %v, skipping instance creation", newInst.Name)
	}

	start := time.Now()
	err = wait.Poll(15*time.Second, 5*time.Minute, func() (bool, error) {
		klog.V(2).Infof("Waiting for instance %v to come up. %v elapsed", i.cfg.Name, time.Since(start))

		instance, err = i.cfg.ComputeService.Instances.Get(i.cfg.Project, i.cfg.Zone, i.cfg.Name).Do()
		if err != nil {
			klog.Errorf("Failed to get instance %q: %v", i.cfg.Name, err)
			return false, nil
		}

		if strings.ToUpper(instance.Status) != "RUNNING" {
			klog.Warningf("instance %s not in state RUNNING, was %s", i.cfg.Name, instance.Status)
			return false, nil
		}

		if i.cfg.CloudtopHost {
			output, err := exec.Command("gcloud", "compute", "ssh", i.cfg.Name, "--zone", i.cfg.Zone, "--project", i.cfg.Project).CombinedOutput()
			if err != nil {
				klog.Errorf("Failed to bootstrap ssh (%v): %s", err, string(output))
				return false, nil
			}
			klog.V(4).Infof("Bootstrapped cloudtop ssh for instance %v", i.cfg.Name)
		}

		externalIP := getexternalIP(instance)
		if len(externalIP) > 0 {
			i.externalIP = externalIP
		}

		if err := i.SSHCheckAlive(); err != nil {
			err = fmt.Errorf("Instance %v in state RUNNING but not available by SSH: %v", i.cfg.Name, err.Error())
			klog.Warningf("SSH encountered an error: %v", err)
			return false, nil
		}
		klog.V(4).Infof("Instance %v in state RUNNING and available by SSH", i.cfg.Name)
		return true, nil
	})

	if err != nil {
		return fmt.Errorf("instance %v did not reach running state in time: %v", i.cfg.Name, err.Error())
	}

	// Instance reached running state in time, make sure that cloud-init is complete
	klog.V(2).Infof("Instance %v has been created successfully", i.cfg.Name)
	return nil
}

func (i *InstanceInfo) DeleteInstance() {
	klog.V(4).Infof("Deleting instance %q", i.cfg.Name)
	_, err := i.cfg.ComputeService.Instances.Delete(i.cfg.Project, i.cfg.Zone, i.cfg.Name).Do()
	if err != nil {
		if isGCEError(err, "notFound") {
			return
		}
		klog.Errorf("Error deleting instance %q: %v", i.cfg.Name, err)
	}
}

func (i *InstanceInfo) DetachDisk(diskName string) error {
	klog.V(4).Infof("Detaching disk %q", diskName)
	op, err := i.cfg.ComputeService.Instances.DetachDisk(i.cfg.Project, i.cfg.Zone, i.cfg.Name, diskName).Do()
	if err != nil {
		if isGCEError(err, "notFound") {
			return nil
		}
		klog.Errorf("Error deleting disk %q: %v", diskName, err)
	}

	start := time.Now()
	if err := wait.Poll(5*time.Second, 1*time.Minute, func() (bool, error) {
		klog.V(2).Infof("Waiting for disk %q to be detached from instance %q. %v elapsed", diskName, i.cfg.Name, time.Since(start))

		op, err = i.cfg.ComputeService.ZoneOperations.Get(i.cfg.Project, i.cfg.Zone, op.Name).Do()
		if err != nil {
			return true, fmt.Errorf("Failed to get operation %q, err: %v", op.Name, err)
		}
		return op.Status == "DONE", nil
	}); err != nil {
		return err
	}

	klog.V(4).Infof("Disk %q has been successfully detached from instance %q\n%v", diskName, i.cfg.Name, op.Error)
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

func getTimestamp() string {
	return fmt.Sprintf("%s", time.Now().Format(timestampFormat))
}

// Create default SSH filewall rule if it does not exist
func (i *InstanceInfo) createDefaultFirewallRule() error {
	var err error
	klog.V(4).Infof("Creating default firewall rule %s...", defaultFirewallRule)

	if _, err = i.cfg.ComputeService.Firewalls.Get(i.cfg.Project, defaultFirewallRule).Do(); err != nil {
		klog.V(4).Infof("Default firewall rule %v does not exist, creating", defaultFirewallRule)
		f := &compute.Firewall{
			Name: defaultFirewallRule,
			Allowed: []*compute.FirewallAllowed{
				{
					IPProtocol: "tcp",
					Ports:      []string{"22"},
				},
			},
		}
		_, err = i.cfg.ComputeService.Firewalls.Insert(i.cfg.Project, f).Do()
		if err != nil {
			if gce.IsGCEError(err, "alreadyExists") {
				klog.V(4).Infof("Default firewall rule %v already exists, skipping creation", defaultFirewallRule)
				return nil
			}
			return fmt.Errorf("Failed to insert required default SSH firewall Rule %v: %v", defaultFirewallRule, err.Error())
		}
	} else {
		klog.V(4).Infof("Default firewall rule %v already exists, skipping creation", defaultFirewallRule)
	}
	return nil
}

func GetComputeClient() (*compute.Service, error) {
	const retries = 10
	const backoff = time.Second * 6

	klog.V(4).Infof("Getting compute client...")

	// Setup the gce client for provisioning instances
	// Getting credentials on gce jenkins is flaky, so try a couple times
	var err error
	var cs *compute.Service
	for i := 0; i < retries; i++ {
		if i > 0 {
			time.Sleep(backoff)
		}

		var client *http.Client
		client, err = google.DefaultClient(context.Background(), compute.ComputeScope)
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

func GetComputeAlphaClient() (*computealpha.Service, error) {
	const retries = 10
	const backoff = time.Second * 6

	klog.V(4).Infof("Getting compute client...")

	// Setup the gce client for provisioning instances
	// Getting credentials on gce jenkins is flaky, so try a couple times
	var err error
	var cs *computealpha.Service
	for i := 0; i < retries; i++ {
		if i > 0 {
			time.Sleep(backoff)
		}

		var client *http.Client
		client, err = google.DefaultClient(context.Background(), computealpha.ComputeScope)
		if err != nil {
			continue
		}

		cs, err = computealpha.New(client)
		if err != nil {
			continue
		}
		return cs, nil
	}
	return nil, err
}

func GetComputeBetaClient() (*computebeta.Service, error) {
	const retries = 10
	const backoff = time.Second * 6

	klog.V(4).Infof("Getting compute client...")

	// Setup the gce client for provisioning instances
	// Getting credentials on gce jenkins is flaky, so try a couple times
	var err error
	var cs *computebeta.Service
	for i := 0; i < retries; i++ {
		if i > 0 {
			time.Sleep(backoff)
		}

		var client *http.Client
		client, err = google.DefaultClient(context.Background(), computebeta.ComputeScope)
		if err != nil {
			continue
		}

		cs, err = computebeta.New(client)
		if err != nil {
			continue
		}
		return cs, nil
	}
	return nil, err
}

func generateMetadataWithPublicKey(pubKeyFile string) (*compute.Metadata, error) {
	publicKeyByte, err := os.ReadFile(pubKeyFile)
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

// isGCEError returns true if given error is a googleapi.Error with given
// reason (e.g. "resourceInUseByAnotherResource")
func isGCEError(err error, reason string) bool {
	var apiErr *googleapi.Error
	if !errors.As(err, &apiErr) {
		return false
	}

	for _, e := range apiErr.Errors {
		if e.Reason == reason {
			return true
		}
	}
	return false
}
