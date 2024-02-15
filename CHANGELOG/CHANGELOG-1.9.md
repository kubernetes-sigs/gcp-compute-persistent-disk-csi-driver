# v1.9.17 - Changelog since v1.9.16

## Changes by Kind

### Bug

- Change GetDisk error reporting to temporary in CreateVolume codepath ([#1603])https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1603), [@k8s-infra-cherrypick-robot](https://github.com/k8s-infra-cherrypick-robot))


# v1.9.16 - Changelog since v1.9.15

## Changes by Kind

### Uncategorized

- Reduce log spam when identifying NVMe devices located in `/dev` ([#1580](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1580), [@pwschuurman](https://github.com/pwschuurman))

## Dependencies

### Added
_Nothing has changed._

### Changed
_Nothing has changed._

### Removed
_Nothing has changed._


# v1.9.15 - Changelog since v1.9.14

## Changes by Kind

### Uncategorized

- Bump golang.org/x/crypto from v0.14.0 to v0.17.0 to fix CVE-2023-48795 ([#1552](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1552), [@sunnylovestiramisu](https://github.com/sunnylovestiramisu))

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


# v1.9.14 - Changelog since v1.9.13

## Changes by Kind

### Uncategorized

- Properly wrap error from GCE Images.Get() API call, to fix a potential nil-ptr dereference ([#1518](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1518), [@k8s-infra-cherrypick-robot](https://github.com/k8s-infra-cherrypick-robot))
- Update golang builder to 1.20.12 ([#1537](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1537), [@k8s-infra-cherrypick-robot](https://github.com/k8s-infra-cherrypick-robot))

## Dependencies

### Added
_Nothing has changed._

### Changed
_Nothing has changed._

### Removed
_Nothing has changed._


# v1.9.13 - Changelog since v1.9.12

## Changes by Kind

### Bug or Regression

- Bump Golang Builder version to 1.20.11 ([#1505](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1505), [@uriel-guzman](https://github.com/uriel-guzman))
- Bump google.golang.org/grpc from v1.53.0 to v1.56.3 to fix CVE-2023-44487. ([#1495](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1495), [@uriel-guzman](https://github.com/uriel-guzman))

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

# v1.9.12 - Changelog since v1.9.10

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

# v1.9.10 - Changelog since v1.9.9

## Changes by Kind

### Bug or Regression

- bump go version to 1.20.8 ([#1394](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1394), [@tyuchn](https://github.com/tyuchn))
- Remove ARG BUILDPLATFORM from Dockerfile ([#1385](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1385), [@tyuchn](https://github.com/tyuchn))
- Update test/run-e2e.sh to match PROW configuration ([#1360](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1360), [@pwschuurman](https://github.com/pwschuurman))
- Always call LoggedError for errors returned from CloudProvider methods ([#1381](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1381), [@amacaskill](https://github.com/amacaskill))

# v1.9.9 - Changelog since v1.9.8

## Changes by Kind

### Bug or Regression

- Add option for serializing formatAndMount, including fsck as well as mkfs. ([#1352](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1352), [@mattcary](https://github.com/mattcary))

### Uncategorized

- Update go version to 1.20.7 to fix CVE-2023-29409 CVE-2023-39533 ([#1349](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1349), [@k8s-infra-cherrypick-robot](https://github.com/k8s-infra-cherrypick-robot))

# v1.9.8 - Changelog since v1.9.7

## Changes by Kind

### Feature

- Added support in PDCSI driver to create confidential hyperdisk storage on GCE. ([#1318](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1318), [@Sneha-at](https://github.com/Sneha-at))

### Bug or Regression

- Update go version to 1.20.6 to fix CVE-2023-29406 ([#1330](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1330), [@pwschuurman](https://github.com/pwschuurman))

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
- github.com/census-instrumentation/opencensus-proto: [v0.2.1 → v0.4.1](https://github.com/census-instrumentation/opencensus-proto/compare/v0.2.1...v0.4.1)
- github.com/cespare/xxhash/v2: [v2.1.2 → v2.2.0](https://github.com/cespare/xxhash/v2/compare/v2.1.2...v2.2.0)
- github.com/cncf/udpa/go: [04548b0 → c52dc94](https://github.com/cncf/udpa/go/compare/04548b0...c52dc94)
- github.com/cncf/xds/go: [cb28da3 → 06c439d](https://github.com/cncf/xds/go/compare/cb28da3...06c439d)
- github.com/envoyproxy/go-control-plane: [49ff273 → v0.10.3](https://github.com/envoyproxy/go-control-plane/compare/49ff273...v0.10.3)
- github.com/envoyproxy/protoc-gen-validate: [v0.1.0 → v0.9.1](https://github.com/envoyproxy/protoc-gen-validate/compare/v0.1.0...v0.9.1)
- github.com/googleapis/enterprise-certificate-proxy: [v0.1.0 → v0.2.3](https://github.com/googleapis/enterprise-certificate-proxy/compare/v0.1.0...v0.2.3)
- github.com/googleapis/gax-go/v2: [v2.4.0 → v2.7.0](https://github.com/googleapis/gax-go/v2/compare/v2.4.0...v2.7.0)
- github.com/stretchr/objx: [v0.2.0 → v0.5.0](https://github.com/stretchr/objx/compare/v0.2.0...v0.5.0)
- github.com/stretchr/testify: [v1.7.0 → v1.8.1](https://github.com/stretchr/testify/compare/v1.7.0...v1.8.1)
- go.opencensus.io: v0.23.0 → v0.24.0
- golang.org/x/net: v0.5.0 → v0.7.0
- golang.org/x/oauth2: 128564f → v0.5.0
- golang.org/x/sync: 0de741c → v0.1.0
- golang.org/x/sys: v0.4.0 → v0.5.0
- golang.org/x/term: v0.4.0 → v0.5.0
- golang.org/x/text: v0.6.0 → v0.7.0
- golang.org/x/xerrors: 65e6541 → 5ec99f8
- google.golang.org/api: v0.86.0 → v0.111.0
- google.golang.org/genproto: 176da50 → 637eb22
- google.golang.org/grpc: v1.48.0 → v1.53.0
- google.golang.org/protobuf: v1.28.0 → v1.28.1

### Removed
- github.com/googleapis/go-type-adapters: [v1.0.0](https://github.com/googleapis/go-type-adapters/tree/v1.0.0)

# v1.9.7 - Changelog since v1.9.6

## Changes by Kind

### Bug or Regression

- Fix resource parsing when the gcp project name ends with alpha, beta or v1 ([#1308](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1308), [@mattcary](https://github.com/mattcary))

### Uncategorized

- Add disk type for all operations metrics. ([#1296](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1296), [@k8s-infra-cherrypick-robot](https://github.com/k8s-infra-cherrypick-robot))
- Use original error code when responding with a backoff error on publish or unpublish. ([#1312](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1312), [@k8s-infra-cherrypick-robot](https://github.com/k8s-infra-cherrypick-robot))

# v1.9.6 - Changelog since v1.9.5

## Changes by Kind

### Bug or Regression

- Fix provisioned-iops-on-create passing logic([#1283](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1283))
- Bugfix for empty disk type being registered in metric for Create volume function. ([#1268](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1268), [@k8s-infra-cherrypick-robot](https://github.com/k8s-infra-cherrypick-robot))

### Cleanup

- Automated cherry pick of #1150: satisfy volume cloning topology requirements when choosing zone for CreateVolume, #1232: Use errors.As so we can detect wrapped errors, #1227: Adding new metric pdcsi_operation_errors to fetch error ([#1240](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1240), [@sunnylovestiramisu](https://github.com/sunnylovestiramisu))
- Update go version to 1.20.5 to address CVE fixes ([#1266](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1266), [@k8s-infra-cherrypick-robot](https://github.com/k8s-infra-cherrypick-robot))
- Updates error message to be more user friendly when PD CSI Driver encounters an disk type UNSUPPORTED_OPERATION on ControllerPublishVolume ([#1222](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1222), [@pwschuurman](https://github.com/pwschuurman))
- [release-1.10] Update Docker.Windows to 1.20.5 ([#1274](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1274), [@k8s-infra-cherrypick-robot](https://github.com/k8s-infra-cherrypick-robot))

# v1.9.5 - Changelog since v1.9.4

### Bug or Regression

- Add libraries needed for determining XFS volume expansion ([#1204](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1204), [@nberlee](https://github.com/nberlee))

### Cleanup

- Updates error message to be more user friendly when PD CSI Driver encounters an disk type UNSUPPORTED_OPERATION on ControllerPublishVolume ([#1222](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1222), [@pwschuurman](https://github.com/pwschuurman))

# v1.9.4 - Changelog since v1.9.3

## Changes by Kind

### Bug or Regression

- Add missing libraries, libbsd and libmd, that are dependencies for XFS volume expansion. ([#1204](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1204), [@nberlee](https://github.com/nberlee))

# v1.9.3 - Changelog since v1.9.2

## Changes by Kind

### Bug or Regression

- Fix missing libedit.so.2 error ([#1177](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1177), [@sunnylovestiramisu](https://github.com/sunnylovestiramisu))
- Update go version to 1.20.3 for k/k 1.27 ([#1180](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1180), [@sunnylovestiramisu](https://github.com/sunnylovestiramisu))

# v1.9.2 - Changelog since v1.9.1

## Changes by Kind

### Bug or Regression

- Revert feature to add PV, PVC and namespace name as labels to the PD [#1090](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1090) ([#1174](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1174), [@sunnylovestiramisu](https://github.com/sunnylovestiramisu))


# v1.9.1 - Changelog since v1.9.0

## Changes by Kind

### Feature

- go version updates ([#1158](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1158), [@saikat-royc](https://github.com/saikat-royc))

- Fix for CVEs - update base image ([#1162](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1162), [@sunnylovestiramisu](https://github.com/sunnylovestiramisu))

- Fix multiarch build ([#1165](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1165), [@sunnylovestiramisu](https://github.com/sunnylovestiramisu))

# v1.9.0 - Changelog since v1.8.2

## Changes by Kind

### Feature

- Adding auto-stamped details like PV, PVC and namespace name from PV description as labels for the PVs/PDs ([#1090](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1090), [@Sneha-at](https://github.com/Sneha-at))
- Support setting snapshot labels to Images ([#1066](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1066), [@jenting](https://github.com/jenting))
- Pass in ProvisionedIOPSOnCreate as a parameter for CreateVolume ([#1079](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1079), [@sunnylovestiramisu](https://github.com/sunnylovestiramisu))
- Add provisionedThroughput for hyperdisk ([#1101](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1101), [@sunnylovestiramisu](https://github.com/sunnylovestiramisu))
- Change iops params directly convert string to int64 ([#1128](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1128), [@sunnylovestiramisu](https://github.com/sunnylovestiramisu))

### Bug or Regression

- Fix the filesystem not being resized when restoring from a snapshot/clone to a larger size than the original ([#972](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/972), [@mattcary](https://github.com/mattcary))
- Add udevadm binary in the container image. ([#1072](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1072), [@jenting](https://github.com/jenting))
- Remove debug.PrintStack() ([#1135](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1135), [@sunnylovestiramisu](https://github.com/sunnylovestiramisu))

### Other (Cleanup or Flake)

- Improve logging for device path verification ([#1142](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1142), [@saikat-royc](https://github.com/saikat-royc))
- Update csi-attacher to v4.2.0 ([#1144](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1144), [@sunnylovestiramisu](https://github.com/sunnylovestiramisu))
- Update sidecar based on internal versions ([#1154](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1154), [@sunnylovestiramisu](https://github.com/sunnylovestiramisu))

## Dependencies

### Changed
- github.com/go-logr/logr: [v1.2.0 → v1.2.3](https://github.com/go-logr/logr/compare/v1.2.0...v1.2.3)
- github.com/google/go-cmp: [v0.5.8 → v0.5.9](https://github.com/google/go-cmp/compare/v0.5.8...v0.5.9)
- github.com/onsi/ginkgo/v2: [v2.1.4 → v2.7.1](https://github.com/onsi/ginkgo/v2/compare/v2.1.4...v2.7.1)
- github.com/onsi/gomega: [v1.20.0 → v1.25.0](https://github.com/onsi/gomega/compare/v1.20.0...v1.25.0)
- golang.org/x/mod: 9b9b3d8 → 86c51ed
- golang.org/x/net: a158d28 → v0.5.0
- golang.org/x/sys: 8c9f86f → v0.4.0
- golang.org/x/term: 03fcf44 → v0.4.0
- golang.org/x/text: v0.3.7 → v0.6.0
- golang.org/x/tools: 897bd77 → v0.5.0

