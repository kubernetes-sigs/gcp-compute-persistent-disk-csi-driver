package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	storagev1alpha1 "k8s.io/api/storage/v1alpha1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	k8sv1alpha1 "k8s.io/client-go/kubernetes/typed/storage/v1alpha1"
	"k8s.io/client-go/tools/clientcmd"

	computev1 "cloud.google.com/go/compute/apiv1"
	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"google.golang.org/api/option"
	// "k8s.io/client-go/util/retry"
	// "k8s.io/klog/v2"
)

const (
	defaultNamespace   = "default"
	driverName         = "pd.csi.storage.gke.io"
	driverNamespace    = "gce-pd-csi-driver"
	storageClassPrefix = "test-storageclass-"
	pvcPrefix          = "test-pvc-"
	vac1Prefix         = "test-vac1-"
	vac2Prefix         = "test-vac2-"
	podPrefix          = "test-pod-"

	disksRequest   = "https://compute.googleapis.com/compute/v1/projects/%s/aggregated/disks"
	projectRequest = "https://metadata.google.internal/computeMetadata/v1/project/project-id"
	zoneRequest    = "https://metadata.google.internal/computeMetadata/v1/instance/zone"
)

// Each test begins by creating a PV with dynamic provisioning. This contains all necessary parameters
type TestMetadata struct {
	DiskType          string
	InitialIops       *string
	InitialThroughput *string
	InitialSize       string
	StorageClassName  string
	VacName1          string
	VacName2          string
	PvcName           string
	PodName           string
}

type DiskInfo struct {
	projectName string
	pvName      string
	zone        string
}

// Disk, DiskList, and AggregatedList structs are used for unmarshalling
type Disk struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Zone string `json:"zone"`
}

type DiskList struct {
	Disks []Disk `json:"disks"`
}

type AggregatedList struct {
	Items map[string]DiskList `json:"items"`
}

func TestControllerModifyVolume(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Run ControllerModifyVolume tests")
}

