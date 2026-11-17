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

package utils

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2/google"
	cloudresourcemanager "google.golang.org/api/cloudresourcemanager/v1"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	boskosclient "sigs.k8s.io/boskos/client"
	"sigs.k8s.io/boskos/common"
	utilcommon "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
	remote "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/test/remote"
)

const (
	DiskLabelKey   = "csi"
	DiskLabelValue = "e2e-test"
)

var (
	boskos, _ = boskosclient.NewClient(os.Getenv("JOB_NAME"), "http://boskos", "", "")
)

type DriverConfig struct {
	ComputeEndpoint string
	ExtraFlags      []string
	Zones           []string
}

// getPackageRoot starts in the current working directory and searches upwards
// through parent directories until it finds the first directory containing "go.mod".
// It returns the absolute path of that directory.
func getPackageRoot() (string, error) {
	currentDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current working directory: %w", err)
	}

	for {
		goModPath := filepath.Join(currentDir, "go.mod")
		info, err := os.Stat(goModPath)
		if err == nil && !info.IsDir() {
			klog.Infof("Found package root %s", currentDir)
			return currentDir, nil
		}
		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			break
		}
		currentDir = parentDir
	}

	return "", fmt.Errorf("Could not find package root: go.mod not found in current or any parent directories")
}

func GCEClientAndDriverSetup(instance *remote.InstanceInfo, driverConfig DriverConfig) (*remote.TestContext, error) {
	port := fmt.Sprintf("%v", 1024+rand.Intn(10000))
	var pkgPath string
	goPath, ok := os.LookupEnv("GOPATH")
	if ok && goPath != "" {
		klog.Infof("Using GOPATH for package")
		pkgPath = path.Join(goPath, "src/sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/")
	} else {
		var err error
		if pkgPath, err = getPackageRoot(); err != nil {
			return nil, err
		}
	}

	binPath := path.Join(pkgPath, "bin/gce-pd-csi-driver")

	endpoint := fmt.Sprintf("tcp://localhost:%s", port)
	extra_flags := []string{
		fmt.Sprintf("--extra-labels=%s=%s", DiskLabelKey, DiskLabelValue),
		"--max-concurrent-format-and-mount=20", // otherwise the serialization times out the e2e test.
		"--multi-zone-volume-handle-enable",
		"--multi-zone-volume-handle-disk-types=pd-standard,hyperdisk-ml",
		"--use-instance-api-to-poll-attachment-disk-types=pd-ssd",
		"--use-instance-api-to-list-volumes-published-nodes",
		"--supports-dynamic-iops-provisioning=hyperdisk-balanced,hyperdisk-extreme",
		"--supports-dynamic-throughput-provisioning=hyperdisk-balanced,hyperdisk-throughput,hyperdisk-ml",
		"--allow-hdha-provisioning",
		"--device-in-use-timeout=10s", // Set lower than the usual value to expedite tests
		fmt.Sprintf("--fallback-requisite-zones=%s", strings.Join(driverConfig.Zones, ",")),
		"--disk-topology=true",
		"--dynamic-volumes=true",
	}

	if instance.HasLocalSSD() {
		extra_flags = append(extra_flags, "--enable-data-cache", "--enable-non-k8s-data-cache-if-ssds")
	}
	extra_flags = append(extra_flags, fmt.Sprintf("--compute-endpoint=%s", driverConfig.ComputeEndpoint))
	extra_flags = append(extra_flags, driverConfig.ExtraFlags...)

	workspace := remote.NewWorkspaceDir("gce-pd-e2e-")
	// Log at V(6) as the compute API calls are emitted at that level and it's
	// useful to see what's happening when debugging tests.
	driverRunCmd := fmt.Sprintf("sh -c '/usr/bin/nohup %s/gce-pd-csi-driver -v=6 --endpoint=%s %s 2> %s/prog.out < /dev/null > /dev/null &'",
		workspace, endpoint, strings.Join(extra_flags, " "), workspace)
	config := &remote.ClientConfig{
		PkgPath:      pkgPath,
		BinPath:      binPath,
		WorkspaceDir: workspace,
		RunDriverCmd: driverRunCmd,
		Port:         port,
	}

	err := os.Setenv("GCE_PD_CSI_STAGING_VERSION", "latest")
	if err != nil {
		return nil, err
	}

	return remote.SetupNewDriverAndClient(instance, config)
}

// getBoskosProject retries acquiring a boskos project until success or timeout
func getBoskosProject(resourceType string) *common.Resource {
	timer := time.NewTimer(30 * time.Minute)
	defer timer.Stop()
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-timer.C:
			klog.Fatalf("timed out trying to acquire boskos project")
		case <-ticker.C:
			p, err := boskos.Acquire(resourceType, "free", "busy")
			if err != nil {
				klog.Warningf("boskos failed to acquire project: %v", err)
			} else if p == nil {
				klog.Warningf("boskos does not have a free %s at the moment", resourceType)
			} else {
				return p
			}
		}
	}

}

func SetupProwConfig(resourceType string) (project, serviceAccount string) {
	// Try to get a Boskos project
	klog.V(4).Infof("Running in PROW")
	klog.V(4).Infof("Fetching a Boskos loaned project")

	p := getBoskosProject(resourceType)
	project = p.Name

	go func(c *boskosclient.Client, proj string) {
		for range time.Tick(time.Minute * 5) {
			if err := c.UpdateOne(p.Name, "busy", nil); err != nil {
				klog.Warningf("[Boskos] Update %s failed with %v", p.Name, err)
			}
		}
	}(boskos, p.Name)

	// If we're on CI overwrite the service account
	klog.V(4).Infof("Fetching the default compute service account")

	c, err := google.DefaultClient(context.Background(), cloudresourcemanager.CloudPlatformScope)
	if err != nil {
		klog.Fatalf("Failed to get Google Default Client: %v", err)
	}

	cloudresourcemanagerService, err := cloudresourcemanager.New(c)
	if err != nil {
		klog.Fatalf("Failed to create new cloudresourcemanager: %v", err)
	}

	resp, err := cloudresourcemanagerService.Projects.Get(project).Do()
	if err != nil {
		klog.Fatalf("Failed to get project %v from Cloud Resource Manager: %v", project, err)
	}

	// Default Compute Engine service account
	// [PROJECT_NUMBER]-compute@developer.gserviceaccount.com
	serviceAccount = fmt.Sprintf("%v-compute@developer.gserviceaccount.com", resp.ProjectNumber)
	klog.Infof("Using project %v and service account %v", project, serviceAccount)
	return project, serviceAccount
}

