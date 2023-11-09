# v1.7.18 - Changelog since v1.7.17

## Changes by Kind

### Bug or Regression

- Bump Golang Builder version to 1.20.11 ([#1503](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1503), [@uriel-guzman](https://github.com/uriel-guzman))
- Bump google.golang.org/grpc from v1.55.1 to v1.56.3 to fix CVE-2023-44487. ([#1497](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1497), [@uriel-guzman](https://github.com/uriel-guzman))

## Dependencies

### Added
_Nothing has changed._

### Changed
- github.com/cncf/xds/go: [32f1caf → e9ce688](https://github.com/cncf/xds/go/compare/32f1caf...e9ce688)
- github.com/envoyproxy/go-control-plane: [v0.11.0 → 9239064](https://github.com/envoyproxy/go-control-plane/compare/v0.11.0...9239064)
- github.com/envoyproxy/protoc-gen-validate: [v0.10.0 → v0.10.1](https://github.com/envoyproxy/protoc-gen-validate/compare/v0.10.0...v0.10.1)
- google.golang.org/grpc: v1.55.1 → v1.56.3

### Removed
_Nothing has changed._

# v1.7.17 - Changelog since v1.7.15

## Changes by Kind

### Bug or Regression

- CVE fixes: CVE-2023-39323 ([#1412](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1412), [@dannawang0221](https://github.com/dannawang0221))

### Other (Cleanup or Flake)

- Update go version to 1.20.10 ([#1453](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1453), [@tyuchn](https://github.com/tyuchn))

## Dependencies

### Added

_Nothing has changed._

### Changed

- golang.org/x/crypto: v0.9.0 → v0.14.0
- golang.org/x/net: v0.10.0 → v0.17.0
- golang.org/x/sys: v0.8.0 → v0.13.0
- golang.org/x/term: v0.8.0 → v0.13.0
- golang.org/x/text: v0.9.0 → v0.13.0

# v1.7.15 - Changelog since v1.7.13

## Changes by Kind

### Bug or Regression

- bump go version to 1.20.8 ([#1394](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1394), [@tyuchn](https://github.com/tyuchn))
- Remove ARG BUILDPLATFORM from Dockerfile ([#1385](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1385), [@tyuchn](https://github.com/tyuchn))

# v1.7.13 - Changelog since v1.7.12

## Changes by Kind

### Bug or Regression

- Upgrade google.golang.org/grpc from v1.55.0 -> v1.55.1 to address https://github.com/grpc/grpc-go/issues/6373 ([#1373](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1373), [@pwschuurman](https://github.com/pwschuurman))

# v1.7.12 - Changelog since v1.7.11

## Changes by Kind

### Bug or Regression

- Update go version to 1.19.12 to fix CVE-2023-29409 CVE-2023-39533 ([#1351](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1351), [@k8s-infra-cherrypick-robot](https://github.com/k8s-infra-cherrypick-robot))

# v1.7.11 - Changelog since v1.7.10

## Changes by Kind

### Bug or Regression

- Update go version to 1.19.11 to fix CVE-2023-29406 ([#1335](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1335), [@Sneha-at](https://github.com/Sneha-at))

### Uncategorized

- #1101: Add provisionedThroughput for hyperdisk
  #1227: Adding new metric pdcsi_operation_errors to fetch error
  #1296: emit metrics even for success scenarios ([#1305](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1305), [@sunnylovestiramisu](https://github.com/sunnylovestiramisu))
- Use errors.As so we can detect wrapped errors, and check for existing error codes in CodesForError ([#1326](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1326), [@judemars](https://github.com/judemars))

# v1.7.10 - Changelog since v1.7.9

## Changes by Kind

### Feature

- #1101: Add provisionedThroughput for hyperdisk
  #1227: Adding new metric pdcsi_operation_errors to fetch error
  #1296: emit metrics even for success scenarios ([#1305](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1305), [@sunnylovestiramisu](https://github.com/sunnylovestiramisu))
- #1150: satisfy volume cloning topology requirements when choosing zone for CreateVolume
  #1079: Add provisionedIops for pd-extreme
  #1128: Change iops params directly convert string to int64 ([#1243](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1243), [@sunnylovestiramisu](https://github.com/sunnylovestiramisu))

### Other (Cleanup or Flake)

- Updates error message to be more user friendly when PD CSI Driver encounters an disk type UNSUPPORTED_OPERATION on ControllerPublishVolume ([#1224](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1224), [@pwschuurman](https://github.com/pwschuurman))

## Dependencies

### Added

- bitbucket.org/creachadair/stringset: v0.0.9
- cloud.google.com/go/accessapproval: v1.6.0
- cloud.google.com/go/accesscontextmanager: v1.7.0
- cloud.google.com/go/aiplatform: v1.37.0
- cloud.google.com/go/analytics: v0.19.0
- cloud.google.com/go/apigateway: v1.5.0
- cloud.google.com/go/apigeeconnect: v1.5.0
- cloud.google.com/go/apigeeregistry: v0.6.0
- cloud.google.com/go/appengine: v1.7.1
- cloud.google.com/go/area120: v0.7.1
- cloud.google.com/go/artifactregistry: v1.13.0
- cloud.google.com/go/asset: v1.13.0
- cloud.google.com/go/assuredworkloads: v1.10.0
- cloud.google.com/go/automl: v1.12.0
- cloud.google.com/go/baremetalsolution: v0.5.0
- cloud.google.com/go/batch: v0.7.0
- cloud.google.com/go/beyondcorp: v0.5.0
- cloud.google.com/go/billing: v1.13.0
- cloud.google.com/go/binaryauthorization: v1.5.0
- cloud.google.com/go/certificatemanager: v1.6.0
- cloud.google.com/go/channel: v1.12.0
- cloud.google.com/go/cloudbuild: v1.9.0
- cloud.google.com/go/clouddms: v1.5.0
- cloud.google.com/go/cloudtasks: v1.10.0
- cloud.google.com/go/compute/metadata: v0.2.3
- cloud.google.com/go/compute: v1.19.3
- cloud.google.com/go/contactcenterinsights: v1.6.0
- cloud.google.com/go/container: v1.15.0
- cloud.google.com/go/containeranalysis: v0.9.0
- cloud.google.com/go/datacatalog: v1.13.0
- cloud.google.com/go/dataflow: v0.8.0
- cloud.google.com/go/dataform: v0.7.0
- cloud.google.com/go/datafusion: v1.6.0
- cloud.google.com/go/datalabeling: v0.7.0
- cloud.google.com/go/dataplex: v1.6.0
- cloud.google.com/go/dataproc: v1.12.0
- cloud.google.com/go/dataqna: v0.7.0
- cloud.google.com/go/datastream: v1.7.0
- cloud.google.com/go/deploy: v1.8.0
- cloud.google.com/go/dialogflow: v1.32.0
- cloud.google.com/go/dlp: v1.9.0
- cloud.google.com/go/documentai: v1.18.0
- cloud.google.com/go/domains: v0.8.0
- cloud.google.com/go/edgecontainer: v1.0.0
- cloud.google.com/go/errorreporting: v0.3.0
- cloud.google.com/go/essentialcontacts: v1.5.0
- cloud.google.com/go/eventarc: v1.11.0
- cloud.google.com/go/filestore: v1.6.0
- cloud.google.com/go/functions: v1.13.0
- cloud.google.com/go/gaming: v1.9.0
- cloud.google.com/go/gkebackup: v0.4.0
- cloud.google.com/go/gkeconnect: v0.7.0
- cloud.google.com/go/gkehub: v0.12.0
- cloud.google.com/go/gkemulticloud: v0.5.0
- cloud.google.com/go/gsuiteaddons: v1.5.0
- cloud.google.com/go/iam: v1.1.0
- cloud.google.com/go/iap: v1.7.1
- cloud.google.com/go/ids: v1.3.0
- cloud.google.com/go/iot: v1.6.0
- cloud.google.com/go/kms: v1.14.0
- cloud.google.com/go/language: v1.9.0
- cloud.google.com/go/lifesciences: v0.8.0
- cloud.google.com/go/logging: v1.7.0
- cloud.google.com/go/longrunning: v0.4.2
- cloud.google.com/go/managedidentities: v1.5.0
- cloud.google.com/go/maps: v0.7.0
- cloud.google.com/go/mediatranslation: v0.7.0
- cloud.google.com/go/memcache: v1.9.0
- cloud.google.com/go/metastore: v1.10.0
- cloud.google.com/go/monitoring: v1.13.0
- cloud.google.com/go/networkconnectivity: v1.11.0
- cloud.google.com/go/networkmanagement: v1.6.0
- cloud.google.com/go/networksecurity: v0.8.0
- cloud.google.com/go/notebooks: v1.8.0
- cloud.google.com/go/optimization: v1.3.1
- cloud.google.com/go/orchestration: v1.6.0
- cloud.google.com/go/orgpolicy: v1.10.0
- cloud.google.com/go/osconfig: v1.11.0
- cloud.google.com/go/oslogin: v1.9.0
- cloud.google.com/go/phishingprotection: v0.7.0
- cloud.google.com/go/policytroubleshooter: v1.6.0
- cloud.google.com/go/privatecatalog: v0.8.0
- cloud.google.com/go/pubsublite: v1.7.0
- cloud.google.com/go/recaptchaenterprise/v2: v2.7.0
- cloud.google.com/go/recommendationengine: v0.7.0
- cloud.google.com/go/recommender: v1.9.0
- cloud.google.com/go/redis: v1.11.0
- cloud.google.com/go/resourcemanager: v1.7.0
- cloud.google.com/go/resourcesettings: v1.5.0
- cloud.google.com/go/retail: v1.12.0
- cloud.google.com/go/run: v0.9.0
- cloud.google.com/go/scheduler: v1.9.0
- cloud.google.com/go/secretmanager: v1.10.0
- cloud.google.com/go/security: v1.13.0
- cloud.google.com/go/securitycenter: v1.19.0
- cloud.google.com/go/servicedirectory: v1.9.0
- cloud.google.com/go/shell: v1.6.0
- cloud.google.com/go/spanner: v1.45.0
- cloud.google.com/go/speech: v1.15.0
- cloud.google.com/go/storagetransfer: v1.8.0
- cloud.google.com/go/talent: v1.5.0
- cloud.google.com/go/texttospeech: v1.6.0
- cloud.google.com/go/tpu: v1.5.0
- cloud.google.com/go/trace: v1.9.0
- cloud.google.com/go/translate: v1.7.0
- cloud.google.com/go/video: v1.15.0
- cloud.google.com/go/videointelligence: v1.10.0
- cloud.google.com/go/vision/v2: v2.7.0
- cloud.google.com/go/vmmigration: v1.6.0
- cloud.google.com/go/vmwareengine: v0.3.0
- cloud.google.com/go/vpcaccess: v1.6.0
- cloud.google.com/go/webrisk: v1.8.0
- cloud.google.com/go/websecurityscanner: v1.5.0
- cloud.google.com/go/workflows: v1.10.0
- contrib.go.opencensus.io/exporter/ocagent: 05415f1
- github.com/IBM-Cloud/power-go-client: [v1.2.2](https://github.com/IBM-Cloud/power-go-client/tree/v1.2.2)
- github.com/IBM/go-sdk-core/v5: [v5.12.1](https://github.com/IBM/go-sdk-core/v5/tree/v5.12.1)
- github.com/IBM/platform-services-go-sdk: [v0.31.4](https://github.com/IBM/platform-services-go-sdk/tree/v0.31.4)
- github.com/IBM/vpc-go-sdk: [v0.31.0](https://github.com/IBM/vpc-go-sdk/tree/v0.31.0)
- github.com/ProtonMail/go-crypto: [04723f9](https://github.com/ProtonMail/go-crypto/tree/04723f9)
- github.com/acomagu/bufpipe: [v1.0.3](https://github.com/acomagu/bufpipe/tree/v1.0.3)
- github.com/andygrunwald/go-jira: [v1.14.0](https://github.com/andygrunwald/go-jira/tree/v1.14.0)
- github.com/blang/semver/v4: [v4.0.0](https://github.com/blang/semver/v4/tree/v4.0.0)
- github.com/blendle/zapdriver: [v1.3.1](https://github.com/blendle/zapdriver/tree/v1.3.1)
- github.com/cncf/xds/go: [32f1caf](https://github.com/cncf/xds/go/tree/32f1caf)
- github.com/danwakefield/fnmatch: [cbb64ac](https://github.com/danwakefield/fnmatch/tree/cbb64ac)
- github.com/denormal/go-gitignore: [ae8ad1d](https://github.com/denormal/go-gitignore/tree/ae8ad1d)
- github.com/dgrijalva/jwt-go/v4: [v4.0.0-preview1](https://github.com/dgrijalva/jwt-go/v4/tree/v4.0.0-preview1)
- github.com/emirpasic/gods: [v1.12.0](https://github.com/emirpasic/gods/tree/v1.12.0)
- github.com/evanphx/json-patch/v5: [v5.6.0](https://github.com/evanphx/json-patch/v5/tree/v5.6.0)
- github.com/fatih/structs: [v1.1.0](https://github.com/fatih/structs/tree/v1.1.0)
- github.com/felixge/fgprof: [v0.9.1](https://github.com/felixge/fgprof/tree/v0.9.1)
- github.com/go-bindata/go-bindata/v3: [v3.1.3](https://github.com/go-bindata/go-bindata/v3/tree/v3.1.3)
- github.com/go-git/gcfg: [v1.5.0](https://github.com/go-git/gcfg/tree/v1.5.0)
- github.com/go-git/go-billy/v5: [v5.3.1](https://github.com/go-git/go-billy/v5/tree/v5.3.1)
- github.com/go-git/go-git/v5: [v5.4.2](https://github.com/go-git/go-git/v5/tree/v5.4.2)
- github.com/go-openapi/analysis: [v0.21.2](https://github.com/go-openapi/analysis/tree/v0.21.2)
- github.com/go-openapi/errors: [v0.20.2](https://github.com/go-openapi/errors/tree/v0.20.2)
- github.com/go-openapi/loads: [v0.21.1](https://github.com/go-openapi/loads/tree/v0.21.1)
- github.com/go-openapi/runtime: [v0.23.0](https://github.com/go-openapi/runtime/tree/v0.23.0)
- github.com/go-openapi/strfmt: [v0.21.3](https://github.com/go-openapi/strfmt/tree/v0.21.3)
- github.com/go-openapi/validate: [v0.20.3](https://github.com/go-openapi/validate/tree/v0.20.3)
- github.com/go-playground/locales: [v0.14.0](https://github.com/go-playground/locales/tree/v0.14.0)
- github.com/go-playground/universal-translator: [v0.18.0](https://github.com/go-playground/universal-translator/tree/v0.18.0)
- github.com/gobuffalo/flect: [v0.2.5](https://github.com/gobuffalo/flect/tree/v0.2.5)
- github.com/golang-jwt/jwt/v4: [v4.3.0](https://github.com/golang-jwt/jwt/v4/tree/v4.3.0)
- github.com/golang-jwt/jwt: [v3.2.1+incompatible](https://github.com/golang-jwt/jwt/tree/v3.2.1)
- github.com/golang/snappy: [v0.0.3](https://github.com/golang/snappy/tree/v0.0.3)
- github.com/google/gnostic: [v0.5.7-v3refs](https://github.com/google/gnostic/tree/v0.5.7-v3refs)
- github.com/google/s2a-go: [v0.1.4](https://github.com/google/s2a-go/tree/v0.1.4)
- github.com/google/wire: [v0.4.0](https://github.com/google/wire/tree/v0.4.0)
- github.com/googleapis/enterprise-certificate-proxy: [v0.2.3](https://github.com/googleapis/enterprise-certificate-proxy/tree/v0.2.3)
- github.com/googleapis/go-type-adapters: [v1.0.0](https://github.com/googleapis/go-type-adapters/tree/v1.0.0)
- github.com/gorilla/handlers: [v1.4.2](https://github.com/gorilla/handlers/tree/v1.4.2)
- github.com/hashicorp/go-retryablehttp: [v0.7.1](https://github.com/hashicorp/go-retryablehttp/tree/v0.7.1)
- github.com/jbenet/go-context: [d14ea06](https://github.com/jbenet/go-context/tree/d14ea06)
- github.com/kevinburke/ssh_config: [4977a11](https://github.com/kevinburke/ssh_config/tree/4977a11)
- github.com/leodido/go-urn: [v1.2.1](https://github.com/leodido/go-urn/tree/v1.2.1)
- github.com/mattn/go-ieproxy: [v0.0.1](https://github.com/mattn/go-ieproxy/tree/v0.0.1)
- github.com/maxbrunsfeld/counterfeiter/v6: [v6.4.1](https://github.com/maxbrunsfeld/counterfeiter/v6/tree/v6.4.1)
- github.com/prometheus/statsd_exporter: [v0.21.0](https://github.com/prometheus/statsd_exporter/tree/v0.21.0)
- github.com/rwcarlsen/goexif: [9e8deec](https://github.com/rwcarlsen/goexif/tree/9e8deec)
- github.com/trivago/tgo: [v1.0.7](https://github.com/trivago/tgo/tree/v1.0.7)
- github.com/xanzy/ssh-agent: [v0.3.0](https://github.com/xanzy/ssh-agent/tree/v0.3.0)
- go.mongodb.org/mongo-driver: v1.10.0
- go4.org: d4a0794
- gocloud.dev: v0.19.0
- google.golang.org/genproto/googleapis/api: e85fd2c
- google.golang.org/genproto/googleapis/bytestream: e85fd2c
- google.golang.org/genproto/googleapis/rpc: e85fd2c
- google.golang.org/grpc/cmd/protoc-gen-go-grpc: v1.1.0
- gopkg.in/go-playground/validator.v9: v9.31.0
- sigs.k8s.io/boskos: a7ef97e
- sigs.k8s.io/controller-tools: v0.9.2
- sigs.k8s.io/json: 9f7c6b3

### Changed

- cloud.google.com/go/bigquery: v1.8.0 → v1.50.0
- cloud.google.com/go/datastore: v1.1.0 → v1.11.0
- cloud.google.com/go/firestore: v1.1.0 → v1.9.0
- cloud.google.com/go/pubsub: v1.3.1 → v1.30.0
- cloud.google.com/go/storage: v1.10.0 → v1.22.1
- cloud.google.com/go: v0.65.0 → v0.110.2
- contrib.go.opencensus.io/exporter/prometheus: v0.1.0 → v0.4.0
- github.com/Azure/azure-pipeline-go: [v0.1.9 → v0.2.2](https://github.com/Azure/azure-pipeline-go/compare/v0.1.9...v0.2.2)
- github.com/Azure/azure-sdk-for-go: [v55.0.0+incompatible → v63.3.0+incompatible](https://github.com/Azure/azure-sdk-for-go/compare/v55.0.0...v63.3.0)
- github.com/Azure/azure-storage-blob-go: [457680c → v0.8.0](https://github.com/Azure/azure-storage-blob-go/compare/457680c...v0.8.0)
- github.com/Azure/go-autorest/autorest/adal: [v0.9.13 → v0.9.18](https://github.com/Azure/go-autorest/autorest/adal/compare/v0.9.13...v0.9.18)
- github.com/Azure/go-autorest/autorest/validation: [v0.1.0 → v0.2.0](https://github.com/Azure/go-autorest/autorest/validation/compare/v0.1.0...v0.2.0)
- github.com/Azure/go-autorest/autorest: [v0.11.18 → v0.11.24](https://github.com/Azure/go-autorest/autorest/compare/v0.11.18...v0.11.24)
- github.com/GoogleCloudPlatform/k8s-cloud-provider: [7901bc8 → v1.18.0](https://github.com/GoogleCloudPlatform/k8s-cloud-provider/compare/7901bc8...v1.18.0)
- github.com/GoogleCloudPlatform/testgrid: [v0.0.1-alpha.3 → v0.0.123](https://github.com/GoogleCloudPlatform/testgrid/compare/v0.0.1-alpha.3...v0.0.123)
- github.com/Microsoft/go-winio: [v0.4.16 → v0.5.1](https://github.com/Microsoft/go-winio/compare/v0.4.16...v0.5.1)
- github.com/andygrunwald/go-gerrit: [174420e → 9d38b0b](https://github.com/andygrunwald/go-gerrit/compare/174420e...9d38b0b)
- github.com/asaskevich/govalidator: [f61b66f → f21760c](https://github.com/asaskevich/govalidator/compare/f61b66f...f21760c)
- github.com/aws/aws-sdk-go: [v1.38.49 → v1.44.72](https://github.com/aws/aws-sdk-go/compare/v1.38.49...v1.44.72)
- github.com/bazelbuild/buildtools: [69366ca → 1038451](https://github.com/bazelbuild/buildtools/compare/69366ca...1038451)
- github.com/census-instrumentation/opencensus-proto: [v0.2.1 → v0.4.1](https://github.com/census-instrumentation/opencensus-proto/compare/v0.2.1...v0.4.1)
- github.com/cespare/xxhash/v2: [v2.1.1 → v2.2.0](https://github.com/cespare/xxhash/v2/compare/v2.1.1...v2.2.0)
- github.com/cncf/udpa/go: [5459f2c → c52dc94](https://github.com/cncf/udpa/go/compare/5459f2c...c52dc94)
- github.com/emicklei/go-restful: [v2.9.5+incompatible → v2.15.0+incompatible](https://github.com/emicklei/go-restful/compare/v2.9.5...v2.15.0)
- github.com/envoyproxy/go-control-plane: [668b12f → v0.11.0](https://github.com/envoyproxy/go-control-plane/compare/668b12f...v0.11.0)
- github.com/envoyproxy/protoc-gen-validate: [v0.1.0 → v0.10.0](https://github.com/envoyproxy/protoc-gen-validate/compare/v0.1.0...v0.10.0)
- github.com/evanphx/json-patch: [v4.11.0+incompatible → v4.12.0+incompatible](https://github.com/evanphx/json-patch/compare/v4.11.0...v4.12.0)
- github.com/fatih/color: [v1.7.0 → v1.12.0](https://github.com/fatih/color/compare/v1.7.0...v1.12.0)
- github.com/fsnotify/fsnotify: [v1.4.9 → v1.5.1](https://github.com/fsnotify/fsnotify/compare/v1.4.9...v1.5.1)
- github.com/fsouza/fake-gcs-server: [e85be23 → v1.19.4](https://github.com/fsouza/fake-gcs-server/compare/e85be23...v1.19.4)
- github.com/go-logr/logr: [v1.2.0 → v1.2.2](https://github.com/go-logr/logr/compare/v1.2.0...v1.2.2)
- github.com/go-logr/zapr: [v0.1.1 → v1.2.3](https://github.com/go-logr/zapr/compare/v0.1.1...v1.2.3)
- github.com/go-openapi/jsonreference: [v0.19.5 → v0.19.6](https://github.com/go-openapi/jsonreference/compare/v0.19.5...v0.19.6)
- github.com/go-openapi/spec: [v0.19.4 → v0.20.4](https://github.com/go-openapi/spec/compare/v0.19.4...v0.20.4)
- github.com/go-openapi/swag: [v0.19.14 → v0.21.1](https://github.com/go-openapi/swag/compare/v0.19.14...v0.21.1)
- github.com/go-stack/stack: [v1.8.0 → v1.8.1](https://github.com/go-stack/stack/compare/v1.8.0...v1.8.1)
- github.com/go-test/deep: [v1.0.4 → v1.0.7](https://github.com/go-test/deep/compare/v1.0.4...v1.0.7)
- github.com/gofrs/uuid: [v4.0.0+incompatible → v4.2.0+incompatible](https://github.com/gofrs/uuid/compare/v4.0.0...v4.2.0)
- github.com/golang/glog: [23def4e → v1.1.0](https://github.com/golang/glog/compare/23def4e...v1.1.0)
- github.com/golang/mock: [v1.4.4 → v1.6.0](https://github.com/golang/mock/compare/v1.4.4...v1.6.0)
- github.com/golang/protobuf: [v1.5.2 → v1.5.3](https://github.com/golang/protobuf/compare/v1.5.2...v1.5.3)
- github.com/gomodule/redigo: [v1.7.0 → v1.8.5](https://github.com/gomodule/redigo/compare/v1.7.0...v1.8.5)
- github.com/google/go-cmp: [v0.5.5 → v0.5.9](https://github.com/google/go-cmp/compare/v0.5.5...v0.5.9)
- github.com/google/go-containerregistry: [a3d713f → 00c59d9](https://github.com/google/go-containerregistry/compare/a3d713f...00c59d9)
- github.com/google/go-querystring: [v1.0.0 → v1.1.0](https://github.com/google/go-querystring/compare/v1.0.0...v1.1.0)
- github.com/google/gofuzz: [v1.1.0 → f78f29f](https://github.com/google/gofuzz/compare/v1.1.0...f78f29f)
- github.com/google/martian/v3: [v3.0.0 → v3.2.1](https://github.com/google/martian/v3/compare/v3.0.0...v3.2.1)
- github.com/google/pprof: [1a94d86 → 4bb14d4](https://github.com/google/pprof/compare/1a94d86...4bb14d4)
- github.com/google/uuid: [v1.1.2 → v1.3.0](https://github.com/google/uuid/compare/v1.1.2...v1.3.0)
- github.com/googleapis/gax-go/v2: [v2.0.5 → v2.11.0](https://github.com/googleapis/gax-go/v2/compare/v2.0.5...v2.11.0)
- github.com/googleapis/gax-go: [v2.0.0+incompatible → v2.0.2+incompatible](https://github.com/googleapis/gax-go/compare/v2.0.0...v2.0.2)
- github.com/gorilla/sessions: [v1.1.3 → v1.2.0](https://github.com/gorilla/sessions/compare/v1.1.3...v1.2.0)
- github.com/hashicorp/errwrap: [v1.0.0 → v1.1.0](https://github.com/hashicorp/errwrap/compare/v1.0.0...v1.1.0)
- github.com/hashicorp/go-cleanhttp: [v0.5.1 → v0.5.2](https://github.com/hashicorp/go-cleanhttp/compare/v0.5.1...v0.5.2)
- github.com/hashicorp/go-multierror: [v1.0.0 → v1.1.1](https://github.com/hashicorp/go-multierror/compare/v1.0.0...v1.1.1)
- github.com/hashicorp/golang-lru: [v0.5.3 → v0.5.4](https://github.com/hashicorp/golang-lru/compare/v0.5.3...v0.5.4)
- github.com/ianlancetaylor/demangle: [5e5cf60 → 28f6c0f](https://github.com/ianlancetaylor/demangle/compare/5e5cf60...28f6c0f)
- github.com/imdario/mergo: [v0.3.8 → v0.3.12](https://github.com/imdario/mergo/compare/v0.3.8...v0.3.12)
- github.com/json-iterator/go: [v1.1.11 → v1.1.12](https://github.com/json-iterator/go/compare/v1.1.11...v1.1.12)
- github.com/klauspost/compress: [v1.4.1 → v1.14.4](https://github.com/klauspost/compress/compare/v1.4.1...v1.14.4)
- github.com/kr/pty: [v1.1.5 → v1.1.1](https://github.com/kr/pty/compare/v1.1.5...v1.1.1)
- github.com/magiconair/properties: [v1.8.1 → v1.8.5](https://github.com/magiconair/properties/compare/v1.8.1...v1.8.5)
- github.com/mailru/easyjson: [v0.7.6 → v0.7.7](https://github.com/mailru/easyjson/compare/v0.7.6...v0.7.7)
- github.com/mattn/go-colorable: [v0.0.9 → v0.1.8](https://github.com/mattn/go-colorable/compare/v0.0.9...v0.1.8)
- github.com/mattn/go-isatty: [v0.0.4 → v0.0.12](https://github.com/mattn/go-isatty/compare/v0.0.4...v0.0.12)
- github.com/mattn/go-zglob: [v0.0.1 → v0.0.2](https://github.com/mattn/go-zglob/compare/v0.0.1...v0.0.2)
- github.com/mitchellh/mapstructure: [v1.1.2 → v1.4.3](https://github.com/mitchellh/mapstructure/compare/v1.1.2...v1.4.3)
- github.com/modern-go/reflect2: [v1.0.1 → v1.0.2](https://github.com/modern-go/reflect2/compare/v1.0.1...v1.0.2)
- github.com/opentracing/opentracing-go: [v1.1.0 → v1.2.0](https://github.com/opentracing/opentracing-go/compare/v1.1.0...v1.2.0)
- github.com/pelletier/go-toml: [v1.3.0 → v1.9.3](https://github.com/pelletier/go-toml/compare/v1.3.0...v1.9.3)
- github.com/prometheus/client_golang: [v1.11.1 → v1.12.1](https://github.com/prometheus/client_golang/compare/v1.11.1...v1.12.1)
- github.com/prometheus/common: [v0.26.0 → v0.32.1](https://github.com/prometheus/common/compare/v0.26.0...v0.32.1)
- github.com/prometheus/procfs: [v0.6.0 → v0.7.3](https://github.com/prometheus/procfs/compare/v0.6.0...v0.7.3)
- github.com/satori/go.uuid: [0aa62d5 → v1.2.0](https://github.com/satori/go.uuid/compare/0aa62d5...v1.2.0)
- github.com/shurcooL/githubv4: [51d7b50 → 83ba7b4](https://github.com/shurcooL/githubv4/compare/51d7b50...83ba7b4)
- github.com/shurcooL/graphql: [e4a3a37 → d48a9a7](https://github.com/shurcooL/graphql/compare/e4a3a37...d48a9a7)
- github.com/sirupsen/logrus: [v1.8.1 → v1.9.0](https://github.com/sirupsen/logrus/compare/v1.8.1...v1.9.0)
- github.com/spf13/afero: [v1.2.2 → v1.6.0](https://github.com/spf13/afero/compare/v1.2.2...v1.6.0)
- github.com/spf13/cast: [v1.3.0 → v1.3.1](https://github.com/spf13/cast/compare/v1.3.0...v1.3.1)
- github.com/spf13/cobra: [v1.1.3 → v1.4.0](https://github.com/spf13/cobra/compare/v1.1.3...v1.4.0)
- github.com/spf13/jwalterweatherman: [v1.0.0 → v1.1.0](https://github.com/spf13/jwalterweatherman/compare/v1.0.0...v1.1.0)
- github.com/spf13/viper: [v1.7.0 → v1.8.1](https://github.com/spf13/viper/compare/v1.7.0...v1.8.1)
- github.com/stretchr/objx: [v0.2.0 → v0.5.0](https://github.com/stretchr/objx/compare/v0.2.0...v0.5.0)
- github.com/stretchr/testify: [v1.7.0 → v1.8.1](https://github.com/stretchr/testify/compare/v1.7.0...v1.8.1)
- github.com/tektoncd/pipeline: [v0.8.0 → v0.36.0](https://github.com/tektoncd/pipeline/compare/v0.8.0...v0.36.0)
- github.com/yuin/goldmark: [v1.3.5 → v1.4.13](https://github.com/yuin/goldmark/compare/v1.3.5...v1.4.13)
- go.opencensus.io: v0.22.4 → v0.24.0
- go.uber.org/atomic: v1.7.0 → v1.9.0
- go.uber.org/multierr: v1.6.0 → v1.7.0
- go.uber.org/zap: v1.17.0 → v1.19.1
- golang.org/x/crypto: 5ea612d → v0.9.0
- golang.org/x/mod: v0.4.2 → v0.8.0
- golang.org/x/net: 37e1c6a → v0.10.0
- golang.org/x/oauth2: 5d25da1 → v0.8.0
- golang.org/x/sync: 036812b → v0.2.0
- golang.org/x/sys: 59db8d7 → v0.8.0
- golang.org/x/term: 6a3ed07 → v0.8.0
- golang.org/x/text: v0.3.6 → v0.9.0
- golang.org/x/time: 1f47c86 → 583f2d6
- golang.org/x/tools: v0.1.2 → v0.6.0
- golang.org/x/xerrors: 5ec99f8 → 04be3eb
- gomodules.xyz/jsonpatch/v2: v2.0.1 → v2.2.0
- google.golang.org/api: v0.34.0 → v0.126.0
- google.golang.org/appengine: v1.6.6 → v1.6.7
- google.golang.org/genproto: f16073e → e85fd2c
- google.golang.org/grpc: v1.38.0 → v1.55.0
- google.golang.org/protobuf: v1.26.0 → v1.30.0
- gopkg.in/ini.v1: v1.51.0 → v1.62.0
- gopkg.in/yaml.v3: 496545a → v3.0.1
- k8s.io/gengo: b6c5ce2 → 4627b89
- k8s.io/klog/v2: v2.60.1 → v2.80.1
- k8s.io/kube-openapi: 9528897 → 3ee0da9
- k8s.io/test-infra: 70a5174 → 46ac1a6
- k8s.io/utils: 4b05e18 → 3019533
- knative.dev/pkg: 56c2594 → 0a1ec2e
- sigs.k8s.io/controller-runtime: v0.3.0 → v0.12.3
- sigs.k8s.io/structured-merge-diff/v4: v4.1.2 → v4.2.1
- sigs.k8s.io/yaml: v1.2.0 → v1.3.0

### Removed

- contrib.go.opencensus.io/exporter/stackdriver: v0.12.8
- git.apache.org/thrift.git: 2566ecd
- github.com/aws/aws-k8s-tester: [b411acf](https://github.com/aws/aws-k8s-tester/tree/b411acf)
- github.com/coreos/go-etcd: [v2.0.0+incompatible](https://github.com/coreos/go-etcd/tree/v2.0.0)
- github.com/cpuguy83/go-md2man: [v1.0.10](https://github.com/cpuguy83/go-md2man/tree/v1.0.10)
- github.com/denisenkom/go-mssqldb: [2fea367](https://github.com/denisenkom/go-mssqldb/tree/2fea367)
- github.com/docker/cli: [7543883](https://github.com/docker/cli/tree/7543883)
- github.com/docker/docker-credential-helpers: [v0.6.3](https://github.com/docker/docker-credential-helpers/tree/v0.6.3)
- github.com/erikstmartin/go-testdb: [8d10e4a](https://github.com/erikstmartin/go-testdb/tree/8d10e4a)
- github.com/go-sql-driver/mysql: [7ebe0a5](https://github.com/go-sql-driver/mysql/tree/7ebe0a5)
- github.com/go-yaml/yaml: [v2.1.0+incompatible](https://github.com/go-yaml/yaml/tree/v2.1.0)
- github.com/golang/lint: [06c8688](https://github.com/golang/lint/tree/06c8688)
- github.com/gorilla/context: [v1.1.1](https://github.com/gorilla/context/tree/v1.1.1)
- github.com/gotestyourself/gotestyourself: [v2.2.0+incompatible](https://github.com/gotestyourself/gotestyourself/tree/v2.2.0)
- github.com/influxdata/influxdb: [049f9b4](https://github.com/influxdata/influxdb/tree/049f9b4)
- github.com/jinzhu/gorm: [572d0a0](https://github.com/jinzhu/gorm/tree/572d0a0)
- github.com/jinzhu/inflection: [f5c5f50](https://github.com/jinzhu/inflection/tree/f5c5f50)
- github.com/jinzhu/now: [v1.0.1](https://github.com/jinzhu/now/tree/v1.0.1)
- github.com/klauspost/cpuid: [v1.2.2](https://github.com/klauspost/cpuid/tree/v1.2.2)
- github.com/knative/build: [v0.1.2](https://github.com/knative/build/tree/v0.1.2)
- github.com/lib/pq: [v1.0.0](https://github.com/lib/pq/tree/v1.0.0)
- github.com/mattbaird/jsonpatch: [81af803](https://github.com/mattbaird/jsonpatch/tree/81af803)
- github.com/mattn/go-sqlite3: [38ee283](https://github.com/mattn/go-sqlite3/tree/38ee283)
- github.com/mitchellh/ioprogress: [6a23b12](https://github.com/mitchellh/ioprogress/tree/6a23b12)
- github.com/openzipkin/zipkin-go: [v0.1.1](https://github.com/openzipkin/zipkin-go/tree/v0.1.1)
- github.com/shurcooL/go: [9e1955d](https://github.com/shurcooL/go/tree/9e1955d)
- github.com/ugorji/go/codec: [d75b2dc](https://github.com/ugorji/go/codec/tree/d75b2dc)
- github.com/xlab/handysort: [fb3537e](https://github.com/xlab/handysort/tree/fb3537e)
- go.etcd.io/etcd: 83304cf
- gopkg.in/airbrake/gobrake.v2: v2.0.9
- gopkg.in/cheggaaa/pb.v1: v1.0.25
- gopkg.in/gemnasium/logrus-airbrake-hook.v2: v2.1.2
- k8s.io/klog: v1.0.0
- sigs.k8s.io/testing_frameworks: v0.1.1
- vbom.ml/util: efcd4e0

# v1.7.9 - Changelog since v1.7.8

## Changes by Kind

### Other (Cleanup or Flake)

- Separate user errors from internal errors ([#1219](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1219), [@sunnylovestiramisu](https://github.com/sunnylovestiramisu))
- Update go version to 1.19.10 ([#1273](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1273), [@k8s-infra-cherrypick-robot](https://github.com/k8s-infra-cherrypick-robot))
- Updates error message to be more user friendly when PD CSI Driver encounters an disk type UNSUPPORTED_OPERATION on ControllerPublishVolume ([#1224](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1224), [@pwschuurman](https://github.com/pwschuurman))

# v1.7.8 - Changelog since v1.7.7

## Changes by Kind

### Bug or Regression

- Separate user errors from internal errors. ([#1092](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1092), [@sunnylovestiramisu](https://github.com/sunnylovestiramisu))
- Upgrade klog v1 to v2 and fix error wrapping. ([#1084](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1084), [@sunnylovestiramisu](https://github.com/sunnylovestiramisu))

# v1.7.7 - Changelog since v1.7.6

## Changes by Kind

### Bug or Regression

- Add missing libraries, libbsd and libmd, that are dependencies for XFS volume expansion. ([#1204](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1204), [@nberlee](https://github.com/nberlee))

# v1.7.6 - Changelog since v.1.7.4

## Changes by Kind

### Other (Cleanup or Flake)

- go version updates ([#1158](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1158), [@saikat-royc](https://github.com/saikat-royc))
- Fix for CVEs - update base image ([#1162](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1162), [@sunnylovestiramisu](https://github.com/sunnylovestiramisu))
- Fix missing shared library libedit.so.2 caused from updating base image in [#1162](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1162) ([#1177](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1177), [@sunnylovestiramisu ](https://github.com/sunnylovestiramisu))

# v1.7.4 - Changelog since v.1.7.3

## Changes by Kind

### Bug or Regression

- Add udevadm binary in the container image. ([#1095](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1095), [@mattcary](https://github.com/mattcary))
- Fixed issue where Regional disks are repeatedly queued for re-attaching and consuming api quota ([#1091](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1091), [@pwschuurman](https://github.com/pwschuurman))

## Dependencies

_Nothing has changed._

# v1.7.3 - Changelog since v.1.7.2

- Update go builder to 1.18.4. Fixes several CVEs. (#1031, @mattcary)

- Cherry pick #1028, Improve backoff to be per-node and disk to avoid missing disks from blocking all operations (#1036, @mattcary)

# v1.7.2 - Changelog since v1.7.1

## Changes by Kind

### Uncategorized

- Enforce implicit pagination limit of 500 of the ListVolumesResponse#Entry field when ListVolumesRequest#max_entries is not set (#1011, @pwschuurman)

## Dependencies

_Nothing has changed._

# v1.7.1 - Changelog since v1.7.0

- Creates v1.7.1 upstream tag with changes from 1.7.0 release.
  - Cloud builder was broken when 1.7.0 was cut, so v1.7.0 upstream tag was not
  created.

# v1.7.0 - Changelog since v1.5.1

>**Attention:** 1.6.0 is not a recommended version to use because of known issues where pods can get stuck (due to controller publish/unpublish failures) during cluster upgrades or during node reboot (as seen in [#987](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/987)). Users should upgrade directly to the 1.7 branch.

## Changes by Kind

### Feature

- Allow to specify how frequently to poll for AttachDisk operation status, or any other global\regional\zonal operation. ([#956](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/956), [@sagor999](https://github.com/sagor999))

### Bug or Regression

- Default to MAXPROCS=1 to improve memory usage on nodes with many CPUs. ([#969](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/969), [@mattcary](https://github.com/mattcary))
- Simplify node backoff logic for controller publish/unpublish op ([#988](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/988), [@saikat-royc](https://github.com/saikat-royc))

### Other (Cleanup or Flake)

- Remove PodSecurityPolicy from deployment for 1.25+ clusters. ([#989](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/989), [@mattcary](https://github.com/mattcary))

### Uncategorized

- Lets users clone a regional disk from a zonal disk if one of the replica zones of the clone matches the zone of the source disk. ([#890](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/890), [@amacaskill](https://github.com/amacaskill))

## Dependencies

### Added

_Nothing has changed._

### Changed

- github.com/prometheus/client_golang: [v1.11.0 → v1.11.1](https://github.com/prometheus/client_golang/compare/v1.11.0...v1.11.1)

### Removed

_Nothing has changed._
