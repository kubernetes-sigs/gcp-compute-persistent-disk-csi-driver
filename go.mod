module sigs.k8s.io/gcp-compute-persistent-disk-csi-driver

go 1.17

require (
	cloud.google.com/go v0.65.0
	github.com/GoogleCloudPlatform/k8s-cloud-provider v0.0.0-20190822182118-27a4ced34534
	github.com/container-storage-interface/spec v1.2.0
	github.com/golang/protobuf v1.4.2
	github.com/google/uuid v1.1.1
	github.com/hashicorp/go-multierror v1.0.0 // indirect
	github.com/kubernetes-csi/csi-proxy/client v1.0.0
	github.com/kubernetes-csi/csi-test/v3 v3.0.0
	github.com/onsi/ginkgo v1.11.0
	github.com/onsi/gomega v1.7.1
	golang.org/x/net v0.0.0-20201021035429-f5854403a974 // indirect
	golang.org/x/oauth2 v0.0.0-20200902213428-5d25da1a8d43
	golang.org/x/sys v0.0.0-20200930185726-fdedc70b468f
	google.golang.org/api v0.34.0
	google.golang.org/genproto v0.0.0-20200904004341-0bd0a958aa1d
	google.golang.org/grpc v1.31.1
	gopkg.in/gcfg.v1 v1.2.3
	gopkg.in/warnings.v0 v0.1.2 // indirect
	k8s.io/api v0.19.0 // indirect
	k8s.io/apimachinery v0.18.0
	k8s.io/client-go v11.0.1-0.20190805182717-6502b5e7b1b5+incompatible
	k8s.io/component-base v0.19.0
	k8s.io/klog v1.0.0
	k8s.io/kubernetes v1.18.0
	k8s.io/mount-utils v0.20.6
	k8s.io/test-infra v0.0.0-20200115230622-70a5174aa78d
	k8s.io/utils v0.0.0-20201110183641-67b214c5f920
)

require (
	github.com/Microsoft/go-winio v0.4.16 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver v3.5.1+incompatible // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/go-logr/logr v0.2.0 // indirect
	github.com/gogo/protobuf v1.3.1 // indirect
	github.com/golang/groupcache v0.0.0-20200121045136-8c9f03a8e57e // indirect
	github.com/google/go-cmp v0.5.2 // indirect
	github.com/google/gofuzz v1.1.0 // indirect
	github.com/googleapis/gax-go/v2 v2.0.5 // indirect
	github.com/googleapis/gnostic v0.3.1 // indirect
	github.com/hashicorp/errwrap v1.0.0 // indirect
	github.com/hpcloud/tail v1.0.0 // indirect
	github.com/imdario/mergo v0.3.8 // indirect
	github.com/json-iterator/go v1.1.8 // indirect
	github.com/konsorten/go-windows-terminal-sequences v1.0.2 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.1 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/prometheus/client_golang v1.0.0 // indirect
	github.com/prometheus/client_model v0.2.0 // indirect
	github.com/prometheus/common v0.4.1 // indirect
	github.com/prometheus/procfs v0.0.8 // indirect
	github.com/sirupsen/logrus v1.4.2 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	go.opencensus.io v0.22.4 // indirect
	golang.org/x/crypto v0.0.0-20200622213623-75b288015ac9 // indirect
	golang.org/x/text v0.3.3 // indirect
	golang.org/x/time v0.0.0-20191024005414-555d28b269f0 // indirect
	google.golang.org/appengine v1.6.6 // indirect
	google.golang.org/protobuf v1.25.0 // indirect
	gopkg.in/fsnotify.v1 v1.4.7 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	gopkg.in/yaml.v2 v2.2.8 // indirect
	k8s.io/klog/v2 v2.4.0 // indirect
	sigs.k8s.io/structured-merge-diff/v3 v3.0.0 // indirect
	sigs.k8s.io/yaml v1.2.0 // indirect
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
