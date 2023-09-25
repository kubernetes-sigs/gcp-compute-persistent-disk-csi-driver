# v1.8.11 - Changelog since v1.8.10

## Changes by Kind

### Bug or Regression

- Upgrade google.golang.org/grpc from v1.55.0 -> v1.55.1 to address https://github.com/grpc/grpc-go/issues/6373 ([#1371](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1371), [@pwschuurman](https://github.com/pwschuurman))

# v1.8.10 - Changelog since v1.8.9

## Changes by Kind

### Bug or Regression

- Update go version to 1.19.12 to fix CVE-2023-29409 CVE-2023-39533 ([#1350](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1350), [@Sneha-at](https://github.com/Sneha-at))

# v1.8.9 - Changelog since v1.8.8

## Changes by Kind

### Bug or Regression

- Update go version to 1.19.11 to fix CVE-2023-29406 ([#1336](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1336), [@Sneha-at](https://github.com/Sneha-at))
- Updated dependencies to fix CVE-2022-27664, CVE-2022-32149, CVE-2022-41723, CVE-2022-41721 ([#1334](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1334), [@Sneha-at](https://github.com/Sneha-at))

### Uncategorized

- Add disk type for all operations metrics. ([#1297](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1297), [@k8s-infra-cherrypick-robot](https://github.com/k8s-infra-cherrypick-robot))

## Dependencies

### Added
- cloud.google.com/go/accessapproval: v1.6.0
- cloud.google.com/go/accesscontextmanager: v1.6.0
- cloud.google.com/go/aiplatform: v1.35.0
- cloud.google.com/go/analytics: v0.18.0
- cloud.google.com/go/apigateway: v1.5.0
- cloud.google.com/go/apigeeconnect: v1.5.0
- cloud.google.com/go/apigeeregistry: v0.5.0
- cloud.google.com/go/apikeys: v0.5.0
- cloud.google.com/go/appengine: v1.6.0
- cloud.google.com/go/area120: v0.7.1
- cloud.google.com/go/artifactregistry: v1.11.2
- cloud.google.com/go/asset: v1.11.1
- cloud.google.com/go/assuredworkloads: v1.10.0
- cloud.google.com/go/automl: v1.12.0
- cloud.google.com/go/baremetalsolution: v0.5.0
- cloud.google.com/go/batch: v0.7.0
- cloud.google.com/go/beyondcorp: v0.4.0
- cloud.google.com/go/billing: v1.12.0
- cloud.google.com/go/binaryauthorization: v1.5.0
- cloud.google.com/go/certificatemanager: v1.6.0
- cloud.google.com/go/channel: v1.11.0
- cloud.google.com/go/cloudbuild: v1.7.0
- cloud.google.com/go/clouddms: v1.5.0
- cloud.google.com/go/cloudtasks: v1.9.0
- cloud.google.com/go/compute/metadata: v0.2.3
- cloud.google.com/go/contactcenterinsights: v1.6.0
- cloud.google.com/go/container: v1.13.1
- cloud.google.com/go/containeranalysis: v0.7.0
- cloud.google.com/go/datacatalog: v1.12.0
- cloud.google.com/go/dataflow: v0.8.0
- cloud.google.com/go/dataform: v0.6.0
- cloud.google.com/go/datafusion: v1.6.0
- cloud.google.com/go/datalabeling: v0.7.0
- cloud.google.com/go/dataplex: v1.5.2
- cloud.google.com/go/dataproc: v1.12.0
- cloud.google.com/go/dataqna: v0.7.0
- cloud.google.com/go/datastream: v1.6.0
- cloud.google.com/go/deploy: v1.6.0
- cloud.google.com/go/dialogflow: v1.31.0
- cloud.google.com/go/dlp: v1.9.0
- cloud.google.com/go/documentai: v1.16.0
- cloud.google.com/go/domains: v0.8.0
- cloud.google.com/go/edgecontainer: v0.3.0
- cloud.google.com/go/errorreporting: v0.3.0
- cloud.google.com/go/essentialcontacts: v1.5.0
- cloud.google.com/go/eventarc: v1.10.0
- cloud.google.com/go/filestore: v1.5.0
- cloud.google.com/go/functions: v1.10.0
- cloud.google.com/go/gaming: v1.9.0
- cloud.google.com/go/gkebackup: v0.4.0
- cloud.google.com/go/gkeconnect: v0.7.0
- cloud.google.com/go/gkehub: v0.11.0
- cloud.google.com/go/gkemulticloud: v0.5.0
- cloud.google.com/go/gsuiteaddons: v1.5.0
- cloud.google.com/go/iap: v1.6.0
- cloud.google.com/go/ids: v1.3.0
- cloud.google.com/go/iot: v1.5.0
- cloud.google.com/go/language: v1.9.0
- cloud.google.com/go/lifesciences: v0.8.0
- cloud.google.com/go/longrunning: v0.4.1
- cloud.google.com/go/managedidentities: v1.5.0
- cloud.google.com/go/maps: v0.6.0
- cloud.google.com/go/mediatranslation: v0.7.0
- cloud.google.com/go/memcache: v1.9.0
- cloud.google.com/go/metastore: v1.10.0
- cloud.google.com/go/monitoring: v1.12.0
- cloud.google.com/go/networkconnectivity: v1.10.0
- cloud.google.com/go/networkmanagement: v1.6.0
- cloud.google.com/go/networksecurity: v0.7.0
- cloud.google.com/go/notebooks: v1.7.0
- cloud.google.com/go/optimization: v1.3.1
- cloud.google.com/go/orchestration: v1.6.0
- cloud.google.com/go/orgpolicy: v1.10.0
- cloud.google.com/go/osconfig: v1.11.0
- cloud.google.com/go/oslogin: v1.9.0
- cloud.google.com/go/phishingprotection: v0.7.0
- cloud.google.com/go/policytroubleshooter: v1.5.0
- cloud.google.com/go/privatecatalog: v0.7.0
- cloud.google.com/go/pubsublite: v1.6.0
- cloud.google.com/go/recaptchaenterprise/v2: v2.6.0
- cloud.google.com/go/recommendationengine: v0.7.0
- cloud.google.com/go/recommender: v1.9.0
- cloud.google.com/go/redis: v1.11.0
- cloud.google.com/go/resourcemanager: v1.5.0
- cloud.google.com/go/resourcesettings: v1.5.0
- cloud.google.com/go/retail: v1.12.0
- cloud.google.com/go/run: v0.8.0
- cloud.google.com/go/scheduler: v1.8.0
- cloud.google.com/go/secretmanager: v1.10.0
- cloud.google.com/go/security: v1.12.0
- cloud.google.com/go/securitycenter: v1.18.1
- cloud.google.com/go/servicecontrol: v1.11.0
- cloud.google.com/go/servicedirectory: v1.8.0
- cloud.google.com/go/servicemanagement: v1.6.0
- cloud.google.com/go/serviceusage: v1.5.0
- cloud.google.com/go/shell: v1.6.0
- cloud.google.com/go/spanner: v1.44.0
- cloud.google.com/go/speech: v1.14.1
- cloud.google.com/go/storagetransfer: v1.7.0
- cloud.google.com/go/talent: v1.5.0
- cloud.google.com/go/texttospeech: v1.6.0
- cloud.google.com/go/tpu: v1.5.0
- cloud.google.com/go/trace: v1.8.0
- cloud.google.com/go/translate: v1.6.0
- cloud.google.com/go/video: v1.13.0
- cloud.google.com/go/videointelligence: v1.10.0
- cloud.google.com/go/vision/v2: v2.6.0
- cloud.google.com/go/vmmigration: v1.5.0
- cloud.google.com/go/vmwareengine: v0.2.2
- cloud.google.com/go/vpcaccess: v1.6.0
- cloud.google.com/go/webrisk: v1.8.0
- cloud.google.com/go/websecurityscanner: v1.5.0
- cloud.google.com/go/workflows: v1.10.0

### Changed
- cloud.google.com/go/bigquery: v1.8.0 → v1.48.0
- cloud.google.com/go/compute: v1.7.0 → v1.18.0
- cloud.google.com/go/datastore: v1.1.0 → v1.10.0
- cloud.google.com/go/firestore: v1.1.0 → v1.9.0
- cloud.google.com/go/iam: v0.3.0 → v0.12.0
- cloud.google.com/go/kms: v1.4.0 → v1.9.0
- cloud.google.com/go/logging: v1.0.0 → v1.7.0
- cloud.google.com/go/pubsub: v1.4.0 → v1.28.0
- cloud.google.com/go/storage: v1.23.0 → v1.12.0
- cloud.google.com/go: v0.103.0 → v0.110.0
- github.com/census-instrumentation/opencensus-proto: [v0.2.1 → v0.4.1](https://github.com/census-instrumentation/opencensus-proto/compare/v0.2.1...v0.4.1)
- github.com/cespare/xxhash/v2: [v2.1.2 → v2.2.0](https://github.com/cespare/xxhash/v2/compare/v2.1.2...v2.2.0)
- github.com/cncf/udpa/go: [04548b0 → c52dc94](https://github.com/cncf/udpa/go/compare/04548b0...c52dc94)
- github.com/cncf/xds/go: [cb28da3 → 32f1caf](https://github.com/cncf/xds/go/compare/cb28da3...32f1caf)
- github.com/envoyproxy/go-control-plane: [49ff273 → v0.11.0](https://github.com/envoyproxy/go-control-plane/compare/49ff273...v0.11.0)
- github.com/envoyproxy/protoc-gen-validate: [v0.1.0 → v0.10.0](https://github.com/envoyproxy/protoc-gen-validate/compare/v0.1.0...v0.10.0)
- github.com/golang/glog: [v1.0.0 → v1.1.0](https://github.com/golang/glog/compare/v1.0.0...v1.1.0)
- github.com/golang/protobuf: [v1.5.2 → v1.5.3](https://github.com/golang/protobuf/compare/v1.5.2...v1.5.3)
- github.com/google/go-cmp: [v0.5.8 → v0.5.9](https://github.com/google/go-cmp/compare/v0.5.8...v0.5.9)
- github.com/googleapis/enterprise-certificate-proxy: [v0.1.0 → v0.2.3](https://github.com/googleapis/enterprise-certificate-proxy/compare/v0.1.0...v0.2.3)
- github.com/googleapis/gax-go/v2: [v2.4.0 → v2.7.0](https://github.com/googleapis/gax-go/v2/compare/v2.4.0...v2.7.0)
- github.com/stretchr/objx: [v0.2.0 → v0.5.0](https://github.com/stretchr/objx/compare/v0.2.0...v0.5.0)
- github.com/stretchr/testify: [v1.7.0 → v1.8.1](https://github.com/stretchr/testify/compare/v1.7.0...v1.8.1)
- go.opencensus.io: v0.23.0 → v0.24.0
- golang.org/x/mod: 9b9b3d8 → v0.8.0
- golang.org/x/net: a158d28 → v0.8.0
- golang.org/x/oauth2: 128564f → v0.6.0
- golang.org/x/sync: 0de741c → v0.1.0
- golang.org/x/sys: 8c9f86f → v0.6.0
- golang.org/x/term: 03fcf44 → v0.6.0
- golang.org/x/text: v0.3.7 → v0.8.0
- golang.org/x/tools: 897bd77 → v0.6.0
- golang.org/x/xerrors: 65e6541 → 5ec99f8
- google.golang.org/api: v0.86.0 → v0.110.0
- google.golang.org/genproto: 176da50 → 7f2fa6f
- google.golang.org/grpc: v1.48.0 → v1.55.0
- google.golang.org/protobuf: v1.28.0 → v1.30.0

### Removed
- github.com/googleapis/go-type-adapters: [v1.0.0](https://github.com/googleapis/go-type-adapters/tree/v1.0.0)

# v1.8.8 - Changelog since v1.8.7

## Changes by Kind

### Uncategorized

- Add disk type for all operations metrics. ([#1297](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1297), [@k8s-infra-cherrypick-robot](https://github.com/k8s-infra-cherrypick-robot))
- Fix provisioned-iops-on-create passing logic ([#1284](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1284), [@sunnylovestiramisu](https://github.com/sunnylovestiramisu))


# v1.8.7 - Changelog since v1.8.6

### Bug or Regression

- Fix provisioned-iops-on-create passing logic([#1284](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1284))
- Bugfix for empty disk type being registered in metric for Create volume function. ([#1269](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1269), [@k8s-infra-cherrypick-robot](https://github.com/k8s-infra-cherrypick-robot))

### Cleanup

- #1079: Add provisionedIops for pd-extreme
  #1101: Add provisionedThroughput for hyperdisk
  #1240: Change iops params directly convert string to int64 ([#1241](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1241), [@sunnylovestiramisu](https://github.com/sunnylovestiramisu))
- #1150: fix bug where volume cloning topology requirements are
  #1232: Use errors.As so we can detect wrapped errors, and check for
  #1227: Adding new metric pdcsi_operation_errors to fetch error ([#1244](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1244), [@sunnylovestiramisu](https://github.com/sunnylovestiramisu))
- Update go version to 1.19.10 ([#1271](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1271), [@saikat-royc](https://github.com/saikat-royc))

# v1.8.6 - Changelog since v1.8.5

## Changes by Kind

### Cleanup

- Updates error message to be more user friendly when PD CSI Driver encounters an disk type UNSUPPORTED_OPERATION on ControllerPublishVolume ([#1223](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1223), [@pwschuurman](https://github.com/pwschuurman))

# v1.8.5 - Changelog since v1.8.4

## Changes by Kind

### Bug or Regression

- Add missing libraries, libbsd and libmd, that are dependencies for XFS volume expansion. ([#1204](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1204), [@nberlee](https://github.com/nberlee))

# v1.8.4 - Changelog since v1.8.2


## Changes by Kind

### Other (Cleanup or Flake)

- go version updates ([#1158](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1158), [@saikat-royc](https://github.com/saikat-royc))
- Fix for CVEs - update base image ([#1162](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1162), [@sunnylovestiramisu](https://github.com/sunnylovestiramisu))
- Fix missing shared library libedit.so.2 caused from updating base image in [#1162](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1162) ([#1177](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1177), [@sunnylovestiramisu ](https://github.com/sunnylovestiramisu))

# v1.8.2 - Changelog since v1.8.1


## Changes by Kind

### Other (Cleanup or Flake)

- Update to go 1.19.4 ([#1103](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1103), [@msau42](https://github.com/msau42))
- limit grpc loging info to a configurable char limit ([#1111](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1111), [@leiyiz](https://github.com/leiyiz))
- Upgrade klog v1 to v2 and fix error wrapping & separate user errors from internal errors ([#1115](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1115), [@sunnylovestiramisu](https://github.com/sunnylovestiramisu))
- Add debugging log for the mapping of a PD name to /dev/* path ([#1115](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1115), [@sunnylovestiramisu](https://github.com/sunnylovestiramisu))


# v1.8.1 - Changelog since v1.8.0

## Changes by Kind

### Bug or Regression

- Add udevadm binary in the container image. ([#1097](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1097), [@mattcary](https://github.com/mattcary))

## Dependencies

_Nothing has changed._

# v1.8.0 - Changelog since v.1.7.3

## Changes by Kind

### Feature

- Add support for setting snapshot labels ([#1017](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1017), [@sagor999](https://github.com/sagor999))
- Go builder updated from 1.18.4 to 1.19.1. ([#1048](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1048), [@kon-angelo](https://github.com/kon-angelo))

### Bug or Regression

- Disable devices in node unstage prior to detaching. ([#1051](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1051), [@mattcary](https://github.com/mattcary))
- Enforce implicit pagination limit of 500 of the ListVolumesResponse#Entry field when ListVolumesRequest#max_entries is not set ([#999](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/999), [@pwschuurman](https://github.com/pwschuurman))
- Fixed issue where Regional disks are repeatedly queued for re-attaching and consuming api quota ([#1050](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1050), [@leiyiz](https://github.com/leiyiz))

### Uncategorized

- Migrate from github.com/golang/protobuf to google.golang.org/protobuf ([#1027](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1027), [@leiyiz](https://github.com/leiyiz))