var _ = Describe("ControllerModifyVolume tests", func() {
	var (
		projectName    string
		credsPath      string
		kubeConfigPath string

		ctx               context.Context
		clientset         *kubernetes.Clientset
		credentialsOption option.ClientOption
		computeClient     *computev1.DisksClient
		storageClient     *k8sv1alpha1.StorageV1alpha1Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		kubeConfigPath, err = getKubeConfig()
		Expect(err).To(BeNil())

		projectName = os.Getenv("PROJECT")
		Expect(projectName).ToNot(Equal(""))

		credsPath = os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
		Expect(credsPath).ToNot(Equal(""))

		// Setup clients
		config, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
		Expect(err).To(BeNil())
		clientset, err = kubernetes.NewForConfig(config)
		Expect(err).To(BeNil())

		storageClient, err = k8sv1alpha1.NewForConfig(config)
		Expect(err).To(BeNil())

		credentialsOption = option.WithCredentialsFile(credsPath)
		computeClient, err = computev1.NewDisksRESTClient(ctx, credentialsOption)
		Expect(err).To(BeNil())

	})

	AfterEach(func() {
		computeClient.Close()
	})

	Context("Updates to hyperdisks", func() {
		It("HdB should pass with normal constraints", func() {
			initialIops := "3000"
			initialThroughput := "150"
			testMetadata := createTestMetadata("hyperdisk-balanced", "64Gi", &initialIops, &initialThroughput)

			pvName, diskInfo, err := createPVWithDynamicProvisioning(clientset, computeClient, storageClient, projectName, testMetadata, ctx)
			defer cleanupResources(clientset, storageClient, testMetadata, ctx)
			Expect(err).To(BeNil())

			updatedIops := "3013"
			updatedThroughput := "181"
			err = createVac(storageClient, testMetadata.VacName2, &updatedIops, &updatedThroughput, ctx)
			defer cleanupVac(storageClient, testMetadata.VacName2, ctx)
			Expect(err).To(BeNil())

			err = patchPvc(clientset, testMetadata.PvcName, testMetadata.VacName2, ctx)
			Expect(err).To(BeNil())

			err = waitUntilUpdate(computeClient, 11, diskInfo, testMetadata.InitialIops, testMetadata.InitialThroughput, ctx)
			Expect(err).To(BeNil())

			currentVacName, err := getVacFromPV(clientset, pvName, ctx)
			Expect(err).To(BeNil())
			Expect(currentVacName).To(Equal(testMetadata.VacName2))

			iops, throughput, err := getMetadataFromPV(computeClient, diskInfo, true, true, ctx)
			Expect(strconv.FormatInt(iops, 10)).To(Equal(updatedIops))
			Expect(strconv.FormatInt(throughput, 10)).To(Equal(updatedThroughput))
		})

		It("HdB with invalid update parameters doesn't update PV", func() {
			initialIops := "3000"
			initialThroughput := "150"
			testMetadata := createTestMetadata("hyperdisk-balanced", "64Gi", &initialIops, &initialThroughput)

			pvName, _, err := createPVWithDynamicProvisioning(clientset, computeClient, storageClient, projectName, testMetadata, ctx)
			defer cleanupResources(clientset, storageClient, testMetadata, ctx)
			Expect(err).To(BeNil())

			updatedIops := "120000"
			updatedThroughput := "150"
			err = createVac(storageClient, testMetadata.VacName2, &updatedIops, &updatedThroughput, ctx)

			err = patchPvc(clientset, testMetadata.PvcName, testMetadata.VacName2, ctx)
			Expect(err).To(BeNil())

			err = waitForVacUpdate(clientset, pvName, testMetadata.VacName1, ctx)
			Expect(err).ToNot(BeNil())

			currentVacName, err := getVacFromPV(clientset, pvName, ctx)
			Expect(err).To(BeNil())
			Expect(currentVacName).To(Equal(testMetadata.VacName1))
		})

		It("HdT should pass with normal constraints", func() {
			initialThroughput := "30"
			testMetadata := createTestMetadata("hyperdisk-throughput", "2048Gi", nil, &initialThroughput)

			pvName, diskInfo, err := createPVWithDynamicProvisioning(clientset, computeClient, storageClient, projectName, testMetadata, ctx)
			defer cleanupResources(clientset, storageClient, testMetadata, ctx)
			Expect(err).To(BeNil())

			updatedThroughput := "40"
			err = createVac(storageClient, testMetadata.VacName2, nil, &updatedThroughput, ctx)
			defer cleanupVac(storageClient, testMetadata.VacName2, ctx)
			Expect(err).To(BeNil())

			err = patchPvc(clientset, testMetadata.PvcName, testMetadata.VacName2, ctx)
			Expect(err).To(BeNil())

			err = waitUntilUpdate(computeClient, 11, diskInfo, testMetadata.InitialIops, testMetadata.InitialThroughput, ctx)
			Expect(err).To(BeNil())

			currentVacName, err := getVacFromPV(clientset, pvName, ctx)
			Expect(err).To(BeNil())
			Expect(currentVacName).To(Equal(testMetadata.VacName2))

			_, throughput, err := getMetadataFromPV(computeClient, diskInfo, false, true, ctx)
			Expect(strconv.FormatInt(throughput, 10)).To(Equal(updatedThroughput))
		})

		It("HdT should fail when providing IOPS in VAC", func() {
			initialThroughput := "30"
			testMetadata := createTestMetadata("hyperdisk-throughput", "2048Gi", nil, &initialThroughput)

			_, _, err := createPVWithDynamicProvisioning(clientset, computeClient, storageClient, projectName, testMetadata, ctx)
			defer cleanupResources(clientset, storageClient, testMetadata, ctx)
			Expect(err).To(BeNil())

			provisionedIops := "130"
			updatedThroughput := "40"
			err = createVac(storageClient, testMetadata.VacName2, &provisionedIops, &updatedThroughput, ctx)
			defer cleanupVac(storageClient, testMetadata.VacName2, ctx)
			Expect(err).To(BeNil())

			err = patchPvc(clientset, testMetadata.PvcName, testMetadata.VacName2, ctx)
			Expect(err).To(BeNil())

			errExists, err := checkForError(clientset, "Cannot specify IOPS for disk type hyperdisk-throughput", ctx)
			Expect(err).To(BeNil())
			Expect(errExists).To(Equal(true))
		})

		It("HdT with invalid update parameters doesn't update PV", func() {
			initialThroughput := "30"
			testMetadata := createTestMetadata("hyperdisk-throughput", "2048Gi", nil, &initialThroughput)

			pvName, _, err := createPVWithDynamicProvisioning(clientset, computeClient, storageClient, projectName, testMetadata, ctx)
			defer cleanupResources(clientset, storageClient, testMetadata, ctx)
			Expect(err).To(BeNil())

			err = patchPvc(clientset, testMetadata.PvcName, testMetadata.VacName2, ctx)
			Expect(err).To(BeNil())

			err = waitForVacUpdate(clientset, pvName, testMetadata.VacName1, ctx)
			Expect(err).ToNot(BeNil())

			currentVacName, err := getVacFromPV(clientset, pvName, ctx)
			Expect(err).To(BeNil())
			Expect(currentVacName).To(Equal(testMetadata.VacName1))
		})

		It("HdX should pass with normal constraints", func() {
			initialIops := "130"
			testMetadata := createTestMetadata("hyperdisk-extreme", "64Gi", &initialIops, nil)

			pvName, diskInfo, err := createPVWithDynamicProvisioning(clientset, computeClient, storageClient, projectName, testMetadata, ctx)
			defer cleanupResources(clientset, storageClient, testMetadata, ctx)
			Expect(err).To(BeNil())

			updatedIops := "140"
			err = createVac(storageClient, testMetadata.VacName2, &updatedIops, nil, ctx)
			defer cleanupVac(storageClient, testMetadata.VacName2, ctx)
			Expect(err).To(BeNil())

			err = patchPvc(clientset, testMetadata.PvcName, testMetadata.VacName2, ctx)
			Expect(err).To(BeNil())

			err = waitUntilUpdate(computeClient, 11, diskInfo, testMetadata.InitialIops, nil, ctx)
			Expect(err).To(BeNil())

			currentVacName, err := getVacFromPV(clientset, pvName, ctx)
			Expect(err).To(BeNil())
			Expect(currentVacName).To(Equal(testMetadata.VacName2))

			iops, _, err := getMetadataFromPV(computeClient, diskInfo, true, false /* getThroughput */, ctx)
			Expect(strconv.FormatInt(iops, 10)).To(Equal(updatedIops))
		})

		It("HdX should fail when providing throughput in VAC", func() {
			initialIops := "130"
			testMetadata := createTestMetadata("hyperdisk-extreme", "64Gi", &initialIops, nil)

			_, _, err := createPVWithDynamicProvisioning(clientset, computeClient, storageClient, projectName, testMetadata, ctx)
			defer cleanupResources(clientset, storageClient, testMetadata, ctx)
			Expect(err).To(BeNil())

			updatedIops := "150"
			provisionedThroughput := "40"
			err = createVac(storageClient, testMetadata.VacName2, &updatedIops, &provisionedThroughput, ctx)
			defer cleanupVac(storageClient, testMetadata.VacName2, ctx)
			Expect(err).To(BeNil())

			err = patchPvc(clientset, testMetadata.PvcName, testMetadata.VacName2, ctx)
			Expect(err).To(BeNil())

			errExists, err := checkForError(clientset, "Cannot specify throughput for disk type hyperdisk-extreme", ctx)
			Expect(err).To(BeNil())
			Expect(errExists).To(Equal(true))
		})

		It("HdX with invalid update parameters doesn't update PV", func() {
			initialIops := "130"
			testMetadata := createTestMetadata("hyperdisk-extreme", "64Gi", &initialIops, nil)

			pvName, _, err := createPVWithDynamicProvisioning(clientset, computeClient, storageClient, projectName, testMetadata, ctx)
			// defer cleanupResources(clientset, storageClient, testMetadata, ctx)

			err = patchPvc(clientset, testMetadata.PvcName, testMetadata.VacName2, ctx)
			Expect(err).To(BeNil())

			err = waitForVacUpdate(clientset, pvName, testMetadata.VacName1, ctx)
			Expect(err).ToNot(BeNil())

			currentVacName, err := getVacFromPV(clientset, pvName, ctx)
			Expect(err).To(BeNil())
			Expect(currentVacName).To(Equal(testMetadata.VacName1))
		})
	})
})