func ForceChmod(instance *remote.InstanceInfo, filePath string, perms string, recursive bool) error {
	originalumask, err := instance.SSHNoSudo("umask")
	if err != nil {
		return fmt.Errorf("failed to umask. Output: %v, errror: %v", originalumask, err.Error())
	}
	output, err := instance.SSHNoSudo("umask", "0000")
	if err != nil {
		return fmt.Errorf("failed to umask. Output: %v, errror: %v", output, err.Error())
	}
	chmodOptions := []string{}
	if recursive {
		chmodOptions = []string{"-R"}
	}
	chmodOptions = append(chmodOptions, perms, filePath)
	chmodCmd := append([]string{"chmod"}, chmodOptions...)
	output, err = instance.SSH(chmodCmd...)
	if err != nil {
		return fmt.Errorf("failed to chmod file %s. Output: %v, errror: %v", filePath, output, err.Error())
	}
	output, err = instance.SSHNoSudo("umask", originalumask)
	if err != nil {
		return fmt.Errorf("failed to umask. Output: %v, errror: %v", output, err.Error())
	}
	return nil
}

func WriteFile(instance *remote.InstanceInfo, filePath, fileContents string) error {
	output, err := instance.SSHNoSudo("echo", fileContents, ">", filePath)
	if err != nil {
		return fmt.Errorf("failed to write test file %s. Output: %v, errror: %v", filePath, output, err.Error())
	}
	return nil
}

func ReadFile(instance *remote.InstanceInfo, filePath string) (string, error) {
	output, err := instance.SSHNoSudo("cat", filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read test file %s. Output: %v, errror: %v", filePath, output, err.Error())
	}
	return output, nil
}

func WriteBlock(instance *remote.InstanceInfo, path, fileContents string) error {
	output, err := instance.SSHNoSudo("echo", fileContents, "|", "dd", "of="+path)
	if err != nil {
		return fmt.Errorf("failed to write test file %s. Output: %v, errror: %v", path, output, err.Error())
	}
	return nil
}

func ReadBlock(instance *remote.InstanceInfo, path string, length int) (string, error) {
	lengthStr := strconv.Itoa(length)
	output, err := instance.SSHNoSudo("dd", "if="+path, "bs="+lengthStr, "count=1", "2>", "/dev/null")
	if err != nil {
		return "", fmt.Errorf("failed to read test file %s. Output: %v, errror: %v", path, output, err.Error())
	}
	return output, nil
}

func GetFSSizeInGb(instance *remote.InstanceInfo, mountPath string) (int64, error) {
	output, err := instance.SSH("df", "--output=size", "-BG", mountPath, "|", "awk", "'NR==2'")
	if err != nil {
		return -1, fmt.Errorf("failed to get size of path %s. Output: %v, error: %v", mountPath, output, err.Error())
	}
	output = strings.TrimSuffix(strings.TrimSpace(output), "G")
	n, err := strconv.ParseInt(output, 10, 64)
	if err != nil {
		return -1, fmt.Errorf("failed to parse size %s into int", output)
	}
	return n, nil
}

func GetBlockSizeInGb(instance *remote.InstanceInfo, devicePath string) (int64, error) {
	output, err := instance.SSH("blockdev", "--getsize64", devicePath)
	if err != nil {
		return -1, fmt.Errorf("failed to get size of path %s. Output: %v, error: %v", devicePath, output, err.Error())
	}
	n, err := strconv.ParseInt(strings.TrimSpace(output), 10, 64)
	if err != nil {
		return -1, fmt.Errorf("failed to parse size %s into int", output)
	}
	return utilcommon.BytesToGbRoundDown(n), nil
}

func Symlink(instance *remote.InstanceInfo, src, dest string) error {
	output, err := instance.SSH("ln", "-s", src, dest)
	if err != nil {
		return fmt.Errorf("failed to symlink from %s to %s. Output: %v, errror: %v", src, dest, output, err.Error())
	}
	return nil
}

func RmAll(instance *remote.InstanceInfo, filePath string) error {
	output, err := instance.SSH("rm", "-rf", filePath)
	if err != nil {
		return fmt.Errorf("failed to delete all %s. Output: %v, errror: %v", filePath, output, err.Error())
	}
	return nil
}

func MkdirAll(instance *remote.InstanceInfo, dir string) error {
	output, err := instance.SSH("mkdir", "-p", dir)
	if err != nil {
		return fmt.Errorf("failed to mkdir -p %s. Output: %v, errror: %v", dir, output, err.Error())
	}
	return nil
}

func CopyFile(instance *remote.InstanceInfo, src, dest string) error {
	output, err := instance.SSH("cp", "-f", src, dest)
	if err != nil {
		return fmt.Errorf("failed to copy %s to %s. Output: %v, errror: %v", src, dest, output, err.Error())
	}
	return nil
}

func InstallDependencies(instance *remote.InstanceInfo, pkgs []string) error {
	_, _ = instance.SSH("apt-get", "update")
	cmdline := []string{"apt-get", "install", "-y"}
	cmdline = append(cmdline, pkgs...)
	output, err := instance.SSH(cmdline...)
	if err != nil {
		return fmt.Errorf("failed to install package %v. Output: %v, error: %v", pkgs, output, err.Error())
	}
	return nil
}

func CleanUpAnyExistingLVMInstances(instance *remote.InstanceInfo) error {
	// Note this depends on the script installed from remote.SetupNewDriverAndClient
	output, err := instance.SSH("/bin/bash", filepath.Join("/tmp", remote.LvmCleanupScript))
	if err == nil {
		klog.Infof("LVM cleanup output: %v", output)
	}
	if err != nil {
		return fmt.Errorf("LVM cleanup script failed: %v; %v", err, output)
	}
	return nil
}

