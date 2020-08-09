module sigs.k8s.io/gcp-compute-persistent-disk-csi-driver

go 1.13

require (
	cloud.google.com/go v0.45.1
	github.com/GoogleCloudPlatform/k8s-cloud-provider v0.0.0-20190822182118-27a4ced34534
	github.com/container-storage-interface/spec v1.2.0
	github.com/golang/protobuf v1.3.4
	github.com/google/uuid v1.1.1
	github.com/hashicorp/go-multierror v1.0.0 // indirect
	github.com/kubernetes-csi/csi-proxy/client v0.0.0-20200330215040-9eff16441b2a
	github.com/kubernetes-csi/csi-test/v3 v3.0.0
	github.com/onsi/ginkgo v1.11.0
	github.com/onsi/gomega v1.7.1
	golang.org/x/oauth2 v0.0.0-20190604053449-0f29369cfe45
	golang.org/x/sys v0.0.0-20191113165036-4c7a9d0fe056
	golang.org/x/tools/gopls v0.3.3 // indirect
	google.golang.org/api v0.10.0
	google.golang.org/genproto v0.0.0-20191114150713-6bbd007550de
	google.golang.org/grpc v1.27.1
	gopkg.in/gcfg.v1 v1.2.3
	gopkg.in/warnings.v0 v0.1.2 // indirect
	k8s.io/apimachinery v0.18.0
	k8s.io/client-go v11.0.1-0.20190805182717-6502b5e7b1b5+incompatible
	k8s.io/klog v1.0.0
	k8s.io/kubernetes v1.18.0
	k8s.io/test-infra v0.0.0-20200115230622-70a5174aa78d
	k8s.io/utils v0.0.0-20200324210504-a9aa75ae1b89
)

replace k8s.io/api => k8s.io/api v0.18.0

replace k8s.io/apimachinery => k8s.io/apimachinery v0.18.0

replace k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.18.0

replace k8s.io/apiserver => k8s.io/apiserver v0.18.0

replace k8s.io/client-go => k8s.io/client-go v0.18.0

replace k8s.io/cli-runtime => k8s.io/cli-runtime v0.18.0

replace k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.18.0

replace k8s.io/cloud-provider => k8s.io/cloud-provider v0.18.0

replace k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.18.0

replace k8s.io/code-generator => k8s.io/code-generator v0.18.0

replace k8s.io/component-base => k8s.io/component-base v0.18.0

replace k8s.io/cri-api => k8s.io/cri-api v0.18.0

replace k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.18.0

replace k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.18.0

replace k8s.io/kube-proxy => k8s.io/kube-proxy v0.18.0

replace k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.18.0

replace k8s.io/kubectl => k8s.io/kubectl v0.18.0

replace k8s.io/kubelet => k8s.io/kubelet v0.18.0

replace k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.18.0

replace k8s.io/metrics => k8s.io/metrics v0.18.0

replace k8s.io/node-api => k8s.io/node-api v0.18.0

replace k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.18.0

replace k8s.io/sample-cli-plugin => k8s.io/sample-cli-plugin v0.18.0

replace k8s.io/sample-controller => k8s.io/sample-controller v0.18.0