func createTestMetadata(diskType string, initialSize string, initialIops *string, initialThroughput *string) TestMetadata {
	suffix := strconv.FormatInt(time.Now().UnixMicro(), 10)
	fmt.Printf("The suffixes for the resources is %s\n", suffix)
	testMetadata := TestMetadata{
		DiskType:          diskType,
		InitialSize:       initialSize,
		InitialIops:       initialIops,
		InitialThroughput: initialThroughput,
		StorageClassName:  storageClassPrefix + suffix,
		VacName1:          vac1Prefix + suffix,
		VacName2:          vac2Prefix + suffix,
		PvcName:           pvcPrefix + suffix,
		PodName:           podPrefix + suffix,
	}
	return testMetadata
}

func createPVWithDynamicProvisioning(
	clientset *kubernetes.Clientset,
	computeClient *computev1.DisksClient,
	storageClient *k8sv1alpha1.StorageV1alpha1Client,
	projectName string,
	testMetadata TestMetadata,
	ctx context.Context,
) (string, DiskInfo, error) {
	err := createStorageClass(clientset, testMetadata.StorageClassName, testMetadata.DiskType, testMetadata.InitialIops, testMetadata.InitialThroughput, ctx)
	if err != nil {
		return "", DiskInfo{}, err
	}

	err = createVac(storageClient, testMetadata.VacName1, testMetadata.InitialIops, testMetadata.InitialThroughput, ctx)
	if err != nil {
		return "", DiskInfo{}, err
	}

	err = createPvc(clientset, testMetadata.PvcName, testMetadata.InitialSize, testMetadata.StorageClassName, testMetadata.VacName1, ctx)
	if err != nil {
		return "", DiskInfo{}, err
	}

	err = createPod(clientset, testMetadata.PodName, testMetadata.PvcName, ctx)
	if err != nil {
		return "", DiskInfo{}, err
	}

	pvName, zoneName, err := getPVNameAndZone(clientset, computeClient, projectName, defaultNamespace, testMetadata.PvcName, 60, ctx)
	if err != nil {
		return "", DiskInfo{}, err
	}

	diskInfo := DiskInfo{
		pvName:      pvName,
		projectName: projectName,
		zone:        zoneName,
	}
	iops, throughput, err := getMetadataFromPV(computeClient, diskInfo, testMetadata.InitialIops != nil, testMetadata.InitialThroughput != nil, ctx)
	if err != nil {
		return "", DiskInfo{}, err
	}

	if testMetadata.InitialIops != nil && strconv.FormatInt(iops, 10) != *testMetadata.InitialIops {
		return "", DiskInfo{}, fmt.Errorf("Error: PV IOPS is %s, want = %s", strconv.FormatInt(iops, 10), *testMetadata.InitialIops)
	}

	if testMetadata.InitialThroughput != nil && strconv.FormatInt(throughput, 10) != *testMetadata.InitialThroughput {
		return "", DiskInfo{}, fmt.Errorf("Error: PV throughput is %s, want = %s", strconv.FormatInt(throughput, 10), *testMetadata.InitialThroughput)
	}
	return pvName, diskInfo, nil
}