func SetupDataCachingConfig(instance *remote.InstanceInfo) error {
	output, err := instance.SSH("/bin/sed", "-i", "-e", "\"s/.*allow_mixed_block_sizes = 0.*/	allow_mixed_block_sizes = 1/\"", "/etc/lvm/lvm.conf")
	if err != nil {
		return fmt.Errorf("failed to update field allow_mixed_block_sizes, error: %w; output: %v", err, output)
	}
	output, err = instance.SSH("/bin/sed", "-i", "-e", "\"s/.*udev_sync = 1.*/ udev_sync = 0/\"", "/etc/lvm/lvm.conf")
	if err != nil {
		return fmt.Errorf("failed to update field udev_sync, error: %w; output: %v", err, output)
	}
	output, err = instance.SSH("/bin/sed", "-i", "-e", "\"s/.*udev_rules = 1.*/ udev_rules = 0/\"", "/etc/lvm/lvm.conf")
	if err != nil {
		return fmt.Errorf("failed to update field udev_rules, error: %w; output: %v", err, output)
	}
	output, err = instance.SSH("/bin/sed", "-i", "-e", "\"s/.*event_activation = 1.*/ event_activation = 0/\"", "/etc/lvm/lvm.conf")
	if err != nil {
		return fmt.Errorf("failed to update field event_activation, error: %w; output: %v", err, output)
	}
	output, err = instance.SSH("/bin/sed", "-i", "-e", "'/^[[:space:]]*backup[[:space:]]*{/,/^[[:space:]]*}/s/^[[:space:]]*#\\{0,1\\}[[:space:]]*backup[[:space:]]*=[[:space:]]*[01][[:space:]]*\\(#.*\\)\\{0,1\\}$/\tbackup = 0/'", "/etc/lvm/lvm.conf")
	if err != nil {
		return fmt.Errorf("failed to update field backup, error: %w; output: %v", err, output)
	}
	output, err = instance.SSH("/bin/sed", "-i", "-e", "'/^[[:space:]]*backup[[:space:]]*{/,/^[[:space:]]*}/s/^[[:space:]]*#\\{0,1\\}[[:space:]]*archive[[:space:]]*=[[:space:]]*[01][[:space:]]*\\(#.*\\)\\{0,1\\}$/\tarchive = 0/'", "/etc/lvm/lvm.conf")
	if err != nil {
		return fmt.Errorf("failed to update field archive, error: %w; output: %v", err, output)
	}
	return nil
}

// ValidateLogicalLinkIsDisk takes a symlink location at "link" and finds the
// link location - it then finds the backing PD using either scsi_id or
// google_nvme_id (depending on the /dev path) and validates that it is the
// same as diskName
func ValidateLogicalLinkIsDisk(instance *remote.InstanceInfo, link, diskName string) (bool, error) {
	const (
		scsiPattern       = `^0Google\s+PersistentDisk\s+([\S]+)\s*$`
		sdPattern         = `sd\w+`
		nvmeSerialPattern = `ID_SERIAL_SHORT=([\S]+)\s*`
		nvmeDevPattern    = `nvme\w+`
	)
	// regex to parse scsi_id output and extract the serial
	scsiRegex := regexp.MustCompile(scsiPattern)
	sdRegex := regexp.MustCompile(sdPattern)
	nvmeSerialRegex := regexp.MustCompile(nvmeSerialPattern)
	nvmeDevRegex := regexp.MustCompile(nvmeDevPattern)

	devFsPath, err := instance.SSH("find", link, "-printf", "'%l'")
	if err != nil {
		// Skip over if there is no matching symlink.
		return false, nil
	}
	if len(devFsPath) == 0 {
		return false, nil
	}

	if sdx := sdRegex.FindString(devFsPath); len(sdx) != 0 {
		fullDevPath := path.Join("/dev/", string(sdx))
		scsiIDOut, err := instance.SSH("/lib/udev_containerized/scsi_id", "--page=0x83", "--whitelisted", fmt.Sprintf("--device=%v", fullDevPath))
		if err != nil {
			return false, fmt.Errorf("failed to find %s's SCSI ID. Output: %v, errror: %v", devFsPath, scsiIDOut, err.Error())
		}
		scsiID := scsiRegex.FindStringSubmatch(scsiIDOut)
		if len(scsiID) == 0 {
			return false, fmt.Errorf("scsi_id output cannot be parsed: %s. Output: %v", scsiID, scsiIDOut)
		}
		if scsiID[1] != diskName {
			return false, fmt.Errorf("scsiID %s did not match expected diskName %s", scsiID, diskName)
		}
		return true, nil
	} else if nvmex := nvmeDevRegex.FindString(devFsPath); len(nvmex) != 0 {
		fullDevPath := path.Join("/dev/", string(nvmex))
		nvmeIDOut, err := instance.SSH("/lib/udev_containerized/google_nvme_id", fmt.Sprintf("-d%v", fullDevPath))
		if err != nil {
			return false, fmt.Errorf("failed to find %s's NVME ID. Output: %v, errror: %v", devFsPath, nvmeIDOut, err.Error())
		}
		nvmeID := nvmeSerialRegex.FindStringSubmatch(nvmeIDOut)
		if len(nvmeID) == 0 {
			return false, fmt.Errorf("google_nvme_id output cannot be parsed: %s. Output: %v", nvmeID, nvmeIDOut)
		}
		if nvmeID[1] != diskName {
			return false, fmt.Errorf("nvmeID %s did not match expected diskName %s", nvmeID, diskName)
		}
		return true, nil
	}
	return false, fmt.Errorf("symlinked disk %s for diskName %s does not match a supported /dev/sd* or /dev/nvme* path", devFsPath, diskName)
}

const (
	driverName                  = "pd.csi.storage.gke.io"
	diskTypeAnnotation          = "pdcsi.gke.io/disk-type-conversion-operation"
	diskTypeTargetAnnotation    = "pdcsi.gke.io/disk-type-conversion-target"
	diskTypeLastErrorAnnotation = "pdcsi.gke.io/disk-type-conversion-last-error"
	conversionPending           = "Pending"
	conversionReconciling       = "Reconciling"
	sourceDiskType              = "pd-balanced"
	targetDiskType              = "hyperdisk-balanced"
	driverContainerName         = "gce-pd-driver"
)

type DiskConversionTestT interface {
	Cleanup(func())
	Errorf(format string, args ...any)
	Fatal(args ...any)
	Fatalf(format string, args ...any)
	Helper()
}

type DiskConversionEnvironment struct {
	client         *kubernetes.Clientset
	compute        *compute.Service
	project        string
	namespace      string
	deploymentName string
	zone           string
	nodeName       string
}

type DiskConversionFixture struct {
	env          *DiskConversionEnvironment
	name         string
	volumeHandle string
	pvName       string
	namespace    string
	claimName    string
	podName      string
	vacNames     []string
	snapshotName string
	snapshotOp   string
}

