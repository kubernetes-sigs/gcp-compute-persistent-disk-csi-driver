package tenancy

import (
	"flag"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

var (
	kubeConfigFile = flag.String("mt-kubeconfig", "", "Path to kubeconfig file with authorization and master location information.")
)

// GetKubeConfig returns kuberentes configuration based on flags.
func GetKubeConfig() *rest.Config {
	if *kubeConfigFile != "" {
		klog.V(1).Infof("Using kubeconfig file: %s", *kubeConfigFile)
		// use the current context in kubeconfig
		config, err := clientcmd.BuildConfigFromFlags("", *kubeConfigFile)
		if err != nil {
			klog.Fatalf("Failed to build config: %v", err)
		}
		return config
	} else {
		config, err := rest.InClusterConfig()
		if err != nil {
			klog.Fatalf("Failed to create config: %v", err)
		}
		return config
	}
}
