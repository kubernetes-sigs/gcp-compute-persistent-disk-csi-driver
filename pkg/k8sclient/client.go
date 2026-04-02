package k8sclient

import (
	"context"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

var (
	backoff = wait.Backoff{
		Duration: 1 * time.Second,
		Factor:   2.0,
		Steps:    5,
	}

	// For testing purposes, this function can be overridden to return a fake client.
	GetClient = func() (kubernetes.Interface, error) {
		cfg, err := rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
		return kubernetes.NewForConfig(cfg)
	}
)

func GetNodeWithRetry(ctx context.Context, nodeName string) (*v1.Node, error) {
	if nodeName == "" {
		return nil, fmt.Errorf("node name is empty")
	}
	kubeClient, err := GetClient()
	if err != nil {
		return nil, err
	}
	return getNodeWithRetry(ctx, kubeClient, nodeName)
}

func GetStorageClassWithRetry(ctx context.Context, scName string) (*storagev1.StorageClass, error) {
	kubeClient, err := GetClient()
	if err != nil {
		return nil, err
	}
	return getStorageClassWithRetry(ctx, kubeClient, scName)
}

func GetPersistentVolumeWithRetry(ctx context.Context, pvName string) (*v1.PersistentVolume, error) {
	kubeClient, err := GetClient()
	if err != nil {
		return nil, err
	}
	return getPersistentVolumeWithRetry(ctx, kubeClient, pvName)
}

func ListPodsInNamespace(ctx context.Context, namespace string) (*v1.PodList, error) {
	kubeClient, err := GetClient()
	if err != nil {
		return nil, err
	}
	return listPodsInNamespace(ctx, kubeClient, namespace)
}

func getNodeWithRetry(ctx context.Context, kubeClient kubernetes.Interface, nodeName string) (*v1.Node, error) {
	var nodeObj *v1.Node
	err := wait.ExponentialBackoffWithContext(ctx, backoff, func(_ context.Context) (bool, error) {
		node, err := kubeClient.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
		if err != nil {
			klog.Warningf("Error getting node %s: %v, retrying...\n", nodeName, err)
			return false, nil
		}
		nodeObj = node
		klog.V(4).Infof("Successfully retrieved node info %s\n", nodeName)
		return true, nil
	})

	if err != nil {
		klog.Errorf("Failed to get node %s after retries: %v\n", nodeName, err)
	}
	return nodeObj, err
}

func getStorageClassWithRetry(ctx context.Context, kubeClient kubernetes.Interface, scName string) (*storagev1.StorageClass, error) {
	var scObj *storagev1.StorageClass
	err := wait.ExponentialBackoffWithContext(ctx, backoff, func(_ context.Context) (bool, error) {
		sc, err := kubeClient.StorageV1().StorageClasses().Get(ctx, scName, metav1.GetOptions{})
		if err != nil {
			klog.Warningf("Error getting StorageClass %s: %v, retrying...\n", scName, err)
			return false, nil
		}
		scObj = sc
		klog.V(4).Infof("Successfully retrieved StorageClass info %s\n", scName)
		return true, nil
	})

	if err != nil {
		klog.Errorf("Failed to get StorageClass %s after retries: %v\n", scName, err)
	}
	return scObj, err
}

func getPersistentVolumeWithRetry(ctx context.Context, kubeClient kubernetes.Interface, pvName string) (*v1.PersistentVolume, error) {
	var pvObj *v1.PersistentVolume
	err := wait.ExponentialBackoffWithContext(ctx, backoff, func(_ context.Context) (bool, error) {
		pv, err := kubeClient.CoreV1().PersistentVolumes().Get(ctx, pvName, metav1.GetOptions{})
		if err != nil {
			klog.Warningf("Error getting PersistentVolume %s: %v, retrying...\n", pvName, err)
			return false, nil
		}
		pvObj = pv
		klog.V(4).Infof("Successfully retrieved PersistentVolume info %s\n", pvName)
		return true, nil
	})

	if err != nil {
		klog.Errorf("Failed to get PersistentVolume %s after retries: %v\n", pvName, err)
	}
	return pvObj, err
}

func listPodsInNamespace(ctx context.Context, kubeClient kubernetes.Interface, namespace string) (*v1.PodList, error) {
	var podList *v1.PodList
	err := wait.ExponentialBackoffWithContext(ctx, backoff, func(_ context.Context) (bool, error) {
		pods, err := kubeClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			klog.Warningf("Error listing pods in namespace %s: %v, retrying...\n", namespace, err)
			return false, nil
		}
		podList = pods
		klog.V(4).Infof("Successfully listed pods in namespace %s\n", namespace)
		return true, nil
	})

	if err != nil {
		klog.Errorf("Failed to list pods in namespace %s after retries: %v\n", namespace, err)
	}
	return podList, err
}
