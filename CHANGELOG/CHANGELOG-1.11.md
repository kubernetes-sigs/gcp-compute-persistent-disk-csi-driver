# v1.11.9 - Changelog since v1.11.8

## Changes by Kind

### Bug

- Change GetDisk error reporting to temporary in CreateVolume codepath ([#1601])https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1601), [@k8s-infra-cherrypick-robot](https://github.com/k8s-infra-cherrypick-robot))

# v1.11.8 - Changelog since v1.11.7

## Changes by Kind

### Uncategorized

- Bump golang.org/x/crypto from v0.14.0 to v0.17.0 to fix CVE-2023-48795 ([#1555](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1555), [@sunnylovestiramisu](https://github.com/sunnylovestiramisu))

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


# v1.11.7 - Changelog since v1.11.6

## Changes by Kind

### Uncategorized

- Update golang builder to 1.20.12 ([#1540](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1540), [@k8s-infra-cherrypick-robot](https://github.com/k8s-infra-cherrypick-robot))

## Dependencies

### Added
_Nothing has changed._

### Changed
_Nothing has changed._

### Removed
_Nothing has changed._

# v1.11.6 - Changelog since v1.11.5

## Changes by Kind

### Uncategorized

- Properly wrap error from GCE Images.Get() API call, to fix a potential nil-ptr dereference ([#1516](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1516), [@k8s-infra-cherrypick-robot](https://github.com/k8s-infra-cherrypick-robot))

## Dependencies

### Added
_Nothing has changed._

### Changed
_Nothing has changed._

### Removed
_Nothing has changed._

# v1.11.5 - Changelog since v1.11.4

## Changes by Kind

### Bug or Regression

- Bump Golang Builder version to 1.20.11 ([#1507](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1507), [@uriel-guzman](https://github.com/uriel-guzman))
- Bump google.golang.org/grpc from v1.56.2 to v1.56.3 to fix CVE-2023-44487. ([#1491](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1491), [@uriel-guzman](https://github.com/uriel-guzman))

### Uncategorized

- Reduce log spam when identifying NVMe devices located in `/dev` ([#1488](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1488), [@pwschuurman](https://github.com/pwschuurman))
- The benign error when DisableDevice is not effective is logged as a warning. ([#1469](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1469), [@k8s-infra-cherrypick-robot](https://github.com/k8s-infra-cherrypick-robot))

## Dependencies

### Added
_Nothing has changed._

### Changed
- google.golang.org/grpc: v1.56.2 → v1.56.3

### Removed
_Nothing has changed._
pigateway: v1.4.0 → v1.6.1
- cloud.google.com/go/apigeeconnect: v1.4.0 → v1.6.1
- cloud.google.com/go/apigeeregistry: v0.4.0 → v0.7.1
- cloud.google.com/go/appengine: v1.5.0 → v1.8.1
- cloud.google.com/go/area120: v0.6.0 → v0.8.1
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

# v1.11.4 - Changelog since v1.11.2

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

# v1.11.2 - Changelog since v1.11.1
## Changes by Kind

### Bug or Regression

- bump go version to 1.20.8 ([#1394](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1394), [@tyuchn](https://github.com/tyuchn))
- Remove ARG BUILDPLATFORM from Dockerfile ([#1385](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1385), [@tyuchn](https://github.com/tyuchn))
- Update test/run-e2e.sh to match PROW configuration ([#1360](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1360), [@pwschuurman](https://github.com/pwschuurman))

# v1.11.1 - Changelog since v1.11.0

## Changes by Kind

### Bug or Regression

- Update go version to 1.20.7 to fix CVE-2023-29409 CVE-2023-39533 ([#1347](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1347), [@Sneha-at](https://github.com/Sneha-at))

### Uncategorized

- Fix zone specification in HdT tests ([#1341](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1341), [@k8s-infra-cherrypick-robot](https://github.com/k8s-infra-cherrypick-robot))

# v1.11.0 - Changelog since v1.10.5

## Changes by Kind

### Bug or Regression

- Update go version to 1.20.6 to fix CVE-2023-29406 ([#1330](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1330), [@Sneha-at](https://github.com/Sneha-at))

### Other (Cleanup or Flake)

- Change e2e test machine type to n2-standard-2 to include more Hyperdisk cases ([#1320](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1320), [@sunnylovestiramisu](https://github.com/sunnylovestiramisu))
- Update k8s-cloud-provider to v1.24.0 and add HdT e2e tests ([#1325](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1325), [@sunnylovestiramisu](https://github.com/sunnylovestiramisu))

## Dependencies

### Added
- github.com/google/go-pkcs11: [v0.2.0](https://github.com/google/go-pkcs11/tree/v0.2.0)
- github.com/google/s2a-go: [v0.1.4](https://github.com/google/s2a-go/tree/v0.1.4)
- google.golang.org/genproto/googleapis/api: ccb25ca
- google.golang.org/genproto/googleapis/bytestream: 659f7aa
- google.golang.org/genproto/googleapis/rpc: 659f7aa

### Changed
- cloud.google.com/go/accessapproval: v1.5.0 → v1.7.1
- cloud.google.com/go/accesscontextmanager: v1.4.0 → v1.8.1
- cloud.google.com/go/aiplatform: v1.27.0 → v1.45.0
- cloud.google.com/go/analytics: v0.12.0 → v0.21.2
- cloud.google.com/go/apigateway: v1.4.0 → v1.6.1
- cloud.google.com/go/apigeeconnect: v1.4.0 → v1.6.1
- cloud.google.com/go/apigeeregistry: v0.4.0 → v0.7.1
- cloud.google.com/go/appengine: v1.5.0 → v1.8.1
- cloud.google.com/go/area120: v0.6.0 → v0.8.1
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
- github.com/GoogleCloudPlatform/k8s-cloud-provider: [v1.18.0 → v1.24.0](https://github.com/GoogleCloudPlatform/k8s-cloud-provider/compare/v1.18.0...v1.24.0)
- github.com/cncf/xds/go: [06c439d → e9ce688](https://github.com/cncf/xds/go/compare/06c439d...e9ce688)
- github.com/envoyproxy/go-control-plane: [v0.10.3 → 9239064](https://github.com/envoyproxy/go-control-plane/compare/v0.10.3...9239064)
- github.com/envoyproxy/protoc-gen-validate: [v0.9.1 → v0.10.1](https://github.com/envoyproxy/protoc-gen-validate/compare/v0.9.1...v0.10.1)
- github.com/golang/glog: [v1.0.0 → v1.1.0](https://github.com/golang/glog/compare/v1.0.0...v1.1.0)
- github.com/golang/protobuf: [v1.5.2 → v1.5.3](https://github.com/golang/protobuf/compare/v1.5.2...v1.5.3)
- github.com/golang/snappy: [v0.0.3 → v0.0.1](https://github.com/golang/snappy/compare/v0.0.3...v0.0.1)
- github.com/google/martian/v3: [v3.2.1 → v3.1.0](https://github.com/google/martian/v3/compare/v3.2.1...v3.1.0)
- github.com/google/pprof: [4bb14d4 → 94a9f03](https://github.com/google/pprof/compare/4bb14d4...94a9f03)
- github.com/googleapis/enterprise-certificate-proxy: [v0.2.3 → v0.2.5](https://github.com/googleapis/enterprise-certificate-proxy/compare/v0.2.3...v0.2.5)
- github.com/googleapis/gax-go/v2: [v2.7.0 → v2.12.0](https://github.com/googleapis/gax-go/v2/compare/v2.7.0...v2.12.0)
- github.com/rogpeppe/go-internal: [v1.5.2 → v1.9.0](https://github.com/rogpeppe/go-internal/compare/v1.5.2...v1.9.0)
- github.com/yuin/goldmark: [v1.4.1 → v1.4.13](https://github.com/yuin/goldmark/compare/v1.4.1...v1.4.13)
- golang.org/x/crypto: 8634188 → v0.11.0
- golang.org/x/mod: 86c51ed → v0.8.0
- golang.org/x/net: v0.7.0 → v0.12.0
- golang.org/x/oauth2: v0.5.0 → v0.10.0
- golang.org/x/sync: v0.1.0 → v0.3.0
- golang.org/x/sys: v0.5.0 → v0.10.0
- golang.org/x/term: v0.5.0 → v0.10.0
- golang.org/x/text: v0.7.0 → v0.11.0
- golang.org/x/tools: v0.5.0 → v0.6.0
- google.golang.org/api: v0.111.0 → v0.134.0
- google.golang.org/genproto: 637eb22 → ccb25ca
- google.golang.org/grpc: v1.53.0 → v1.56.2
- google.golang.org/protobuf: v1.28.1 → v1.31.0

### Removed
- cloud.google.com/go/apikeys: v0.4.0
- cloud.google.com/go/servicecontrol: v1.5.0
- cloud.google.com/go/servicemanagement: v1.5.0
- cloud.google.com/go/serviceusage: v1.4.0
- google.golang.org/grpc/cmd/protoc-gen-go-grpc: v1.1.0
