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
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2/google"
	computealpha "google.golang.org/api/compute/v0.alpha"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
	gce "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/compute"
)

const (
	defaultFirewallRule = "default-allow-ssh"

	// timestampFormat is the timestamp format used in the e2e directory name.
	timestampFormat = "20060102T150405"
)

type InstanceInfo struct {
	project      string
	architecture string
	zone         string
	name         string
	machineType  string

	// External IP is filled in after instance creation
	externalIP string

	computeService *compute.Service
}

func (i *InstanceInfo) GetIdentity() (string, string, string) {
	return i.project, i.zone, i.name
}

func (i *InstanceInfo) GetName() string {
	return i.name
}

func (i *InstanceInfo) GetNodeID() string {
	return common.CreateNodeID(i.project, i.zone, i.name)
}

func CreateInstanceInfo(project, instanceArchitecture, instanceZone, name, machineType string, cs *compute.Service) (*InstanceInfo, error) {
	return &InstanceInfo{
		project:      project,
		architecture: instanceArchitecture,
		zone:         instanceZone,
		name:         name,
		machineType:  machineType,

		computeService: cs,
	}, nil
}

// Provision a gce instance using image
func (i *InstanceInfo) CreateOrGetInstance(imageURL, serviceAccount string) error {
	var err error
	var instance *compute.Instance
	klog.V(4).Infof("Creating instance: %v", i.name)

	myuuid := string(uuid.NewUUID())

	err = i.createDefaultFirewallRule()
	if err != nil {
		return fmt.Errorf("Failed to create firewall rule: %v", err)
	}

	newInst := &compute.Instance{
		Name:        i.name,
		MachineType: fmt.Sprintf("zones/%s/machineTypes/%s", i.zone, i.machineType),
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
	newInst.ServiceAccounts = []*compute.ServiceAccount{saObj}

	if pubkey, ok := os.LookupEnv("JENKINS_GCE_SSH_PUBLIC_KEY_FILE"); ok {
		klog.V(4).Infof("JENKINS_GCE_SSH_PUBLIC_KEY_FILE set to %v, adding public key to Instance", pubkey)
		meta, err := generateMetadataWithPublicKey(pubkey)
		if err != nil {
			return err
		}
		newInst.Metadata = meta
	}

	curInst, _ := i.computeService.Instances.Get(i.project, i.zone, newInst.Name).Do()
	if curInst != nil {
		if !strings.Contains(curInst.MachineType, newInst.MachineType) {
			klog.V(4).Infof("Instance machine type doesn't match the required one. Remove instance.")
			if _, err := i.computeService.Instances.Delete(i.project, i.zone, i.name).Do(); err != nil {
				return err
			}

			then := time.Now()
			err := wait.Poll(15*time.Second, 5*time.Minute, func() (bool, error) {
				klog.V(2).Infof("Waiting for instance to be deleted. %v elapsed", time.Since(then))
				if instance, _ = i.computeService.Instances.Get(i.project, i.zone, i.name).Do(); instance != nil {
					return false, nil
				}
				return true, nil
			})
			if err != nil {
				return err
			}

			if err := insertInstance(i, newInst); err != nil {
				return err
			}
		} else {
			klog.V(4).Infof("Compute service GOT instance %v, skipping instance creation", newInst.Name)
		}
	} else {
		if err := insertInstance(i, newInst); err != nil {
			return err
		}
	}

	then := time.Now()
	err = wait.Poll(15*time.Second, 5*time.Minute, func() (bool, error) {
		klog.V(2).Infof("Waiting for instance %v to come up. %v elapsed", i.name, time.Since(then))

		instance, err = i.computeService.Instances.Get(i.project, i.zone, i.name).Do()
		if err != nil {
			klog.Errorf("Failed to get instance %v: %v", i.name, err)
			return false, nil
		}

		if strings.ToUpper(instance.Status) != "RUNNING" {
			klog.Warningf("instance %s not in state RUNNING, was %s", i.name, instance.Status)
			return false, nil
		}

		externalIP := getexternalIP(instance)
		if len(externalIP) > 0 {
			i.externalIP = externalIP
		}

		if sshOut, err := i.SSHCheckAlive(); err != nil {
			err = fmt.Errorf("Instance %v in state RUNNING but not available by SSH: %v", i.name, err)
			klog.Warningf("SSH encountered an error: %v, output: %v", err, sshOut)
			return false, nil
		}
		klog.V(4).Infof("Instance %v in state RUNNING and available by SSH", i.name)
		return true, nil
	})

	// If instance didn't reach running state in time, return with error now.
	if err != nil {
		return err
	}

	// Instance reached running state in time, make sure that cloud-init is complete
	klog.V(2).Infof("Instance %v has been created successfully", i.name)
	return nil
}

func insertInstance(i *InstanceInfo, newInst *compute.Instance) error {
	op, err := i.computeService.Instances.Insert(i.project, i.zone, newInst).Do()
	klog.V(4).Infof("Inserted instance %v in project: %v, zone: %v", newInst.Name, i.project, i.zone)
	if err != nil {
		ret := fmt.Sprintf("could not create instance %s: API error: %v", i.name, err)
		if op != nil {
			ret = fmt.Sprintf("%s. op error: %v", ret, op.Error)
		}
		return errors.New(ret)
	} else if op.Error != nil {
		return fmt.Errorf("could not create instance %s: %+v", i.name, op.Error)
	}
	return nil
}

func (i *InstanceInfo) DeleteInstance() {
	klog.V(4).Infof("Deleting instance %q", i.name)
	_, err := i.computeService.Instances.Delete(i.project, i.zone, i.name).Do()
	if err != nil {
		if isGCEError(err, "notFound") {
			return
		}
		klog.Errorf("Error deleting instance %q: %v", i.name, err)
	}
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
	return fmt.Sprintf(time.Now().Format(timestampFormat))
}

// Create default SSH filewall rule if it does not exist
func (i *InstanceInfo) createDefaultFirewallRule() error {
	var err error
	klog.V(4).Infof("Creating default firewall rule %s...", defaultFirewallRule)

	if _, err = i.computeService.Firewalls.Get(i.project, defaultFirewallRule).Do(); err != nil {
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
		_, err = i.computeService.Firewalls.Insert(i.project, f).Do()
		if err != nil {
			if gce.IsGCEError(err, "alreadyExists") {
				klog.V(4).Infof("Default firewall rule %v already exists, skipping creation", defaultFirewallRule)
				return nil
			}
			return fmt.Errorf("Failed to insert required default SSH firewall Rule %v: %v", defaultFirewallRule, err)
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

// isGCEError returns true if given error is a googleapi.Error with given
// reason (e.g. "resourceInUseByAnotherResource")
func isGCEError(err error, reason string) bool {
	apiErr, ok := err.(*googleapi.Error)
	if !ok {
		return false
	}

	for _, e := range apiErr.Errors {
		if e.Reason == reason {
			return true
		}
	}
	return false
}
