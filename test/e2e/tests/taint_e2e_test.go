package tests

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var _ = Describe("Taint Controller", func() {
	It("Should remove taints from a node when the driver is healthy", func() {
		clientset := getKubeClientOrDie()

		nodes, err := clientset.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
		Expect(err).ToNot(HaveOccurred())
		Expect(nodes.Items).ToNot(BeEmpty())

		// Use the first node found
		testNodeName := nodes.Items[0].Name

		// 3. Add the Taint
		taintKey := "pd.csi.storage.gke.io/startup"
		err = addTaint(clientset, testNodeName, taintKey)
		Expect(err).ToNot(HaveOccurred())

		// 4. Wait for Removal
		Eventually(func() bool {
			node, err := clientset.CoreV1().Nodes().Get(context.TODO(), testNodeName, metav1.GetOptions{})
			if err != nil {
				return false
			}
			return hasTaint(node, taintKey)
		}, "2m", "5s").Should(BeFalse(), "Controller failed to remove the taint!")
	})
})

func getKubeClientOrDie() *kubernetes.Clientset {
	// Flags > Env > Home > InCluster
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	config, err := kubeConfig.ClientConfig()
	if err != nil {
		panic("Failed to load kubeconfig: " + err.Error())
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic("Failed to create k8s client: " + err.Error())
	}
	return clientset
}

func hasTaint(node *v1.Node, key string) bool {
	for _, t := range node.Spec.Taints {
		if t.Key == key {
			return true
		}
	}
	return false
}

func addTaint(client *kubernetes.Clientset, nodeName, key string) error {
	// Simple retry loop to handle conflicts
	node, err := client.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	node.Spec.Taints = append(node.Spec.Taints, v1.Taint{
		Key:    key,
		Effect: v1.TaintEffectNoSchedule,
	})

	_, err = client.CoreV1().Nodes().Update(context.TODO(), node, metav1.UpdateOptions{})
	return err
}
