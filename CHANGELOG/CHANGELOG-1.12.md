# v1.12.6 - Changelog since v1.12.5

## Changes by Kind

### Uncategorized

- Bump golang.org/x/crypto from v0.15.0 to v0.17.0 to fix CVE-2023-48795 ([#1550](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1550), [@sunnylovestiramisu](https://github.com/sunnylovestiramisu))

## Dependencies

### Added
_Nothing has changed._

### Changed
- golang.org/x/crypto: v0.14.0 → v0.17.0
- golang.org/x/sys: v0.13.0 → v0.15.0
- golang.org/x/term: v0.13.0 → v0.15.0
- golang.org/x/text: v0.13.0 → v0.14.0

### Removed
_Nothing has changed._


# v1.12.5 - Changelog since v1.12.4

## Changes by Kind

### Other (Cleanup or Flake)

- Update golang builder to 1.20.12 ([#1536](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1536), [@msau42](https://github.com/msau42))

### Uncategorized

- Add --fallback-requisite-zones flag to allow disk provisioning to fallback to a default set of zones when there are an insufficient number of zones available in a passed in requisite topology in CreateVolume. ([#1542](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1542), [@k8s-infra-cherrypick-robot](https://github.com/k8s-infra-cherrypick-robot))
- Properly wrap error from GCE Images.Get() API call, to fix a potential nil-ptr dereference ([#1515](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1515), [@k8s-infra-cherrypick-robot](https://github.com/k8s-infra-cherrypick-robot))

## Dependencies

### Added
_Nothing has changed._

### Changed
_Nothing has changed._

### Removed
_Nothing has changed._

# v1.12.4 - Changelog since v1.12.3

## Changes by Kind

### Uncategorized

- Properly wrap error from GCE Images.Get() API call, to fix a potential nil-ptr dereference ([#1515](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1515), [@k8s-infra-cherrypick-robot](https://github.com/k8s-infra-cherrypick-robot))

## Dependencies

### Added
_Nothing has changed._

### Changed
_Nothing has changed._

### Removed
_Nothing has changed._

# v1.12.3 - Changelog since v1.12.2

## Changes by Kind

### Bug or Regression

- Bump Golang Builder version to 1.20.11 ([#1502](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1502), [@uriel-guzman](https://github.com/uriel-guzman))
- Bump google.golang.org/grpc from v1.56.2 to v1.56.3 to fix CVE-2023-44487. ([#1492](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1492), [@uriel-guzman](https://github.com/uriel-guzman))

### Uncategorized

- Reduce log spam when identifying NVMe devices located in `/dev` ([#1487](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1487), [@pwschuurman](https://github.com/pwschuurman))
- The benign error when DisableDevice is not effective is logged as a warning. ([#1468](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1468), [@k8s-infra-cherrypick-robot](https://github.com/k8s-infra-cherrypick-robot))
- Update go version to 1.20.10 ([#1460](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1460), [@k8s-infra-cherrypick-robot](https://github.com/k8s-infra-cherrypick-robot))

## Dependencies

### Added
_Nothing has changed._

### Changed
- google.golang.org/grpc: v1.56.2 → v1.56.3

### Removed
_Nothing has changed._
e.com/go/area120: v0.6.0 → v0.8.1
- cloud.google.com/go/artifactregistry: v1.9.0 → v1.14.1
- cloud.google.com/go/asset: v1.10.0 → v1.14.1
- cloud.google.com/go/assuredworkloads: v1.9.0 → v1.11.1
- cloud.google.com/go/automl: v1.8.0 → v1.13.1
- cloud.google.com/go/baremetalsolution: v0.4.0 → v0.5.0
- cloud.google.com/go/batch: v0.4.0 → v0.7.0
- cloud.google.com/go/beyondcorp: v0.3.0 → v0.6.1
- cloud.google.com/go/bigquery: v1.44.0 → v1.52.0
- cloud.google.com/go/billing: v1.7.0 → v1.16.0
- cloud.google.com/go/binaryauthorization: v1.4.0 → v1.6.1
- cloud.google.com/go/certificatemanager: v1.4.0 → v1.7.1
- cloud.google.com/go/channel: v1.9.0 → v1.16.0
- cloud.google.com/go/cloudbuild: v1.4.0 → v1.10.1
- cloud.google.com/go/clouddms: v1.4.0 → v1.6.1
- cloud.google.com/go/cloudtasks: v1.8.0 → v1.11.1
- cloud.google.com/go/compute: v1.18.0 → v1.20.1
- cloud.google.com/go/contactcenterinsights: v1.4.0 → v1.9.1
- cloud.google.com/go/container: v1.7.0 → v1.22.1
- cloud.google.com/go/containeranalysis: v0.6.0 → v0.10.1
- cloud.google.com/go/datacatalog: v1.8.0 → v1.14.1
- cloud.google.com/go/dataflow: v0.7.0 → v0.9.1
- cloud.google.com/go/dataform: v0.5.0 → v0.8.1
- cloud.google.com/go/datafusion: v1.5.0 → v1.7.1
- cloud.google.com/go/datalabeling: v0.6.0 → v0.8.1
- cloud.google.com/go/dataplex: v1.4.0 → v1.8.1
- cloud.google.com/go/dataproc: v1.8.0 → v1.12.0
- cloud.google.com/go/dataqna: v0.6.0 → v0.8.1
- cloud.google.com/go/datastore: v1.10.0 → v1.12.0
- cloud.google.com/go/datastream: v1.5.0 → v1.9.1
- cloud.google.com/go/deploy: v1.5.0 → v1.11.0
- cloud.google.com/go/dialogflow: v1.29.0 → v1.38.0
- cloud.google.com/go/dlp: v1.7.0 → v1.10.1
- cloud.google.com/go/documentai: v1.10.0 → v1.20.0
- cloud.google.com/go/domains: v0.7.0 → v0.9.1
- cloud.google.com/go/edgecontainer: v0.2.0 → v1.1.1
- cloud.google.com/go/essentialcontacts: v1.4.0 → v1.6.2
- cloud.google.com/go/eventarc: v1.8.0 → v1.12.1
- cloud.google.com/go/filestore: v1.4.0 → v1.7.1
- cloud.google.com/go/firestore: v1.9.0 → v1.11.0
- cloud.google.com/go/functions: v1.9.0 → v1.15.1
- cloud.google.com/go/gaming: v1.8.0 → v1.10.1
- cloud.google.com/go/gkebackup: v0.3.0 → v0.4.0
- cloud.google.com/go/gkeconnect: v0.6.0 → v0.8.1
- cloud.google.com/go/gkehub: v0.10.0 → v0.14.1
- cloud.google.com/go/gkemulticloud: v0.4.0 → v0.6.1
- cloud.google.com/go/gsuiteaddons: v1.4.0 → v1.6.1
- cloud.google.com/go/iam: v0.11.0 → v1.1.0
- cloud.google.com/go/iap: v1.5.0 → v1.8.1
- cloud.google.com/go/ids: v1.2.0 → v1.4.1
- cloud.google.com/go/iot: v1.4.0 → v1.7.1
- cloud.google.com/go/kms: v1.6.0 → v1.12.1
- cloud.google.com/go/language: v1.8.0 → v1.10.1
- cloud.google.com/go/lifesciences: v0.6.0 → v0.9.1
- cloud.google.com/go/logging: v1.6.1 → v1.7.0
- cloud.google.com/go/longrunning: v0.3.0 → v0.5.1
- cloud.google.com/go/managedidentities: v1.4.0 → v1.6.1
- cloud.google.com/go/maps: v0.1.0 → v0.7.0
- cloud.google.com/go/mediatranslation: v0.6.0 → v0.8.1
- cloud.google.com/go/memcache: v1.7.0 → v1.10.1
- cloud.google.com/go/metastore: v1.8.0 → v1.11.1
- cloud.google.com/go/monitoring: v1.8.0 → v1.15.1
- cloud.google.com/go/networkconnectivity: v1.7.0 → v1.12.1
- cloud.google.com/go/networkmanagement: v1.5.0 → v1.8.0
- cloud.google.com/go/networksecurity: v0.6.0 → v0.9.1
- cloud.google.com/go/notebooks: v1.5.0 → v1.9.1
- cloud.google.com/go/optimization: v1.2.0 → v1.4.1
- cloud.google.com/go/orchestration: v1.4.0 → v1.8.1
- cloud.google.com/go/orgpolicy: v1.5.0 → v1.11.1
- cloud.google.com/go/osconfig: v1.10.0 → v1.12.1
- cloud.google.com/go/oslogin: v1.7.0 → v1.10.1
- cloud.google.com/go/phishingprotection: v0.6.0 → v0.8.1
- cloud.google.com/go/policytroubleshooter: v1.4.0 → v1.7.1
- cloud.google.com/go/privatecatalog: v0.6.0 → v0.9.1
- cloud.google.com/go/pubsub: v1.27.1 → v1.32.0
- cloud.google.com/go/pubsublite: v1.5.0 → v1.8.1
- cloud.google.com/go/recaptchaenterprise/v2: v2.5.0 → v2.7.2
- cloud.google.com/go/recommendationengine: v0.6.0 → v0.8.1
- cloud.google.com/go/recommender: v1.8.0 → v1.10.1
- cloud.google.com/go/redis: v1.10.0 → v1.13.1
- cloud.google.com/go/resourcemanager: v1.4.0 → v1.9.1
- cloud.google.com/go/resourcesettings: v1.4.0 → v1.6.1
- cloud.google.com/go/retail: v1.11.0 → v1.14.1
- cloud.google.com/go/run: v0.3.0 → v0.9.0
- cloud.google.com/go/scheduler: v1.7.0 → v1.10.1
- cloud.google.com/go/secretmanager: v1.9.0 → v1.11.1
- cloud.google.com/go/security: v1.10.0 → v1.15.1
- cloud.google.com/go/securitycenter: v1.16.0 → v1.23.0
- cloud.google.com/go/servicedirectory: v1.7.0 → v1.10.1
- cloud.google.com/go/shell: v1.4.0 → v1.7.1
- cloud.google.com/go/spanner: v1.41.0 → v1.47.0
- cloud.google.com/go/speech: v1.9.0 → v1.17.1
- cloud.google.com/go/storagetransfer: v1.6.0 → v1.10.0
- cloud.google.com/go/talent: v1.4.0 → v1.6.2
- cloud.google.com/go/texttospeech: v1.5.0 → v1.7.1
- cloud.google.com/go/tpu: v1.4.0 → v1.6.1
- cloud.google.com/go/trace: v1.4.0 → v1.10.1
- cloud.google.com/go/translate: v1.4.0 → v1.8.1
- cloud.google.com/go/video: v1.9.0 → v1.17.1
- cloud.google.com/go/videointelligence: v1.9.0 → v1.11.1
- cloud.google.com/go/vision/v2: v2.5.0 → v2.7.2
- cloud.google.com/go/vmmigration: v1.3.0 → v1.7.1
- cloud.google.com/go/vmwareengine: v0.1.0 → v0.4.1
- cloud.google.com/go/vpcaccess: v1.5.0 → v1.7.1
- cloud.google.com/go/webrisk: v1.7.0 → v1.9.1
- cloud.google.com/go/websecurityscanner: v1.4.0 → v1.6.1
- cloud.google.com/go/workflows: v1.9.0 → v1.11.1
- cloud.google.com/go: v0.107.0 → v0.110.4
- github.com/cncf/xds/go: [06c439d → e9ce688](https://github.com/cncf/xds/go/compare/06c439d...e9ce688)
- github.com/envoyproxy/go-control-plane: [v0.10.3 → 9239064](https://github.com/envoyproxy/go-control-plane/compare/v0.10.3...9239064)
- github.com/envoyproxy/protoc-gen-validate: [v0.9.1 → v0.10.1](https://github.com/envoyproxy/protoc-gen-validate/compare/v0.9.1...v0.10.1)
- github.com/golang/glog: [v1.0.0 → v1.1.0](https://github.com/golang/glog/compare/v1.0.0...v1.1.0)
- github.com/golang/protobuf: [v1.5.2 → v1.5.3](https://github.com/golang/protobuf/compare/v1.5.2...v1.5.3)
- github.com/googleapis/enterprise-certificate-proxy: [v0.2.3 → v0.2.5](https://github.com/googleapis/enterprise-certificate-proxy/compare/v0.2.3...v0.2.5)
- github.com/googleapis/gax-go/v2: [v2.7.0 → v2.12.0](https://github.com/googleapis/gax-go/v2/compare/v2.7.0...v2.12.0)
- github.com/yuin/goldmark: [v1.4.1 → v1.4.13](https://github.com/yuin/goldmark/compare/v1.4.1...v1.4.13)
- golang.org/x/oauth2: v0.5.0 → v0.10.0
- golang.org/x/sync: v0.1.0 → v0.3.0
- google.golang.org/api: v0.111.0 → v0.134.0
- google.golang.org/genproto: 637eb22 → ccb25ca
- google.golang.org/grpc: v1.53.0 → v1.56.3
- google.golang.org/protobuf: v1.28.1 → v1.31.0

### Removed
- cloud.google.com/go/apikeys: v0.4.0
- cloud.google.com/go/servicecontrol: v1.5.0
- cloud.google.com/go/servicemanagement: v1.5.0
- cloud.google.com/go/serviceusage: v1.4.0

# v1.12.2 - Changelog since v1.12.0

## Changes by Kind

### Bug or Regression

- CVE fixes: CVE-2023-39323 ([#1412](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1412), [@dannawang0221](https://github.com/dannawang0221))

### Other (Cleanup or Flake)

- Update go version to 1.20.10 ([#1453](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1453), [@tyuchn](https://github.com/tyuchn))

## Dependencies

### Changed

- golang.org/x/crypto: v0.11.0 → v0.14.0
- golang.org/x/net: v0.12.0 → v0.17.0
- golang.org/x/sys: v0.10.0 → v0.13.0
- golang.org/x/term: v0.10.0 → v0.13.0
- golang.org/x/text: v0.11.0 → v0.13.0

# v1.12.0 - Changelog since v1.11.1

## Changes by Kind

### Uncategorized

- Build and publish arm64 images ([#1369](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1369), [@upodroid](https://github.com/upodroid))

## Dependencies

### Added

_Nothing has changed._

### Changed

_Nothing has changed._

### Removed

_Nothing has changed._