func SetupDiskConversionE2E(t DiskConversionTestT) (context.Context, context.CancelFunc, *DiskConversionEnvironment, func(context.Context) error) {
	t.Helper()
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		t.Fatal("KUBECONFIG must point to a Kubernetes cluster with the PD CSI driver deployed")
	}
	project := os.Getenv("GCP_PROJECT")
	if project == "" {
		project = os.Getenv("GOOGLE_CLOUD_PROJECT")
	}
	if project == "" {
		t.Fatal("GCP_PROJECT or GOOGLE_CLOUD_PROJECT must identify the project containing the test disks")
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		t.Fatalf("load Kubernetes configuration: %v", err)
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatalf("create Kubernetes client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Hour)
	setupComplete := false
	defer func() {
		if !setupComplete {
			cancel()
		}
	}()
	computeClient, err := compute.NewService(ctx, option.WithScopes(compute.CloudPlatformScope))
	if err != nil {
		t.Fatalf("create Compute Engine client: %v", err)
	}

	namespace := envOrDefault("PD_CSI_DRIVER_NAMESPACE", "gce-pd-csi-driver")
	deploymentName := envOrDefault("PD_CSI_CONTROLLER_DEPLOYMENT", "csi-gce-pd-controller")
	node, err := hyperdiskCompatibleNode(ctx, client)
	if err != nil {
		t.Fatal(err)
	}
	env := &DiskConversionEnvironment{
		client:         client,
		compute:        computeClient,
		project:        project,
		namespace:      namespace,
		deploymentName: deploymentName,
		zone:           node.Labels[corev1.LabelTopologyZone],
		nodeName:       node.Name,
	}
	if env.zone == "" {
		t.Fatalf("node %s has no %s label", node.Name, corev1.LabelTopologyZone)
	}
	restore, err := EnableDiskConversion(ctx, client, namespace, deploymentName)
	if err != nil {
		t.Fatalf("enable disk conversion on controller deployment %s/%s: %v", namespace, deploymentName, err)
	}
	setupComplete = true
	return ctx, cancel, env, restore
}

func NewDiskConversionFixture(t DiskConversionTestT, env *DiskConversionEnvironment) *DiskConversionFixture {
	t.Helper()
	name := "pdconv-" + strings.ReplaceAll(strings.ToLower(string(uuid.NewUUID())), "-", "")[:12]
	fixture := &DiskConversionFixture{
		env:          env,
		name:         name,
		volumeHandle: fmt.Sprintf("projects/%s/zones/%s/disks/%s", env.project, env.zone, name),
		pvName:       name,
		namespace:    "default",
		claimName:    name,
		podName:      name + "-pod",
	}

	disk := &compute.Disk{
		Name:   name,
		Type:   fmt.Sprintf("zones/%s/diskTypes/%s", env.zone, sourceDiskType),
		SizeGb: 110,
		Labels: map[string]string{"pdcsi-e2e": "true"},
	}
	t.Cleanup(func() {
		fixture.cleanup(t)
	})
	op, err := env.compute.Disks.Insert(env.project, env.zone, disk).Context(context.Background()).Do()
	if err != nil {
		t.Fatalf("create source disk %s: %v", name, err)
	}
	if err := waitZoneOperation(context.Background(), env.compute, env.project, env.zone, op.Name, 10*time.Minute); err != nil {
		t.Fatalf("wait for source disk %s creation: %v", name, err)
	}

	fixture.createStaticPVAndPVC(t)
	return fixture
}

func (f *DiskConversionFixture) createStaticPVAndPVC(t DiskConversionTestT) {
	t.Helper()
	ctx := context.Background()
	className := ""
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: f.pvName},
		Spec: corev1.PersistentVolumeSpec{
			Capacity:                      corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("100Gi")},
			AccessModes:                   []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
			StorageClassName:              className,
			ClaimRef: &corev1.ObjectReference{
				Namespace: f.namespace,
				Name:      f.claimName,
			},
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					Driver:       driverName,
					VolumeHandle: f.volumeHandle,
					FSType:       "ext4",
				},
			},
			NodeAffinity: &corev1.VolumeNodeAffinity{
				Required: &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{
					MatchExpressions: []corev1.NodeSelectorRequirement{{
						Key: corev1.LabelTopologyZone, Operator: corev1.NodeSelectorOpIn, Values: []string{f.env.zone},
					}},
				}}},
			},
		},
	}
	if _, err := f.env.client.CoreV1().PersistentVolumes().Create(ctx, pv, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create static PV %s: %v", f.pvName, err)
	}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Namespace: f.namespace, Name: f.claimName},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			StorageClassName: &className,
			VolumeName:       f.pvName,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("100Gi")},
			},
		},
	}
	if _, err := f.env.client.CoreV1().PersistentVolumeClaims(f.namespace).Create(ctx, pvc, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create PVC %s/%s: %v", f.namespace, f.claimName, err)
	}
	if err := poll(ctx, 3*time.Minute, time.Second, func() (bool, error) {
		current, err := f.env.client.CoreV1().PersistentVolumeClaims(f.namespace).Get(ctx, f.claimName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		return current.Status.Phase == corev1.ClaimBound, nil
	}); err != nil {
		t.Fatalf("wait for PVC %s/%s to bind: %v", f.namespace, f.claimName, err)
	}
}

func (f *DiskConversionFixture) RequestConversion(t DiskConversionTestT, ctx context.Context, iops, throughput int64) {
	t.Helper()
	vacName := f.name + "-vac"
	vac := &storagev1.VolumeAttributesClass{
		ObjectMeta: metav1.ObjectMeta{Name: vacName},
		DriverName: driverName,
		Parameters: map[string]string{
			"type":       targetDiskType,
			"iops":       fmt.Sprintf("%d", iops),
			"throughput": fmt.Sprintf("%dMi", throughput),
		},
	}
	if _, err := f.env.client.StorageV1().VolumeAttributesClasses().Create(ctx, vac, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create VAC %s: %v", vacName, err)
	}
	f.vacNames = append(f.vacNames, vacName)
	if err := updatePVC(ctx, f.env, f.namespace, f.claimName, func(pvc *corev1.PersistentVolumeClaim) {
		pvc.Spec.VolumeAttributesClassName = ptr.To(vacName)
	}); err != nil {
		t.Fatalf("set PVC %s/%s VolumeAttributesClassName: %v", f.namespace, f.claimName, err)
	}
}