func cleanupResources(clientset *kubernetes.Clientset, storageClient *k8sv1alpha1.StorageV1alpha1Client, testMetadata TestMetadata, ctx context.Context) {
	cleanupStorageClass(clientset, testMetadata.StorageClassName, ctx)
	cleanupVac(storageClient, testMetadata.VacName1, ctx)
	cleanupPvc(clientset, testMetadata.PvcName, ctx)
	cleanupPod(clientset, testMetadata.PodName, ctx)
}

func createStorageClass(clientset *kubernetes.Clientset, storageClassName string, diskType string, iops *string, throughput *string, ctx context.Context) error {
	waitForFirstConsumer := storagev1.VolumeBindingWaitForFirstConsumer
	parameters := map[string]string{
		"type": diskType,
	}
	if iops != nil {
		parameters["provisioned-iops-on-create"] = *iops
	}
	if throughput != nil {
		parameters["provisioned-throughput-on-create"] = *throughput + "Mi"
	}
	storageClass := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: storageClassName,
		},
		Provisioner:       driverName,
		Parameters:        parameters,
		VolumeBindingMode: &waitForFirstConsumer,
	}
	_, err := clientset.StorageV1().StorageClasses().Create(ctx, storageClass, metav1.CreateOptions{})
	return err
}

