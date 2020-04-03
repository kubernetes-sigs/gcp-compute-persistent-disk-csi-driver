module sigs.k8s.io/gcp-compute-persistent-disk-csi-driver

go 1.13

require (
	cloud.google.com/go v0.45.1
	github.com/GoogleCloudPlatform/k8s-cloud-provider v0.0.0-20190612171043-2e19bb35a278
	github.com/container-storage-interface/spec v1.2.0
	github.com/golang/protobuf v1.3.4
	github.com/google/uuid v1.1.1
	github.com/hashicorp/go-multierror v1.0.0 // indirect
	github.com/kubernetes-csi/csi-proxy/client v0.0.0-20200330215040-9eff16441b2a
	github.com/kubernetes-csi/csi-test/v3 v3.0.0
	github.com/onsi/ginkgo v1.10.3
	github.com/onsi/gomega v1.7.1
	golang.org/x/oauth2 v0.0.0-20190604053449-0f29369cfe45
	golang.org/x/sys v0.0.0-20191113165036-4c7a9d0fe056
	golang.org/x/tools/gopls v0.3.3 // indirect
	google.golang.org/api v0.10.0
	google.golang.org/genproto v0.0.0-20191114150713-6bbd007550de
	google.golang.org/grpc v1.27.1
	gopkg.in/gcfg.v1 v1.2.3
	gopkg.in/warnings.v0 v0.1.2 // indirect
	k8s.io/apimachinery v0.17.1
	k8s.io/client-go v11.0.1-0.20190805182717-6502b5e7b1b5+incompatible
	k8s.io/klog v1.0.0
	k8s.io/test-infra v0.0.0-20200115230622-70a5174aa78d
	k8s.io/utils v0.0.0-20200124190032-861946025e34
)