func (f *DiskConversionFixture) ClearVolumeAttributesClass(t DiskConversionTestT, ctx context.Context) {
	t.Helper()
	if err := updatePVC(ctx, f.env, f.namespace, f.claimName, func(pvc *corev1.PersistentVolumeClaim) {
		pvc.Spec.VolumeAttributesClassName = nil
	}); err != nil {
		t.Fatalf("clear PVC %s/%s VolumeAttributesClassName: %v", f.namespace, f.claimName, err)
	}
}

func (f *DiskConversionFixture) StartPod(t DiskConversionTestT, ctx context.Context) {
	t.Helper()
	f.CreatePod(t, ctx)
	if err := f.waitForPodRunning(ctx, 15*time.Minute); err != nil {
		t.Fatalf("wait for workload Pod %s/%s to mount the disk: %v", f.namespace, f.podName, err)
	}
}

func (f *DiskConversionFixture) CreatePod(t DiskConversionTestT, ctx context.Context) {
	t.Helper()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: f.namespace, Name: f.podName},
		Spec: corev1.PodSpec{
			NodeName: f.env.nodeName,
			Containers: []corev1.Container{{
				Name:  "volume-user",
				Image: "registry.k8s.io/pause:3.10",
				VolumeMounts: []corev1.VolumeMount{{
					Name: "disk", MountPath: "/mnt/disk",
				}},
			}},
			Volumes: []corev1.Volume{{
				Name: "disk",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: f.claimName},
				},
			}},
		},
	}
	if _, err := f.env.client.CoreV1().Pods(f.namespace).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create workload Pod %s/%s: %v", f.namespace, f.podName, err)
	}
}

func (f *DiskConversionFixture) StopPod(t DiskConversionTestT, ctx context.Context) {
	t.Helper()
	err := f.env.client.CoreV1().Pods(f.namespace).Delete(ctx, f.podName, metav1.DeleteOptions{})
	if err != nil && !apiNotFound(err) {
		t.Fatalf("delete workload Pod %s/%s: %v", f.namespace, f.podName, err)
	}
	if err := poll(ctx, 5*time.Minute, 2*time.Second, func() (bool, error) {
		_, err := f.env.client.CoreV1().Pods(f.namespace).Get(ctx, f.podName, metav1.GetOptions{})
		if apiNotFound(err) {
			return true, nil
		}
		return false, err
	}); err != nil {
		t.Fatalf("wait for workload Pod %s/%s deletion: %v", f.namespace, f.podName, err)
	}
	if err := f.waitForDiskDetached(ctx, 5*time.Minute); err != nil {
		t.Fatalf("wait for disk %s to detach: %v", f.name, err)
	}
}