func cleanupStorageClass(clientset *kubernetes.Clientset, storageClassName string, ctx context.Context) {
	err := clientset.StorageV1().StorageClasses().Delete(ctx, storageClassName, metav1.DeleteOptions{})
	if err != nil {
		fmt.Printf("Deleting storage class %s failed with error: %v\n", storageClassName, err)
	}
}

func createVac(storageClient *k8sv1alpha1.StorageV1alpha1Client, vacName string, iops *string, throughput *string, ctx context.Context) error {
	parameters := map[string]string{}
	if iops != nil {
		parameters["iops"] = *iops
	}
	if throughput != nil {
		parameters["throughput"] = *throughput
	}
	vac1 := &storagev1alpha1.VolumeAttributesClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: vacName,
		},
		DriverName: driverName,
		Parameters: parameters,
	}
	_, err := storageClient.VolumeAttributesClasses().Create(ctx, vac1, metav1.CreateOptions{})
	return err
}

func cleanupVac(storageClient *k8sv1alpha1.StorageV1alpha1Client, vacName string, ctx context.Context) {
	err := storageClient.VolumeAttributesClasses().Delete(ctx, vacName, metav1.DeleteOptions{})
	if err != nil {
		fmt.Printf("Deleting VolumeAttributesClass %s failed with error: %v\n", vacName, err)
	}
}

func createPvc(clientset *kubernetes.Clientset, pvcName string, size string, storageClassName string, vacName string, ctx context.Context) error {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvcName,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(size),
				},
			},
			StorageClassName:          &storageClassName,
			VolumeAttributesClassName: &vacName,
		},
	}
	_, err := clientset.CoreV1().PersistentVolumeClaims(defaultNamespace).Create(ctx, pvc, metav1.CreateOptions{})
	return err
}

func cleanupPvc(clientset *kubernetes.Clientset, pvcName string, ctx context.Context) {
	err := clientset.CoreV1().PersistentVolumeClaims(defaultNamespace).Delete(ctx, pvcName, metav1.DeleteOptions{})
	if err != nil {
		fmt.Printf("Deleting PVC %s failed with error: %v\n", pvcName, err)
	}
}

