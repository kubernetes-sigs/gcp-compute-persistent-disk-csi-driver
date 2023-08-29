# v1.10.7 - Changelog since v1.10.6

## Changes by Kind

### Uncategorized

- Update go version to 1.20.7 to fix CVE-2023-29409 CVE-2023-39533 ([#1348](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1348), [@k8s-infra-cherrypick-robot](https://github.com/k8s-infra-cherrypick-robot))

# v1.10.6 - Changelog since v1.10.5

## Changes by Kind

### Bug or Regression

- Update go version to 1.20.6 to fix CVE-2023-29406 ([#1331](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1331), [@pwschuurman](https://github.com/pwschuurman))

# v1.10.5 - Changelog since v1.10.4

## Changes by Kind

### Bug or Regression

- Add option for serializing formatAndMount, including fsck as well as mkfs. ([#1313](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1313), [@mattcary](https://github.com/mattcary))

### Uncategorized

- Add disk type for all operations metrics. ([#1295](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1295), [@k8s-infra-cherrypick-robot](https://github.com/k8s-infra-cherrypick-robot))
- Fix resource parsing when the gcp project name ends with alpha, beta or v1 ([#1307](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1307), [@k8s-infra-cherrypick-robot](https://github.com/k8s-infra-cherrypick-robot))
- Use original error code when responding with a backoff error on publish or unpublish. ([#1311](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1311), [@k8s-infra-cherrypick-robot](https://github.com/k8s-infra-cherrypick-robot))
- Added support in PDCSI driver to create confidential hyperdisk storage on GCE. ([#1315](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1315), [@k8s-infra-cherrypick-robot](https://github.com/k8s-infra-cherrypick-robot))


# v1.10.4 - Changelog since v1.10.3

## Changes by Kind

## Features

- Add alpha-level force attach storage class parameter. ([#1257](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1257), [@k8s-infra-cherrypick-robot](https://github.com/k8s-infra-cherrypick-robot))

## Bug or Regression

- Fix provisioned-iops-on-create passing logic([#1282](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1282))
- Bugfix for empty disk type being registered in metric for Create volume function. ([#1267](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1267), [@k8s-infra-cherrypick-robot](https://github.com/k8s-infra-cherrypick-robot))

### Cleanup

- Update go version to 1.20.5 to address CVE fixes ([#1265](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1265), [@k8s-infra-cherrypick-robot](https://github.com/k8s-infra-cherrypick-robot))
- [release-1.10] Update Docker.Windows to 1.20.5 ([#1272](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1272), [@saikat-royc](https://github.com/saikat-royc))

# v1.10.3 - Changelog since v1.10.2

### Feature

- Add alpha-level force attach storage class parameter. ([#1257](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1257), [@k8s-infra-cherrypick-robot](https://github.com/k8s-infra-cherrypick-robot))

### Cleanup

- Add metrics for CSI server side error count ([#1237](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1237), [@k8s-infra-cherrypick-robot](https://github.com/k8s-infra-cherrypick-robot))


# v1.10.2 - Changelog since v1.10.1

### Bug or Regression

- Add libraries needed for determining XFS volume expansion ([#1204](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1204), [@nberlee](https://github.com/nberlee))

### Cleanup

- Add metrics for CSI server side error count ([#1237](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1237), [@k8s-infra-cherrypick-robot](https://github.com/k8s-infra-cherrypick-robot))
- Updates error message to be more user friendly when PD CSI Driver encounters an disk type UNSUPPORTED_OPERATION on ControllerPublishVolume ([#1221](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1221), [@pwschuurman](https://github.com/pwschuurman))
- Use errors.As so we can detect wrapped errors, and check for existing error codes in CodesForError ([#1233](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1233), [@k8s-infra-cherrypick-robot](https://github.com/k8s-infra-cherrypick-robot))

# v1.10.1 - Changelog since v1.10.0

## Changes by Kind

### Bug or Regression

- Add missing libraries, libbsd and libmd, that are dependencies for XFS volume expansion. ([#1204](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1204), [@nberlee](https://github.com/nberlee))

# v1.10.0 - Changelog since v1.9.2

## Changes by Kind

### Feature
- It is no longer necessary to specify allowedTopologies for zonal -> zonal cloning and zonal/regional -> regional cloning to ensure that the clone zone is compatible with the GCE volume cloning requirements. ([#1150](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1150), [@amacaskill](https://github.com/amacaskill))

### Bug or Regression

- Fix missing libedit.so.2 error ([#1177](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1177), [@sunnylovestiramisu](https://github.com/sunnylovestiramisu))
- Set concurrency limit to prevent OOMing when issuing multiple concurrent mkfs calls. ([#1169](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1169/commits/7f1e04e15631b14f7d3f441cc97bed1a968f2e47), [@artemvmin](https://github.com/artemvmin))

### Other (Cleanup or Flake)

- Update go version to 1.20.3 for k/k 1.27 ([#1180](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1180), [@sunnylovestiramisu](https://github.com/sunnylovestiramisu))
- Upgrade mount-utils to v0.27.0. ([#1169](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1169/commits/232bd0aa691a72c1bd806d8c4ed881bb3a47174f), [@artemvmin](https://github.com/artemvmin))
- Break dependency on k/k test/e2e/storage/podlogs. ([#1169](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1169/commits/1b6e7e537bbc264a1c076c317e6194a976509011), [@artemvmin](https://github.com/artemvmin))

## Dependencies

### Added
- cloud.google.com/go/accessapproval: v1.5.0
- cloud.google.com/go/accesscontextmanager: v1.4.0
- cloud.google.com/go/aiplatform: v1.27.0
- cloud.google.com/go/analytics: v0.12.0
- cloud.google.com/go/apigateway: v1.4.0
- cloud.google.com/go/apigeeconnect: v1.4.0
- cloud.google.com/go/apigeeregistry: v0.4.0
- cloud.google.com/go/apikeys: v0.4.0
- cloud.google.com/go/appengine: v1.5.0
- cloud.google.com/go/area120: v0.6.0
- cloud.google.com/go/artifactregistry: v1.9.0
- cloud.google.com/go/asset: v1.10.0
- cloud.google.com/go/assuredworkloads: v1.9.0
- cloud.google.com/go/automl: v1.8.0
- cloud.google.com/go/baremetalsolution: v0.4.0
- cloud.google.com/go/batch: v0.4.0
- cloud.google.com/go/beyondcorp: v0.3.0
- cloud.google.com/go/billing: v1.7.0
- cloud.google.com/go/binaryauthorization: v1.4.0
- cloud.google.com/go/certificatemanager: v1.4.0
- cloud.google.com/go/channel: v1.9.0
- cloud.google.com/go/cloudbuild: v1.4.0
- cloud.google.com/go/clouddms: v1.4.0
- cloud.google.com/go/cloudtasks: v1.8.0
- cloud.google.com/go/compute/metadata: v0.2.3
- cloud.google.com/go/contactcenterinsights: v1.4.0
- cloud.google.com/go/container: v1.7.0
- cloud.google.com/go/containeranalysis: v0.6.0
- cloud.google.com/go/datacatalog: v1.8.0
- cloud.google.com/go/dataflow: v0.7.0
- cloud.google.com/go/dataform: v0.5.0
- cloud.google.com/go/datafusion: v1.5.0
- cloud.google.com/go/datalabeling: v0.6.0
- cloud.google.com/go/dataplex: v1.4.0
- cloud.google.com/go/dataproc: v1.8.0
- cloud.google.com/go/dataqna: v0.6.0
- cloud.google.com/go/datastream: v1.5.0
- cloud.google.com/go/deploy: v1.5.0
- cloud.google.com/go/dialogflow: v1.29.0
- cloud.google.com/go/dlp: v1.7.0
- cloud.google.com/go/documentai: v1.10.0
- cloud.google.com/go/domains: v0.7.0
- cloud.google.com/go/edgecontainer: v0.2.0
- cloud.google.com/go/errorreporting: v0.3.0
- cloud.google.com/go/essentialcontacts: v1.4.0
- cloud.google.com/go/eventarc: v1.8.0
- cloud.google.com/go/filestore: v1.4.0
- cloud.google.com/go/functions: v1.9.0
- cloud.google.com/go/gaming: v1.8.0
- cloud.google.com/go/gkebackup: v0.3.0
- cloud.google.com/go/gkeconnect: v0.6.0
- cloud.google.com/go/gkehub: v0.10.0
- cloud.google.com/go/gkemulticloud: v0.4.0
- cloud.google.com/go/gsuiteaddons: v1.4.0
- cloud.google.com/go/iap: v1.5.0
- cloud.google.com/go/ids: v1.2.0
- cloud.google.com/go/iot: v1.4.0
- cloud.google.com/go/language: v1.8.0
- cloud.google.com/go/lifesciences: v0.6.0
- cloud.google.com/go/longrunning: v0.3.0
- cloud.google.com/go/managedidentities: v1.4.0
- cloud.google.com/go/maps: v0.1.0
- cloud.google.com/go/mediatranslation: v0.6.0
- cloud.google.com/go/memcache: v1.7.0
- cloud.google.com/go/metastore: v1.8.0
- cloud.google.com/go/monitoring: v1.8.0
- cloud.google.com/go/networkconnectivity: v1.7.0
- cloud.google.com/go/networkmanagement: v1.5.0
- cloud.google.com/go/networksecurity: v0.6.0
- cloud.google.com/go/notebooks: v1.5.0
- cloud.google.com/go/optimization: v1.2.0
- cloud.google.com/go/orchestration: v1.4.0
- cloud.google.com/go/orgpolicy: v1.5.0
- cloud.google.com/go/osconfig: v1.10.0
- cloud.google.com/go/oslogin: v1.7.0
- cloud.google.com/go/phishingprotection: v0.6.0
- cloud.google.com/go/policytroubleshooter: v1.4.0
- cloud.google.com/go/privatecatalog: v0.6.0
- cloud.google.com/go/pubsublite: v1.5.0
- cloud.google.com/go/recaptchaenterprise/v2: v2.5.0
- cloud.google.com/go/recommendationengine: v0.6.0
- cloud.google.com/go/recommender: v1.8.0
- cloud.google.com/go/redis: v1.10.0
- cloud.google.com/go/resourcemanager: v1.4.0
- cloud.google.com/go/resourcesettings: v1.4.0
- cloud.google.com/go/retail: v1.11.0
- cloud.google.com/go/run: v0.3.0
- cloud.google.com/go/scheduler: v1.7.0
- cloud.google.com/go/secretmanager: v1.9.0
- cloud.google.com/go/security: v1.10.0
- cloud.google.com/go/securitycenter: v1.16.0
- cloud.google.com/go/servicecontrol: v1.5.0
- cloud.google.com/go/servicedirectory: v1.7.0
- cloud.google.com/go/servicemanagement: v1.5.0
- cloud.google.com/go/serviceusage: v1.4.0
- cloud.google.com/go/shell: v1.4.0
- cloud.google.com/go/spanner: v1.41.0
- cloud.google.com/go/speech: v1.9.0
- cloud.google.com/go/storagetransfer: v1.6.0
- cloud.google.com/go/talent: v1.4.0
- cloud.google.com/go/texttospeech: v1.5.0
- cloud.google.com/go/tpu: v1.4.0
- cloud.google.com/go/trace: v1.4.0
- cloud.google.com/go/translate: v1.4.0
- cloud.google.com/go/video: v1.9.0
- cloud.google.com/go/videointelligence: v1.9.0
- cloud.google.com/go/vision/v2: v2.5.0
- cloud.google.com/go/vmmigration: v1.3.0
- cloud.google.com/go/vmwareengine: v0.1.0
- cloud.google.com/go/vpcaccess: v1.5.0
- cloud.google.com/go/webrisk: v1.7.0
- cloud.google.com/go/websecurityscanner: v1.4.0
- cloud.google.com/go/workflows: v1.9.0
- github.com/docker/spdystream: [449fdfc](https://github.com/docker/spdystream/tree/449fdfc)
- github.com/golangplus/bytes: [45c989f](https://github.com/golangplus/bytes/tree/45c989f)
- github.com/golangplus/fmt: [2a5d6d7](https://github.com/golangplus/fmt/tree/2a5d6d7)
- github.com/natefinch/lumberjack: [v2.0.0+incompatible](https://github.com/natefinch/lumberjack/tree/v2.0.0)
- gopkg.in/yaml.v1: 9f9df34
- sigs.k8s.io/kustomize: v2.0.3+incompatible
- sigs.k8s.io/structured-merge-diff/v3: v3.0.0

### Changed
- cloud.google.com/go/bigquery: v1.8.0 → v1.44.0
- cloud.google.com/go/compute: v1.7.0 → v1.18.0
- cloud.google.com/go/datastore: v1.1.0 → v1.10.0
- cloud.google.com/go/firestore: v1.1.0 → v1.9.0
- cloud.google.com/go/iam: v0.3.0 → v0.11.0
- cloud.google.com/go/kms: v1.4.0 → v1.6.0
- cloud.google.com/go/logging: v1.0.0 → v1.6.1
- cloud.google.com/go/pubsub: v1.4.0 → v1.27.1
- cloud.google.com/go/storage: v1.23.0 → v1.12.0
- cloud.google.com/go: v0.103.0 → v0.107.0
- dmitri.shuralyov.com/gpu/mtl: 28db891 → 666a987
- github.com/Azure/azure-sdk-for-go: [v55.0.0+incompatible → v42.3.0+incompatible](https://github.com/Azure/azure-sdk-for-go/compare/v55.0.0...v42.3.0)
- github.com/Azure/go-autorest/autorest/to: [v0.4.0 → v0.3.0](https://github.com/Azure/go-autorest/autorest/to/compare/v0.4.0...v0.3.0)
- github.com/Microsoft/hcsshim: [v0.8.22 → v0.8.7](https://github.com/Microsoft/hcsshim/compare/v0.8.22...v0.8.7)
- github.com/aws/aws-sdk-go: [v1.38.49 → v1.37.22](https://github.com/aws/aws-sdk-go/compare/v1.38.49...v1.37.22)
- github.com/census-instrumentation/opencensus-proto: [v0.2.1 → v0.4.1](https://github.com/census-instrumentation/opencensus-proto/compare/v0.2.1...v0.4.1)
- github.com/cespare/xxhash/v2: [v2.1.2 → v2.2.0](https://github.com/cespare/xxhash/v2/compare/v2.1.2...v2.2.0)
- github.com/cncf/udpa/go: [04548b0 → c52dc94](https://github.com/cncf/udpa/go/compare/04548b0...c52dc94)
- github.com/cncf/xds/go: [cb28da3 → 06c439d](https://github.com/cncf/xds/go/compare/cb28da3...06c439d)
- github.com/containerd/cgroups: [v1.0.1 → bf292b2](https://github.com/containerd/cgroups/compare/v1.0.1...bf292b2)
- github.com/containerd/console: [v1.0.3 → c12b1e7](https://github.com/containerd/console/compare/v1.0.3...c12b1e7)
- github.com/containerd/containerd: [v1.4.12 → v1.3.3](https://github.com/containerd/containerd/compare/v1.4.12...v1.3.3)
- github.com/containerd/continuity: [v0.1.0 → 26c1120](https://github.com/containerd/continuity/compare/v0.1.0...26c1120)
- github.com/containerd/fifo: [v1.0.0 → a9fb20d](https://github.com/containerd/fifo/compare/v1.0.0...a9fb20d)
- github.com/containerd/go-runc: [v1.0.0 → 5a6d9f3](https://github.com/containerd/go-runc/compare/v1.0.0...5a6d9f3)
- github.com/containerd/ttrpc: [v1.0.2 → 0e0f228](https://github.com/containerd/ttrpc/compare/v1.0.2...0e0f228)
- github.com/containerd/typeurl: [v1.0.2 → a93fcdb](https://github.com/containerd/typeurl/compare/v1.0.2...a93fcdb)
- github.com/coreos/bbolt: [v1.3.2 → v1.3.3](https://github.com/coreos/bbolt/compare/v1.3.2...v1.3.3)
- github.com/coreos/etcd: [v3.3.13+incompatible → v3.3.17+incompatible](https://github.com/coreos/etcd/compare/v3.3.13...v3.3.17)
- github.com/cyphar/filepath-securejoin: [v0.2.3 → v0.2.2](https://github.com/cyphar/filepath-securejoin/compare/v0.2.3...v0.2.2)
- github.com/docker/distribution: [v2.8.1+incompatible → v2.7.1+incompatible](https://github.com/docker/distribution/compare/v2.8.1...v2.7.1)
- github.com/docker/docker: [v20.10.12+incompatible → v1.13.1](https://github.com/docker/docker/compare/v20.10.12...v1.13.1)
- github.com/envoyproxy/go-control-plane: [49ff273 → v0.10.3](https://github.com/envoyproxy/go-control-plane/compare/49ff273...v0.10.3)
- github.com/envoyproxy/protoc-gen-validate: [v0.1.0 → v0.9.1](https://github.com/envoyproxy/protoc-gen-validate/compare/v0.1.0...v0.9.1)
- github.com/frankban/quicktest: [v1.11.3 → v1.8.1](https://github.com/frankban/quicktest/compare/v1.11.3...v1.8.1)
- github.com/godbus/dbus/v5: [v5.0.6 → v5.0.4](https://github.com/godbus/dbus/v5/compare/v5.0.6...v5.0.4)
- github.com/googleapis/enterprise-certificate-proxy: [v0.1.0 → v0.2.3](https://github.com/googleapis/enterprise-certificate-proxy/compare/v0.1.0...v0.2.3)
- github.com/googleapis/gax-go/v2: [v2.4.0 → v2.7.0](https://github.com/googleapis/gax-go/v2/compare/v2.4.0...v2.7.0)
- github.com/gopherjs/gopherjs: [fce0ec3 → 0766667](https://github.com/gopherjs/gopherjs/compare/fce0ec3...0766667)
- github.com/karrick/godirwalk: [v1.16.1 → v1.10.3](https://github.com/karrick/godirwalk/compare/v1.16.1...v1.10.3)
- github.com/kr/pretty: [v0.2.1 → v0.3.0](https://github.com/kr/pretty/compare/v0.2.1...v0.3.0)
- github.com/moby/sys/mountinfo: [v0.6.0 → v0.6.2](https://github.com/moby/sys/mountinfo/compare/v0.6.0...v0.6.2)
- github.com/olekukonko/tablewriter: [v0.0.4 → a0225b3](https://github.com/olekukonko/tablewriter/compare/v0.0.4...a0225b3)
- github.com/opencontainers/go-digest: [v1.0.0 → v1.0.0-rc1](https://github.com/opencontainers/go-digest/compare/v1.0.0...v1.0.0-rc1)
- github.com/opencontainers/image-spec: [v1.0.2 → v1.0.1](https://github.com/opencontainers/image-spec/compare/v1.0.2...v1.0.1)
- github.com/opencontainers/runc: [v1.1.1 → v0.1.1](https://github.com/opencontainers/runc/compare/v1.1.1...v0.1.1)
- github.com/opencontainers/runtime-spec: [1c3f411 → 5b71a03](https://github.com/opencontainers/runtime-spec/compare/1c3f411...5b71a03)
- github.com/rubiojr/go-vhd: [02e2102 → 0bfd3b3](https://github.com/rubiojr/go-vhd/compare/02e2102...0bfd3b3)
- github.com/smartystreets/assertions: [v1.1.0 → v1.0.0](https://github.com/smartystreets/assertions/compare/v1.1.0...v1.0.0)
- github.com/stretchr/objx: [v0.2.0 → v0.5.0](https://github.com/stretchr/objx/compare/v0.2.0...v0.5.0)
- github.com/stretchr/testify: [v1.7.0 → v1.8.1](https://github.com/stretchr/testify/compare/v1.7.0...v1.8.1)
- github.com/syndtr/gocapability: [42c35b4 → db04d3c](https://github.com/syndtr/gocapability/compare/42c35b4...db04d3c)
- github.com/urfave/cli: [v1.22.2 → v1.20.0](https://github.com/urfave/cli/compare/v1.22.2...v1.20.0)
- go.etcd.io/etcd: 83304cf → dd1b699
- go.opencensus.io: v0.23.0 → v0.24.0
- golang.org/x/exp: 85be41e → 6cc2880
- golang.org/x/mobile: e6ae53a → 597adff
- golang.org/x/net: v0.5.0 → v0.7.0
- golang.org/x/oauth2: 128564f → v0.5.0
- golang.org/x/sync: 0de741c → v0.1.0
- golang.org/x/sys: v0.4.0 → v0.5.0
- golang.org/x/term: v0.4.0 → v0.5.0
- golang.org/x/text: v0.6.0 → v0.7.0
- golang.org/x/xerrors: 65e6541 → 5ec99f8
- gonum.org/v1/gonum: v0.6.2 → 3d26580
- google.golang.org/api: v0.86.0 → v0.111.0
- google.golang.org/genproto: 176da50 → 637eb22
- google.golang.org/grpc: v1.48.0 → v1.53.0
- google.golang.org/protobuf: v1.28.0 → v1.28.1
- gopkg.in/check.v1: 8fa4692 → 10cb982
- k8s.io/apiextensions-apiserver: v0.24.1 → v0.21.1
- k8s.io/cli-runtime: v0.24.1 → v0.17.3
- k8s.io/code-generator: v0.24.1 → v0.21.1
- k8s.io/csi-translation-lib: v0.24.1 → v0.17.4
- k8s.io/gengo: c02415c → 485abfe
- k8s.io/klog/v2: v2.60.1 → v2.90.1
- k8s.io/kubectl: v0.24.1 → v0.17.2
- k8s.io/kubernetes: v1.24.1 → v1.14.7
- k8s.io/legacy-cloud-providers: v0.24.1 → v0.17.4
- k8s.io/metrics: v0.24.1 → v0.17.2
- k8s.io/mount-utils: v0.24.1 → v0.27.0-alpha.3
- k8s.io/utils: 56c0de1 → a36077c
- sigs.k8s.io/structured-merge-diff: 15d366b → v1.0.1

### Removed
- bitbucket.org/bertimus9/systemstat: 0eeff89
- github.com/JeffAshton/win_pdh: [76bb4ee](https://github.com/JeffAshton/win_pdh/tree/76bb4ee)
- github.com/ajstarks/svgo: [644b8db](https://github.com/ajstarks/svgo/tree/644b8db)
- github.com/antlr/antlr4/runtime/Go/antlr: [b48c857](https://github.com/antlr/antlr4/runtime/Go/antlr/tree/b48c857)
- github.com/auth0/go-jwt-middleware: [v1.0.1](https://github.com/auth0/go-jwt-middleware/tree/v1.0.1)
- github.com/boltdb/bolt: [v1.3.1](https://github.com/boltdb/bolt/tree/v1.3.1)
- github.com/checkpoint-restore/go-criu/v5: [v5.3.0](https://github.com/checkpoint-restore/go-criu/v5/tree/v5.3.0)
- github.com/cilium/ebpf: [v0.7.0](https://github.com/cilium/ebpf/tree/v0.7.0)
- github.com/clusterhq/flocker-go: [2b8b725](https://github.com/clusterhq/flocker-go/tree/2b8b725)
- github.com/coredns/caddy: [v1.1.0](https://github.com/coredns/caddy/tree/v1.1.0)
- github.com/coredns/corefile-migration: [v1.0.14](https://github.com/coredns/corefile-migration/tree/v1.0.14)
- github.com/euank/go-kmsg-parser: [v2.0.0+incompatible](https://github.com/euank/go-kmsg-parser/tree/v2.0.0)
- github.com/fogleman/gg: [0403632](https://github.com/fogleman/gg/tree/0403632)
- github.com/go-errors/errors: [v1.0.1](https://github.com/go-errors/errors/tree/v1.0.1)
- github.com/go-ozzo/ozzo-validation: [v3.5.0+incompatible](https://github.com/go-ozzo/ozzo-validation/tree/v3.5.0)
- github.com/gofrs/uuid: [v4.0.0+incompatible](https://github.com/gofrs/uuid/tree/v4.0.0)
- github.com/golang/freetype: [e2365df](https://github.com/golang/freetype/tree/e2365df)
- github.com/google/cadvisor: [v0.44.1](https://github.com/google/cadvisor/tree/v0.44.1)
- github.com/google/cel-go: [v0.10.1](https://github.com/google/cel-go/tree/v0.10.1)
- github.com/google/cel-spec: [v0.6.0](https://github.com/google/cel-spec/tree/v0.6.0)
- github.com/google/shlex: [e7afc7f](https://github.com/google/shlex/tree/e7afc7f)
- github.com/googleapis/go-type-adapters: [v1.0.0](https://github.com/googleapis/go-type-adapters/tree/v1.0.0)
- github.com/heketi/heketi: [v10.3.0+incompatible](https://github.com/heketi/heketi/tree/v10.3.0)
- github.com/heketi/tests: [f3775cb](https://github.com/heketi/tests/tree/f3775cb)
- github.com/ishidawataru/sctp: [7c296d4](https://github.com/ishidawataru/sctp/tree/7c296d4)
- github.com/jung-kurt/gofpdf: [24315ac](https://github.com/jung-kurt/gofpdf/tree/24315ac)
- github.com/libopenstorage/openstorage: [v1.0.0](https://github.com/libopenstorage/openstorage/tree/v1.0.0)
- github.com/lpabon/godbc: [v0.1.1](https://github.com/lpabon/godbc/tree/v0.1.1)
- github.com/mindprince/gonvml: [9ebdce4](https://github.com/mindprince/gonvml/tree/9ebdce4)
- github.com/mistifyio/go-zfs: [f784269](https://github.com/mistifyio/go-zfs/tree/f784269)
- github.com/moby/ipvs: [v1.0.1](https://github.com/moby/ipvs/tree/v1.0.1)
- github.com/monochromegane/go-gitignore: [205db1a](https://github.com/monochromegane/go-gitignore/tree/205db1a)
- github.com/mrunalp/fileutils: [v0.5.0](https://github.com/mrunalp/fileutils/tree/v0.5.0)
- github.com/mvdan/xurls: [v1.1.0](https://github.com/mvdan/xurls/tree/v1.1.0)
- github.com/opencontainers/selinux: [v1.10.0](https://github.com/opencontainers/selinux/tree/v1.10.0)
- github.com/quobyte/api: [v0.1.8](https://github.com/quobyte/api/tree/v0.1.8)
- github.com/robfig/cron/v3: [v3.0.1](https://github.com/robfig/cron/v3/tree/v3.0.1)
- github.com/seccomp/libseccomp-golang: [3879420](https://github.com/seccomp/libseccomp-golang/tree/3879420)
- github.com/storageos/go-api: [v2.2.0+incompatible](https://github.com/storageos/go-api/tree/v2.2.0)
- github.com/urfave/negroni: [v1.0.0](https://github.com/urfave/negroni/tree/v1.0.0)
- github.com/vishvananda/netlink: [v1.1.0](https://github.com/vishvananda/netlink/tree/v1.1.0)
- github.com/vishvananda/netns: [db3c7e5](https://github.com/vishvananda/netns/tree/db3c7e5)
- github.com/xlab/treeprint: [a009c39](https://github.com/xlab/treeprint/tree/a009c39)
- go.starlark.net: 8dd3e2e
- gonum.org/v1/plot: e2840ee
- k8s.io/cluster-bootstrap: v0.24.1
- k8s.io/cri-api: v0.24.1
- k8s.io/kube-aggregator: v0.24.1
- k8s.io/kube-controller-manager: v0.24.1
- k8s.io/kube-proxy: v0.24.1
- k8s.io/kube-scheduler: v0.24.1
- k8s.io/kubelet: v0.24.1
- k8s.io/pod-security-admission: v0.24.1
- k8s.io/sample-apiserver: v0.24.1
- k8s.io/system-validators: v1.7.0
- rsc.io/pdf: v0.1.1
- sigs.k8s.io/kustomize/api: v0.11.4
- sigs.k8s.io/kustomize/cmd/config: v0.10.6
- sigs.k8s.io/kustomize/kustomize/v4: v4.5.4
- sigs.k8s.io/kustomize/kyaml: v0.13.6