func (f *DiskConversionFixture) waitForPodRunning(ctx context.Context, timeout time.Duration) error {
	podCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return poll(podCtx, timeout, 2*time.Second, func() (bool, error) {
		pod, err := f.env.client.CoreV1().Pods(f.namespace).Get(podCtx, f.podName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if pod.Status.Phase == corev1.PodFailed {
			return false, fmt.Errorf("Pod failed: %s", pod.Status.Message)
		}
		return pod.Status.Phase == corev1.PodRunning, nil
	})
}

func (f *DiskConversionFixture) WaitForPodAndDisk(ctx context.Context, expectedType string, timeout time.Duration) error {
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return poll(checkCtx, timeout, 2*time.Second, func() (bool, error) {
		disk, err := f.env.compute.Disks.Get(f.env.project, f.env.zone, f.name).Context(checkCtx).Do()
		if err != nil {
			return false, err
		}
		pod, err := f.env.client.CoreV1().Pods(f.namespace).Get(checkCtx, f.podName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if pod.Status.Phase == corev1.PodRunning && !strings.Contains(disk.Type, expectedType) {
			return false, fmt.Errorf("Pod became runnable while disk conversion was incomplete: type=%s", disk.Type)
		}
		return strings.Contains(disk.Type, expectedType) && pod.Status.Phase == corev1.PodRunning, nil
	})
}

func (f *DiskConversionFixture) WaitForPodBlocked(ctx context.Context, operation string, timeout time.Duration) error {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return poll(waitCtx, timeout, time.Second, func() (bool, error) {
		pv, err := f.env.client.CoreV1().PersistentVolumes().Get(waitCtx, f.pvName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if pv.Annotations[diskTypeAnnotation] != operation {
			return false, fmt.Errorf("conversion operation changed before attachment was blocked: got %q, want %q", pv.Annotations[diskTypeAnnotation], operation)
		}
		disk, err := f.env.compute.Disks.Get(f.env.project, f.env.zone, f.name).Context(waitCtx).Do()
		if err != nil {
			return false, err
		}
		if strings.Contains(disk.Type, targetDiskType) {
			return false, fmt.Errorf("conversion completed before attachment blocking could be observed")
		}
		attachments, err := f.env.client.StorageV1().VolumeAttachments().List(waitCtx, metav1.ListOptions{})
		if err != nil {
			return false, err
		}
		for _, attachment := range attachments.Items {
			if attachment.Spec.Source.PersistentVolumeName != nil &&
				*attachment.Spec.Source.PersistentVolumeName == f.pvName {
				if attachment.Status.Attached {
					return false, fmt.Errorf("VolumeAttachment %s attached the disk while conversion operation %s was active", attachment.Name, operation)
				}
				if attachment.Status.AttachError == nil {
					return false, nil
				}
				message := strings.ToLower(attachment.Status.AttachError.Message)
				if !strings.Contains(message, "conversion") && !strings.Contains(message, "in progress") {
					return false, fmt.Errorf("VolumeAttachment %s failed for an unexpected reason while conversion was active: %s", attachment.Name, attachment.Status.AttachError.Message)
				}
				return true, nil
			}
		}
		return false, nil
	})
}

func (f *DiskConversionFixture) WaitForDiskType(ctx context.Context, expectedType string, timeout time.Duration) error {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return poll(waitCtx, timeout, 10*time.Second, func() (bool, error) {
		disk, err := f.env.compute.Disks.Get(f.env.project, f.env.zone, f.name).Context(waitCtx).Do()
		if err != nil {
			return false, err
		}
		return strings.Contains(disk.Type, expectedType), nil
	})
}

func (f *DiskConversionFixture) WaitForConversionComplete(ctx context.Context, expectedType string, timeout time.Duration) error {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return poll(waitCtx, timeout, 5*time.Second, func() (bool, error) {
		disk, err := f.env.compute.Disks.Get(f.env.project, f.env.zone, f.name).Context(waitCtx).Do()
		if err != nil {
			return false, err
		}
		if !strings.Contains(disk.Type, expectedType) {
			return false, nil
		}
		pv, err := f.env.client.CoreV1().PersistentVolumes().Get(waitCtx, f.pvName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		_, operationExists := pv.Annotations[diskTypeAnnotation]
		_, targetExists := pv.Annotations[diskTypeTargetAnnotation]
		return !operationExists && !targetExists, nil
	})
}

func (f *DiskConversionFixture) WaitForPVAnnotation(ctx context.Context, key, value string, timeout time.Duration) error {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return poll(waitCtx, timeout, time.Second, func() (bool, error) {
		pv, err := f.env.client.CoreV1().PersistentVolumes().Get(waitCtx, f.pvName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		return pv.Annotations[key] == value, nil
	})
}

func (f *DiskConversionFixture) WaitForConversionCancelled(ctx context.Context, timeout time.Duration) error {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return poll(waitCtx, timeout, time.Second, func() (bool, error) {
		pv, err := f.env.client.CoreV1().PersistentVolumes().Get(waitCtx, f.pvName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		for _, key := range []string{
			diskTypeAnnotation,
			diskTypeTargetAnnotation,
			"pdcsi.gke.io/disk-type-conversion-request-id",
		} {
			if _, exists := pv.Annotations[key]; exists {
				return false, nil
			}
		}
		return true, nil
	})
}

func (f *DiskConversionFixture) WaitForActiveOperation(ctx context.Context, timeout time.Duration) (string, error) {
	var operation string
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	err := poll(waitCtx, timeout, time.Second, func() (bool, error) {
		pv, err := f.env.client.CoreV1().PersistentVolumes().Get(waitCtx, f.pvName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		operation = pv.Annotations[diskTypeAnnotation]
		return operation != "" &&
			operation != conversionPending &&
			operation != conversionReconciling &&
			operation != "null", nil
	})
	if err != nil {
		return "", err
	}
	return operation, nil
}

func (f *DiskConversionFixture) VerifyConversionIntent(ctx context.Context, operation string) error {
	pv, err := f.env.client.CoreV1().PersistentVolumes().Get(ctx, f.pvName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("read PV after controller restart: %w", err)
	}
	disk, err := f.env.compute.Disks.Get(f.env.project, f.env.zone, f.name).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("read disk after controller restart: %w", err)
	}
	if !strings.Contains(disk.Type, targetDiskType) &&
		(pv.Annotations[diskTypeTargetAnnotation] != targetDiskType ||
			(pv.Annotations[diskTypeAnnotation] != operation && pv.Annotations[diskTypeAnnotation] != conversionPending)) {
		return fmt.Errorf("conversion intent was not preserved across controller restart: operation=%q (before restart %q), target=%q",
			pv.Annotations[diskTypeAnnotation], operation, pv.Annotations[diskTypeTargetAnnotation])
	}
	return nil
}

func (f *DiskConversionFixture) WaitForSnapshotRetry(ctx context.Context, timeout time.Duration) error {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return poll(waitCtx, timeout, 5*time.Second, func() (bool, error) {
		pv, err := f.env.client.CoreV1().PersistentVolumes().Get(waitCtx, f.pvName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if strings.Contains(strings.ToLower(pv.Annotations[diskTypeLastErrorAnnotation]), "snapshot") {
			return true, nil
		}
		events, err := f.env.client.CoreV1().Events(f.namespace).List(waitCtx, metav1.ListOptions{})
		if err != nil {
			return false, err
		}
		for _, event := range events.Items {
			if (event.InvolvedObject.Name == f.pvName || event.InvolvedObject.Name == f.claimName) &&
				strings.Contains(strings.ToLower(event.Message), "snapshot") {
				return true, nil
			}
		}
		return false, nil
	})
}

func (f *DiskConversionFixture) CreateInstantSnapshot(ctx context.Context) error {
	f.snapshotName = f.name + "-snapshot"
	disk, err := f.env.compute.Disks.Get(f.env.project, f.env.zone, f.name).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get source disk: %w", err)
	}
	op, err := f.env.compute.InstantSnapshots.Insert(f.env.project, f.env.zone, &compute.InstantSnapshot{
		Name:       f.snapshotName,
		SourceDisk: disk.SelfLink,
	}).Context(ctx).Do()
	if err != nil {
		return err
	}
	f.snapshotOp = op.Name
	return f.waitForSnapshotCreation(ctx)
}

func (f *DiskConversionFixture) waitForSnapshotCreation(ctx context.Context) error {
	if f.snapshotOp == "" {
		return nil
	}
	return waitZoneOperation(ctx, f.env.compute, f.env.project, f.env.zone, f.snapshotOp, 15*time.Minute)
}

func (f *DiskConversionFixture) DeleteInstantSnapshot(t DiskConversionTestT, ctx context.Context) {
	t.Helper()
	if f.snapshotName == "" {
		return
	}
	if err := f.waitForSnapshotCreation(ctx); err != nil {
		t.Fatalf("wait for instant snapshot %s creation: %v", f.snapshotName, err)
	}
	op, err := f.env.compute.InstantSnapshots.Delete(f.env.project, f.env.zone, f.snapshotName).Context(ctx).Do()
	if err != nil {
		t.Fatalf("delete instant snapshot %s: %v", f.snapshotName, err)
	}
	if err := waitZoneOperation(ctx, f.env.compute, f.env.project, f.env.zone, op.Name, 15*time.Minute); err != nil {
		t.Fatalf("wait for instant snapshot %s deletion: %v", f.snapshotName, err)
	}
	f.snapshotName = ""
	f.snapshotOp = ""
}

func (f *DiskConversionFixture) waitForDiskDetached(ctx context.Context, timeout time.Duration) error {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return poll(waitCtx, timeout, 2*time.Second, func() (bool, error) {
		disk, err := f.env.compute.Disks.Get(f.env.project, f.env.zone, f.name).Context(waitCtx).Do()
		if err != nil {
			return false, err
		}
		return len(disk.Users) == 0, nil
	})
}

func (f *DiskConversionFixture) cleanup(t DiskConversionTestT) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	pvc, err := f.env.client.CoreV1().PersistentVolumeClaims(f.namespace).Get(ctx, f.claimName, metav1.GetOptions{})
	if err == nil && pvc.Spec.VolumeAttributesClassName != nil {
		if err := updatePVC(ctx, f.env, f.namespace, f.claimName, func(pvc *corev1.PersistentVolumeClaim) {
			pvc.Spec.VolumeAttributesClassName = nil
		}); err != nil {
			t.Errorf("clear PVC %s/%s VolumeAttributesClassName during cleanup: %v", f.namespace, f.claimName, err)
		}
	} else if err != nil && !apiNotFound(err) {
		t.Errorf("read PVC %s/%s during cleanup: %v", f.namespace, f.claimName, err)
	}
	if pv, err := f.env.client.CoreV1().PersistentVolumes().Get(ctx, f.pvName, metav1.GetOptions{}); err == nil &&
		pv.Annotations[diskTypeAnnotation] == conversionPending {
		if err := f.WaitForConversionCancelled(ctx, 5*time.Minute); err != nil {
			t.Errorf("cancel queued conversion during cleanup: %v", err)
		}
	} else if err != nil && !apiNotFound(err) {
		t.Errorf("read PV %s during cleanup: %v", f.pvName, err)
	}
	if f.podName != "" {
		err := f.env.client.CoreV1().Pods(f.namespace).Delete(ctx, f.podName, metav1.DeleteOptions{})
		if err != nil && !apiNotFound(err) {
			t.Errorf("cleanup Pod %s/%s: %v", f.namespace, f.podName, err)
		}
		if err := waitForGone(ctx, func() error {
			_, err := f.env.client.CoreV1().Pods(f.namespace).Get(ctx, f.podName, metav1.GetOptions{})
			return err
		}, 5*time.Minute); err != nil {
			t.Errorf("wait for Pod %s/%s deletion during cleanup: %v", f.namespace, f.podName, err)
		}
		if err := f.waitForDiskDetached(ctx, 5*time.Minute); err != nil && !apiNotFound(err) {
			t.Errorf("wait for disk %s to detach during cleanup: %v", f.name, err)
		}
	}
	if f.snapshotName != "" {
		err := f.waitForSnapshotCreation(ctx)
		if err != nil {
			t.Errorf("wait for instant snapshot %s creation during cleanup: %v", f.snapshotName, err)
		}
		op, err := f.env.compute.InstantSnapshots.Delete(f.env.project, f.env.zone, f.snapshotName).Context(ctx).Do()
		if err == nil {
			err = waitZoneOperation(ctx, f.env.compute, f.env.project, f.env.zone, op.Name, 15*time.Minute)
		}
		if err != nil && !apiNotFound(err) {
			t.Errorf("cleanup instant snapshot %s: %v", f.snapshotName, err)
		}
	}
	if err := f.env.client.CoreV1().PersistentVolumeClaims(f.namespace).Delete(ctx, f.claimName, metav1.DeleteOptions{}); err != nil && !apiNotFound(err) {
		t.Errorf("cleanup PVC %s/%s: %v", f.namespace, f.claimName, err)
	}
	if err := waitForGone(ctx, func() error {
		_, err := f.env.client.CoreV1().PersistentVolumeClaims(f.namespace).Get(ctx, f.claimName, metav1.GetOptions{})
		return err
	}, 5*time.Minute); err != nil {
		t.Errorf("wait for PVC %s/%s deletion during cleanup: %v", f.namespace, f.claimName, err)
	}
	if err := f.env.client.CoreV1().PersistentVolumes().Delete(ctx, f.pvName, metav1.DeleteOptions{}); err != nil && !apiNotFound(err) {
		t.Errorf("cleanup PV %s: %v", f.pvName, err)
	}
	if err := waitForGone(ctx, func() error {
		_, err := f.env.client.CoreV1().PersistentVolumes().Get(ctx, f.pvName, metav1.GetOptions{})
		return err
	}, 5*time.Minute); err != nil {
		t.Errorf("wait for PV %s deletion during cleanup: %v", f.pvName, err)
	}
	for _, name := range f.vacNames {
		if err := f.env.client.StorageV1().VolumeAttributesClasses().Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !apiNotFound(err) {
			t.Errorf("cleanup VAC %s: %v", name, err)
		}
	}
	if err := deleteDisk(ctx, f.env.compute, f.env.project, f.env.zone, f.name); err != nil && !apiNotFound(err) {
		t.Errorf("cleanup disk %s: %v", f.name, err)
	}
}

func updatePVC(ctx context.Context, env *DiskConversionEnvironment, namespace, name string, mutate func(*corev1.PersistentVolumeClaim)) error {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		pvc, err := env.client.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		mutate(pvc)
		_, err = env.client.CoreV1().PersistentVolumeClaims(namespace).Update(ctx, pvc, metav1.UpdateOptions{})
		return err
	})
	if err != nil {
		return fmt.Errorf("update PVC %s/%s: %w", namespace, name, err)
	}
	return nil
}

func EnableDiskConversion(ctx context.Context, client *kubernetes.Clientset, namespace, deploymentName string) (func(context.Context) error, error) {
	deployments := client.AppsV1().Deployments(namespace)
	deployment, err := deployments.Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	originalArgs := make([]string, 0)
	containerIndex := -1
	for i := range deployment.Spec.Template.Spec.Containers {
		container := &deployment.Spec.Template.Spec.Containers[i]
		if container.Name == driverContainerName {
			containerIndex = i
			originalArgs = append(originalArgs, container.Args...)
			break
		}
	}
	if containerIndex < 0 {
		return nil, fmt.Errorf("deployment does not contain container %q", driverContainerName)
	}
	args := deployment.Spec.Template.Spec.Containers[containerIndex].Args
	originalArgs = append([]string(nil), originalArgs...)
	enabledArgs, changed := withEnabledFlag(args, "--enable-disk-type-conversion")
	if changed {
		deployment.Spec.Template.Spec.Containers[containerIndex].Args = enabledArgs
	}
	if changed {
		if _, err := deployments.Update(ctx, deployment, metav1.UpdateOptions{}); err != nil {
			return nil, err
		}
	}
	restore := func(restoreCtx context.Context) error {
		if !changed {
			return nil
		}
		current, err := deployments.Get(restoreCtx, deploymentName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		for i := range current.Spec.Template.Spec.Containers {
			if current.Spec.Template.Spec.Containers[i].Name == driverContainerName {
				current.Spec.Template.Spec.Containers[i].Args = originalArgs
				break
			}
		}
		if _, err := deployments.Update(restoreCtx, current, metav1.UpdateOptions{}); err != nil {
			return err
		}
		return waitDeploymentReady(restoreCtx, client, namespace, deploymentName, 15*time.Minute)
	}
	if err := waitDeploymentReady(ctx, client, namespace, deploymentName, 15*time.Minute); err != nil {
		if restoreErr := restore(ctx); restoreErr != nil {
			return nil, fmt.Errorf("controller failed to become ready with conversion enabled: %v; restoring its original arguments also failed: %w", err, restoreErr)
		}
		return nil, err
	}
	return restore, nil
}

func RestartDiskConversionController(ctx context.Context, env *DiskConversionEnvironment) error {
	return restartController(ctx, env.client, env.namespace, env.deploymentName)
}

func restartController(ctx context.Context, client *kubernetes.Clientset, namespace, deploymentName string) error {
	pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, pod := range pods.Items {
		for _, owner := range pod.OwnerReferences {
			if owner.Kind != "ReplicaSet" {
				continue
			}
			replicaSet, err := client.AppsV1().ReplicaSets(namespace).Get(ctx, owner.Name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			for _, replicaSetOwner := range replicaSet.OwnerReferences {
				if replicaSetOwner.Kind == "Deployment" && replicaSetOwner.Name == deploymentName {
					if err := client.CoreV1().Pods(namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{}); err != nil {
						return fmt.Errorf("delete controller pod %s: %w", pod.Name, err)
					}
					if err := poll(ctx, 10*time.Minute, time.Second, func() (bool, error) {
						_, err := client.CoreV1().Pods(namespace).Get(ctx, pod.Name, metav1.GetOptions{})
						if apiNotFound(err) {
							return true, nil
						}
						return false, err
					}); err != nil {
						return fmt.Errorf("wait for controller pod %s to terminate: %w", pod.Name, err)
					}
					return waitDeploymentReady(ctx, client, namespace, deploymentName, 15*time.Minute)
				}
			}
		}
	}
	return fmt.Errorf("no pod owned by deployment %s/%s found", namespace, deploymentName)
}

func waitDeploymentReady(ctx context.Context, client *kubernetes.Clientset, namespace, deploymentName string, timeout time.Duration) error {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return poll(waitCtx, timeout, 2*time.Second, func() (bool, error) {
		deployment, err := client.AppsV1().Deployments(namespace).Get(waitCtx, deploymentName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		desired := int32(1)
		if deployment.Spec.Replicas != nil {
			desired = *deployment.Spec.Replicas
		}
		if desired < 1 {
			return false, fmt.Errorf("controller deployment %s/%s is scaled to zero", namespace, deploymentName)
		}
		return deployment.Status.ObservedGeneration >= deployment.Generation &&
			deployment.Status.UpdatedReplicas >= desired &&
			deployment.Status.ReadyReplicas >= desired, nil
	})
}

func hyperdiskCompatibleNode(ctx context.Context, client *kubernetes.Clientset) (*corev1.Node, error) {
	nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list Kubernetes nodes: %w", err)
	}
	for i := range nodes.Items {
		node := &nodes.Items[i]
		if node.Spec.Unschedulable || node.Labels[corev1.LabelTopologyZone] == "" || !nodeReady(node) {
			continue
		}
		machineType := node.Labels["node.kubernetes.io/instance-type"]
		for _, prefix := range []string{"c3-", "c3d-", "c4-", "c4a-", "m3-", "n4-", "z3-", "x4-"} {
			if strings.HasPrefix(machineType, prefix) {
				return node, nil
			}
		}
	}
	return nil, fmt.Errorf("no schedulable Hyperdisk-compatible node found; provide a node with a node.kubernetes.io/instance-type label in a supported family (C3/C4/M3/N4/Z3/X4)")
}

func waitZoneOperation(ctx context.Context, client *compute.Service, project, zone, operationName string, timeout time.Duration) error {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return poll(waitCtx, timeout, 2*time.Second, func() (bool, error) {
		op, err := client.ZoneOperations.Get(project, zone, operationName).Context(waitCtx).Do()
		if err != nil {
			return false, err
		}
		if op.Status != "DONE" {
			return false, nil
		}
		if op.Error != nil {
			return false, fmt.Errorf("Compute Engine operation %s failed: %v", operationName, op.Error.Errors)
		}
		return true, nil
	})
}

func deleteDisk(ctx context.Context, client *compute.Service, project, zone, name string) error {
	waitCtx, cancel := context.WithTimeout(ctx, 2*time.Hour)
	defer cancel()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	var lastErr error
	for {
		op, err := client.Disks.Delete(project, zone, name).Context(waitCtx).Do()
		if apiNotFound(err) {
			return nil
		}
		if err == nil {
			if err := waitZoneOperation(waitCtx, client, project, zone, op.Name, time.Hour); err == nil {
				return nil
			} else {
				lastErr = err
			}
		} else {
			lastErr = err
		}
		select {
		case <-waitCtx.Done():
			if lastErr != nil {
				return fmt.Errorf("delete disk %s: %w (context: %v)", name, lastErr, waitCtx.Err())
			}
			return waitCtx.Err()
		case <-ticker.C:
		}
	}
}

func poll(ctx context.Context, timeout, interval time.Duration, condition func() (bool, error)) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		done, err := condition()
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("timed out after %s", timeout)
		case <-ticker.C:
		}
	}
}

func waitForGone(ctx context.Context, get func() error, timeout time.Duration) error {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return poll(waitCtx, timeout, 2*time.Second, func() (bool, error) {
		err := get()
		if apiNotFound(err) {
			return true, nil
		}
		return false, err
	})
}

func withEnabledFlag(args []string, flag string) ([]string, bool) {
	for i, arg := range args {
		if arg == flag || arg == flag+"=true" {
			return args, false
		}
		if strings.HasPrefix(arg, flag+"=") {
			updated := append([]string(nil), args...)
			updated[i] = flag + "=true"
			return updated, true
		}
	}
	return append(append([]string(nil), args...), flag+"=true"), true
}

func apiNotFound(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "not found") || strings.Contains(message, "notfound")
}

func nodeReady(node *corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