func createPod(clientset *kubernetes.Clientset, podName string, pvcName string, ctx context.Context) error {
	pvcVolumeSource := corev1.PersistentVolumeClaimVolumeSource{
		ClaimName: pvcName,
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: podName,
		},
		Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{
				{
					Name: "test-vol",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &pvcVolumeSource,
					},
				},
			},
			Containers: []corev1.Container{
				{
					Name:  "test-container",
					Image: "nginx:1.14.2",
					Ports: []corev1.ContainerPort{
						{
							ContainerPort: 80,
						},
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "test-vol",
							MountPath: "/vol",
						},
					},
				},
			},
		},
	}
	_, err := clientset.CoreV1().Pods(defaultNamespace).Create(ctx, pod, metav1.CreateOptions{})
	return err
}

func cleanupPod(clientset *kubernetes.Clientset, podName string, ctx context.Context) {
	err := clientset.CoreV1().Pods(defaultNamespace).Delete(ctx, podName, metav1.DeleteOptions{})
	if err != nil {
		fmt.Printf("Deleting PVC %s failed with error: %v\n", podName, err)
	}
}

func getPVNameAndZone(clientset *kubernetes.Clientset, computeClient *computev1.DisksClient, projectName string, nsName string, pvcName string, timeout int64, ctx context.Context) (string, string, error) {
	pvName, zones, err := getPVNameAndZones(clientset, nsName, pvcName, timeout)
	if err != nil {
		return "", "", err
	}
	fmt.Printf("The pvName is %s and the projectName is %s\n", pvName, projectName)
	getDiskRequest := &computepb.GetDiskRequest{
		Disk:    pvName,
		Project: projectName,
	}
	// zones represents all possible zones the pv is in, iterate to see which zone the PV is in
	for _, zone := range zones {
		getDiskRequest.Zone = zone
		pv, err := computeClient.Get(ctx, getDiskRequest)
		if err == nil && pv != nil {
			return pvName, zone, nil
		}
	}
	return "", "", fmt.Errorf("Could not find the zone for the PV!")
}

// getPVNameAndZones returns the PV name and possible zones (based off the node affinity) corresponding to the pvcName in namespace nsName.
func getPVNameAndZones(clientset *kubernetes.Clientset, nsName string, pvcName string, timeout int64) (string, []string, error) {
	pvName := ""
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()
	pvcErr := wait.PollUntilContextCancel(ctx, 5*time.Second, false, func(ctx context.Context) (bool, error) {
		pvc, err := clientset.CoreV1().PersistentVolumeClaims(nsName).Get(ctx, pvcName, metav1.GetOptions{})
		if err != nil {
			// We want to retry until the code returns a success or times out, so return nil for error to allow retrying
			fmt.Printf("The error is: %v\n", err)
			return false, nil
		}
		if pvc.Status.Phase != corev1.ClaimBound {
			return false, nil
		}
		if pvc.Spec.VolumeName == "" {
			return false, nil
		}
		pvName = pvc.Spec.VolumeName
		return true, nil
	})
	if pvcErr != nil {
		return "", nil, fmt.Errorf("Could not get the PV name for the PVC %s.", pvcName)
	}
	var zoneNames []string
	pvErr := wait.PollUntilContextCancel(ctx, 5*time.Second, true, func(ctx context.Context) (bool, error) {
		pv, err := clientset.CoreV1().PersistentVolumes().Get(ctx, pvName, metav1.GetOptions{})
		if err != nil {
			return false, nil
		}
		if pv.Spec.NodeAffinity != nil && pv.Spec.NodeAffinity.Required != nil && pv.Spec.NodeAffinity.Required.NodeSelectorTerms != nil {
			if len(pv.Spec.NodeAffinity.Required.NodeSelectorTerms) > 0 {
				for _, nodeSelectorTerm := range pv.Spec.NodeAffinity.Required.NodeSelectorTerms {
					for _, nodeSelectorRequirement := range nodeSelectorTerm.MatchExpressions {
						if nodeSelectorRequirement.Key == "topology.gke.io/zone" {
							if len(nodeSelectorRequirement.Values) > 0 {
								zoneNames = nodeSelectorRequirement.Values
								return true, nil
							}
						}
					}
				}
			}
		}
		return false, nil
	})
	if pvErr != nil {
		return "", nil, pvErr
	}
	return pvName, zoneNames, nil
}

func getMetadataFromPV(computeClient *computev1.DisksClient, diskInfo DiskInfo, getIops bool, getThroughput bool, ctx context.Context) (int64, int64, error) {
	getDiskRequest := &computepb.GetDiskRequest{
		Disk:    diskInfo.pvName,
		Project: diskInfo.projectName,
		Zone:    diskInfo.zone,
	}
	pv, err := computeClient.Get(ctx, getDiskRequest)
	var iops int64
	var throughput int64
	if err != nil {
		return iops, throughput, err
	}
	if getIops {
		iops = *pv.ProvisionedIops
	}
	if getThroughput {
		throughput = *pv.ProvisionedThroughput
	}
	return iops, throughput, nil
}

func getVacFromPV(clientset *kubernetes.Clientset, pvName string, ctx context.Context) (string, error) {
	pv, err := clientset.CoreV1().PersistentVolumes().Get(ctx, pvName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	vacName := pv.Spec.VolumeAttributesClassName
	if vacName == nil || *vacName == "" {
		return "", fmt.Errorf("Could not get the VolumeAttributesClassName.")
	}
	return *vacName, nil
}

func patchPvc(clientset *kubernetes.Clientset, pvcName string, vacName string, ctx context.Context) error {
	patch := []map[string]interface{}{
		{
			"op":    "replace",
			"path":  "/spec/volumeAttributesClassName",
			"value": vacName,
		},
	}
	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return err
	}
	_, err = clientset.CoreV1().PersistentVolumeClaims(defaultNamespace).Patch(ctx, pvcName, types.JSONPatchType, patchBytes, metav1.PatchOptions{})
	return err
}

func waitUntilUpdate(computeClient *computev1.DisksClient, numMinutes int, diskInfo DiskInfo, initialIops *string, initialThroughput *string, ctx context.Context) error {
	backoff := wait.Backoff{
		Duration: 1 * time.Minute,
		Factor:   1.0,
		Steps:    numMinutes,
		Cap:      time.Duration(numMinutes) * time.Minute,
	}
	err := wait.ExponentialBackoffWithContext(ctx, backoff, func(ctx context.Context) (bool, error) {
		iops, throughput, err := getMetadataFromPV(computeClient, diskInfo, initialIops != nil, initialThroughput != nil, ctx)
		if err != nil {
			return false, err
		}
		if initialIops != nil && *initialIops != strconv.FormatInt(iops, 10) {
			return true, nil
		}
		if initialThroughput != nil && *initialThroughput != strconv.FormatInt(throughput, 10) {
			return true, nil
		}
		return false, nil
	})
	return err
}

func waitForVacUpdate(clientset *kubernetes.Clientset, pvName string, vacName string, ctx context.Context) error {
	contextTimeout, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	err := wait.PollUntilContextCancel(contextTimeout, 10*time.Second, true, func(ctx context.Context) (bool, error) {
		pvVacName, err := getVacFromPV(clientset, pvName, ctx)
		if err != nil {
			return false, nil
		}
		if pvVacName != "" && pvVacName != vacName {
			return true, nil
		}
		return false, nil
	})
	return err
}

func checkForError(clientset *kubernetes.Clientset, message string, ctx context.Context) (bool, error) {
	podList, err := clientset.CoreV1().Pods(driverNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, err
	}
	podLogOptions := &corev1.PodLogOptions{
		Container: "gce-pd-driver",
	}
	for _, pod := range podList.Items {
		logRequest := clientset.CoreV1().Pods(driverNamespace).GetLogs(pod.Name, podLogOptions)
		logs, err := logRequest.Stream(ctx)
		if err != nil {
			continue
		}
		messages, err := io.ReadAll(logs)
		fullLogs := string(messages[:])
		if strings.Contains(fullLogs, message) {
			return true, nil
		}
	}
	return false, nil
}
