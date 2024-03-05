# v1.13.3 - Changelog since v1.13.2

## Changes by Kind

### Uncategorized

- Flag --use-instance-api-to-poll-attachment-disk-types uses instances.get API when polling for disk attachment in ControllerPublish for passed in disk types ([#1630](https://github.com/kube
rnetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1630), [@k8s-infra-cherrypick-robot](https://github.com/k8s-infra-cherrypick-robot))
- Support for read_ahead_kb mount flag to change block device readahead ([#1631](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1631), [@k8s-infra-cherrypick-
robot](https://github.com/k8s-infra-cherrypick-robot))

## Dependencies

### Added
_Nothing has changed._

### Changed
_Nothing has changed._

### Removed
_Nothing has changed._

# v1.13.2 - Changelog since v1.13.1

## Changes by Kind

### Bug

- Fix error when no compute endpoint is passed ([#1622](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1622), [@Sneha-at](https://github.com/Sneha-at))

## Dependencies

### Added
_Nothing has changed._

### Changed
_Nothing has changed._

### Removed
_Nothing has changed._

# v1.13.1 - Changelog since v1.13.0

## Changes by Kind

### Feature

- Add support for multi-zone volumeHandle ([#1616](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1616), [@pwschuurman](https://github.com/pwschuurman))
- Update driver to support staging compute ([#1614](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1614), [@Sneha-at](https://github.com/Sneha-at))

### Other (Cleanup or Flake)

- Map UNSUPPORTED_OPERATION GCE operation error codes to InvalidArgument ([#1610](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1610), [@amacaskill](https://github.com/amacaskill))

## Dependencies

### Added
_Nothing has changed._

### Changed
_Nothing has changed._

### Removed
_Nothing has changed._


# v1.13.0 - Changelog since v1.12.6

## Changes by Kind

### Feature

- Added enable-otel-tracing flag to enable opentelemetry tracing for self-managed drivers. The flag is disabled by default ([#1403](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1403), [@Fricounet](https://github.com/Fricounet))
- Adds validation and support for creating volumes in storage pools. ([#1526](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1526), [@amacaskill](https://github.com/amacaskill))

### Bug or Regression

- Add --fallback-requisite-zones flag to allow disk provisioning to fallback to a default set of zones when there are an insufficient number of zones available in a passed in requisite topology in CreateVolume. ([#1532](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1532), [@pwschuurman](https://github.com/pwschuurman))
- CVE fixes: CVE-2023-39323 ([#1412](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1412), [@dannawang0221](https://github.com/dannawang0221))
- Filter user misconfigured multiattach errors. ([#1559](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1559), [@mattcary](https://github.com/mattcary))
- Properly wrap error from GCE Images.Get() API call, to fix a potential nil-ptr dereference ([#1514](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1514), [@pwschuurman](https://github.com/pwschuurman))
- Reduce log spam when identifying NVMe devices located in `/dev` ([#1485](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1485), [@pwschuurman](https://github.com/pwschuurman))

### Other (Cleanup or Flake)

- The benign error when DisableDevice is not effective is logged as a warning. ([#1467](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1467), [@mattcary](https://github.com/mattcary))
- Update go version to 1.20.10 ([#1453](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1453), [@tyuchn](https://github.com/tyuchn))

### Uncategorized

- Adds enable_storage_pools label to the OperationErrorMetric so we can track CSI operation error rates when storage pools is used. ([#1534](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1534), [@amacaskill](https://github.com/amacaskill))

## Dependencies

### Added
- cloud.google.com/go/apikeys: v0.6.0
- cloud.google.com/go/dataproc/v2: v2.3.0
- cloud.google.com/go/grafeas: v0.2.0
- cloud.google.com/go/recaptchaenterprise: v1.3.1
- cloud.google.com/go/servicecontrol: v1.11.1
- cloud.google.com/go/servicemanagement: v1.8.0
- cloud.google.com/go/serviceusage: v1.6.0
- cloud.google.com/go/vision: v1.2.0
- gioui.org: 57750fc
- git.sr.ht/~sbinet/gg: v0.3.1
- github.com/JohnCGriffin/overflow: [46fa312](https://github.com/JohnCGriffin/overflow/tree/46fa312)
- github.com/ajstarks/deck/generate: [c3f852c](https://github.com/ajstarks/deck/generate/tree/c3f852c)
- github.com/ajstarks/deck: [30c9fc6](https://github.com/ajstarks/deck/tree/30c9fc6)
- github.com/ajstarks/svgo: [1546f12](https://github.com/ajstarks/svgo/tree/1546f12)
- github.com/apache/arrow/go/v10: [v10.0.1](https://github.com/apache/arrow/go/v10/tree/v10.0.1)
- github.com/apache/arrow/go/v11: [v11.0.0](https://github.com/apache/arrow/go/v11/tree/v11.0.0)
- github.com/boombuler/barcode: [v1.0.1](https://github.com/boombuler/barcode/tree/v1.0.1)
- github.com/buger/jsonparser: [v1.1.1](https://github.com/buger/jsonparser/tree/v1.1.1)
- github.com/cenkalti/backoff/v4: [v4.2.1](https://github.com/cenkalti/backoff/v4/tree/v4.2.1)
- github.com/flowstack/go-jsonschema: [v0.1.1](https://github.com/flowstack/go-jsonschema/tree/v0.1.1)
- github.com/fogleman/gg: [v1.3.0](https://github.com/fogleman/gg/tree/v1.3.0)
- github.com/go-fonts/dejavu: [v0.1.0](https://github.com/go-fonts/dejavu/tree/v0.1.0)
- github.com/go-fonts/latin-modern: [v0.2.0](https://github.com/go-fonts/latin-modern/tree/v0.2.0)
- github.com/go-fonts/liberation: [v0.2.0](https://github.com/go-fonts/liberation/tree/v0.2.0)
- github.com/go-fonts/stix: [v0.1.0](https://github.com/go-fonts/stix/tree/v0.1.0)
- github.com/go-latex/latex: [c0d11ff](https://github.com/go-latex/latex/tree/c0d11ff)
- github.com/go-logr/stdr: [v1.2.2](https://github.com/go-logr/stdr/tree/v1.2.2)
- github.com/go-pdf/fpdf: [v0.6.0](https://github.com/go-pdf/fpdf/tree/v0.6.0)
- github.com/goccy/go-json: [v0.9.11](https://github.com/goccy/go-json/tree/v0.9.11)
- github.com/golang/freetype: [e2365df](https://github.com/golang/freetype/tree/e2365df)
- github.com/google/flatbuffers: [v2.0.8+incompatible](https://github.com/google/flatbuffers/tree/v2.0.8)
- github.com/google/gnostic-models: [c7be7c7](https://github.com/google/gnostic-models/tree/c7be7c7)
- github.com/googleapis/go-type-adapters: [v1.0.0](https://github.com/googleapis/go-type-adapters/tree/v1.0.0)
- github.com/googleapis/google-cloud-go-testing: [bcd43fb](https://github.com/googleapis/google-cloud-go-testing/tree/bcd43fb)
- github.com/grpc-ecosystem/grpc-gateway/v2: [v2.16.0](https://github.com/grpc-ecosystem/grpc-gateway/v2/tree/v2.16.0)
- github.com/iancoleman/strcase: [v0.2.0](https://github.com/iancoleman/strcase/tree/v0.2.0)
- github.com/jung-kurt/gofpdf: [24315ac](https://github.com/jung-kurt/gofpdf/tree/24315ac)
- github.com/klauspost/asmfmt: [v1.3.2](https://github.com/klauspost/asmfmt/tree/v1.3.2)
- github.com/klauspost/cpuid/v2: [v2.0.9](https://github.com/klauspost/cpuid/v2/tree/v2.0.9)
- github.com/lyft/protoc-gen-star: [v0.6.1](https://github.com/lyft/protoc-gen-star/tree/v0.6.1)
- github.com/minio/asm2plan9s: [cdd7644](https://github.com/minio/asm2plan9s/tree/cdd7644)
- github.com/minio/c2goasm: [36a3d3b](https://github.com/minio/c2goasm/tree/36a3d3b)
- github.com/phpdave11/gofpdf: [v1.4.2](https://github.com/phpdave11/gofpdf/tree/v1.4.2)
- github.com/phpdave11/gofpdi: [v1.0.13](https://github.com/phpdave11/gofpdi/tree/v1.0.13)
- github.com/pierrec/lz4/v4: [v4.1.15](https://github.com/pierrec/lz4/v4/tree/v4.1.15)
- github.com/pkg/diff: [20ebb0f](https://github.com/pkg/diff/tree/20ebb0f)
- github.com/ruudk/golang-pdf417: [a7e3863](https://github.com/ruudk/golang-pdf417/tree/a7e3863)
- github.com/zeebo/assert: [v1.3.0](https://github.com/zeebo/assert/tree/v1.3.0)
- github.com/zeebo/xxh3: [v1.0.2](https://github.com/zeebo/xxh3/tree/v1.0.2)
- go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc: v1.21.0
- go.opentelemetry.io/otel/exporters/otlp/otlptrace: v1.21.0
- gonum.org/v1/plot: v0.10.1
- google.golang.org/grpc/cmd/protoc-gen-go-grpc: v1.1.0
- lukechampine.com/uint128: v1.2.0
- modernc.org/cc/v3: v3.36.3
- modernc.org/ccgo/v3: v3.16.9
- modernc.org/ccorpus: v1.11.6
- modernc.org/httpfs: v1.0.6
- modernc.org/libc: v1.17.1
- modernc.org/memory: v1.2.1
- modernc.org/opt: v0.1.3
- modernc.org/sqlite: v1.18.1
- modernc.org/tcl: v1.13.1
- modernc.org/token: v1.0.0
- modernc.org/z: v1.5.1
- rsc.io/pdf: v0.1.1

### Changed
- cloud.google.com/go/accessapproval: v1.7.1 → v1.7.4
- cloud.google.com/go/accesscontextmanager: v1.8.1 → v1.8.4
- cloud.google.com/go/aiplatform: v1.45.0 → v1.57.0
- cloud.google.com/go/analytics: v0.21.2 → v0.21.6
- cloud.google.com/go/apigateway: v1.6.1 → v1.6.4
- cloud.google.com/go/apigeeconnect: v1.6.1 → v1.6.4
- cloud.google.com/go/apigeeregistry: v0.7.1 → v0.8.2
- cloud.google.com/go/appengine: v1.8.1 → v1.8.4
- cloud.google.com/go/area120: v0.8.1 → v0.8.4
- cloud.google.com/go/artifactregistry: v1.14.1 → v1.14.6
- cloud.google.com/go/asset: v1.14.1 → v1.15.3
- cloud.google.com/go/assuredworkloads: v1.11.1 → v1.11.4
- cloud.google.com/go/automl: v1.13.1 → v1.13.4
- cloud.google.com/go/baremetalsolution: v0.5.0 → v1.2.3
- cloud.google.com/go/batch: v0.7.0 → v1.7.0
- cloud.google.com/go/beyondcorp: v0.6.1 → v1.0.3
- cloud.google.com/go/bigquery: v1.52.0 → v1.57.1
- cloud.google.com/go/billing: v1.16.0 → v1.18.0
- cloud.google.com/go/binaryauthorization: v1.6.1 → v1.8.0
- cloud.google.com/go/certificatemanager: v1.7.1 → v1.7.4
- cloud.google.com/go/channel: v1.16.0 → v1.17.3
- cloud.google.com/go/cloudbuild: v1.10.1 → v1.15.0
- cloud.google.com/go/clouddms: v1.6.1 → v1.7.3
- cloud.google.com/go/cloudtasks: v1.11.1 → v1.12.4
- cloud.google.com/go/compute: v1.20.1 → v1.23.3
- cloud.google.com/go/contactcenterinsights: v1.9.1 → v1.12.1
- cloud.google.com/go/container: v1.22.1 → v1.29.0
- cloud.google.com/go/containeranalysis: v0.10.1 → v0.11.3
- cloud.google.com/go/datacatalog: v1.14.1 → v1.19.0
- cloud.google.com/go/dataflow: v0.9.1 → v0.9.4
- cloud.google.com/go/dataform: v0.8.1 → v0.9.1
- cloud.google.com/go/datafusion: v1.7.1 → v1.7.4
- cloud.google.com/go/datalabeling: v0.8.1 → v0.8.4
- cloud.google.com/go/dataplex: v1.8.1 → v1.13.0
- cloud.google.com/go/dataqna: v0.8.1 → v0.8.4
- cloud.google.com/go/datastore: v1.12.0 → v1.15.0
- cloud.google.com/go/datastream: v1.9.1 → v1.10.3
- cloud.google.com/go/deploy: v1.11.0 → v1.16.0
- cloud.google.com/go/dialogflow: v1.38.0 → v1.47.0
- cloud.google.com/go/dlp: v1.10.1 → v1.11.1
- cloud.google.com/go/documentai: v1.20.0 → v1.23.6
- cloud.google.com/go/domains: v0.9.1 → v0.9.4
- cloud.google.com/go/edgecontainer: v1.1.1 → v1.1.4
- cloud.google.com/go/essentialcontacts: v1.6.2 → v1.6.5
- cloud.google.com/go/eventarc: v1.12.1 → v1.13.3
- cloud.google.com/go/filestore: v1.7.1 → v1.8.0
- cloud.google.com/go/firestore: v1.11.0 → v1.14.0
- cloud.google.com/go/functions: v1.15.1 → v1.15.4
- cloud.google.com/go/gaming: v1.10.1 → v1.9.0
- cloud.google.com/go/gkebackup: v0.4.0 → v1.3.4
- cloud.google.com/go/gkeconnect: v0.8.1 → v0.8.4
- cloud.google.com/go/gkehub: v0.14.1 → v0.14.4
- cloud.google.com/go/gkemulticloud: v0.6.1 → v1.0.3
- cloud.google.com/go/gsuiteaddons: v1.6.1 → v1.6.4
- cloud.google.com/go/iam: v1.1.0 → v1.1.5
- cloud.google.com/go/iap: v1.8.1 → v1.9.3
- cloud.google.com/go/ids: v1.4.1 → v1.4.4
- cloud.google.com/go/iot: v1.7.1 → v1.7.4
- cloud.google.com/go/kms: v1.12.1 → v1.15.5
- cloud.google.com/go/language: v1.10.1 → v1.12.2
- cloud.google.com/go/lifesciences: v0.9.1 → v0.9.4
- cloud.google.com/go/logging: v1.7.0 → v1.8.1
- cloud.google.com/go/longrunning: v0.5.1 → v0.5.4
- cloud.google.com/go/managedidentities: v1.6.1 → v1.6.4
- cloud.google.com/go/maps: v0.7.0 → v1.6.2
- cloud.google.com/go/mediatranslation: v0.8.1 → v0.8.4
- cloud.google.com/go/memcache: v1.10.1 → v1.10.4
- cloud.google.com/go/metastore: v1.11.1 → v1.13.3
- cloud.google.com/go/monitoring: v1.15.1 → v1.16.3
- cloud.google.com/go/networkconnectivity: v1.12.1 → v1.14.3
- cloud.google.com/go/networkmanagement: v1.8.0 → v1.9.3
- cloud.google.com/go/networksecurity: v0.9.1 → v0.9.4
- cloud.google.com/go/notebooks: v1.9.1 → v1.11.2
- cloud.google.com/go/optimization: v1.4.1 → v1.6.2
- cloud.google.com/go/orchestration: v1.8.1 → v1.8.4
- cloud.google.com/go/orgpolicy: v1.11.1 → v1.11.4
- cloud.google.com/go/osconfig: v1.12.1 → v1.12.4
- cloud.google.com/go/oslogin: v1.10.1 → v1.12.2
- cloud.google.com/go/phishingprotection: v0.8.1 → v0.8.4
- cloud.google.com/go/policytroubleshooter: v1.7.1 → v1.10.2
- cloud.google.com/go/privatecatalog: v0.9.1 → v0.9.4
- cloud.google.com/go/pubsub: v1.32.0 → v1.33.0
- cloud.google.com/go/recaptchaenterprise/v2: v2.7.2 → v2.9.0
- cloud.google.com/go/recommendationengine: v0.8.1 → v0.8.4
- cloud.google.com/go/recommender: v1.10.1 → v1.11.3
- cloud.google.com/go/redis: v1.13.1 → v1.14.1
- cloud.google.com/go/resourcemanager: v1.9.1 → v1.9.4
- cloud.google.com/go/resourcesettings: v1.6.1 → v1.6.4
- cloud.google.com/go/retail: v1.14.1 → v1.14.4
- cloud.google.com/go/run: v0.9.0 → v1.3.3
- cloud.google.com/go/scheduler: v1.10.1 → v1.10.5
- cloud.google.com/go/secretmanager: v1.11.1 → v1.11.4
- cloud.google.com/go/security: v1.15.1 → v1.15.4
- cloud.google.com/go/securitycenter: v1.23.0 → v1.24.3
- cloud.google.com/go/servicedirectory: v1.10.1 → v1.11.3
- cloud.google.com/go/shell: v1.7.1 → v1.7.4
- cloud.google.com/go/spanner: v1.47.0 → v1.53.1
- cloud.google.com/go/speech: v1.17.1 → v1.21.0
- cloud.google.com/go/storage: v1.12.0 → v1.29.0
- cloud.google.com/go/storagetransfer: v1.10.0 → v1.10.3
- cloud.google.com/go/talent: v1.6.2 → v1.6.5
- cloud.google.com/go/texttospeech: v1.7.1 → v1.7.4
- cloud.google.com/go/tpu: v1.6.1 → v1.6.4
- cloud.google.com/go/trace: v1.10.1 → v1.10.4
- cloud.google.com/go/translate: v1.8.1 → v1.9.3
- cloud.google.com/go/video: v1.17.1 → v1.20.3
- cloud.google.com/go/videointelligence: v1.11.1 → v1.11.4
- cloud.google.com/go/vision/v2: v2.7.2 → v2.7.5
- cloud.google.com/go/vmmigration: v1.7.1 → v1.7.4
- cloud.google.com/go/vmwareengine: v0.4.1 → v1.0.3
- cloud.google.com/go/vpcaccess: v1.7.1 → v1.7.4
- cloud.google.com/go/webrisk: v1.9.1 → v1.9.4
- cloud.google.com/go/websecurityscanner: v1.6.1 → v1.6.4
- cloud.google.com/go/workflows: v1.11.1 → v1.12.3
- cloud.google.com/go: v0.110.4 → v0.111.0
- github.com/Microsoft/go-winio: [v0.4.17 → v0.6.1](https://github.com/Microsoft/go-winio/compare/v0.4.17...v0.6.1)
- github.com/andybalholm/brotli: [5f990b6 → v1.0.4](https://github.com/andybalholm/brotli/compare/5f990b6...v1.0.4)
- github.com/apache/thrift: [v0.12.0 → v0.16.0](https://github.com/apache/thrift/compare/v0.12.0...v0.16.0)
- github.com/envoyproxy/go-control-plane: [9239064 → v0.11.1](https://github.com/envoyproxy/go-control-plane/compare/9239064...v0.11.1)
- github.com/envoyproxy/protoc-gen-validate: [v0.10.1 → v1.0.2](https://github.com/envoyproxy/protoc-gen-validate/compare/v0.10.1...v1.0.2)
- github.com/felixge/httpsnoop: [v1.0.1 → v1.0.4](https://github.com/felixge/httpsnoop/compare/v1.0.1...v1.0.4)
- github.com/go-logr/logr: [v1.2.3 → v1.3.0](https://github.com/go-logr/logr/compare/v1.2.3...v1.3.0)
- github.com/go-openapi/jsonpointer: [v0.19.5 → v0.20.0](https://github.com/go-openapi/jsonpointer/compare/v0.19.5...v0.20.0)
- github.com/go-openapi/swag: [v0.21.1 → v0.22.4](https://github.com/go-openapi/swag/compare/v0.21.1...v0.22.4)
- github.com/go-task/slim-sprig: [348f09d → 52ccab3](https://github.com/go-task/slim-sprig/compare/348f09d...52ccab3)
- github.com/golang/glog: [v1.1.0 → v1.1.2](https://github.com/golang/glog/compare/v1.1.0...v1.1.2)
- github.com/golang/snappy: [v0.0.1 → v0.0.4](https://github.com/golang/snappy/compare/v0.0.1...v0.0.4)
- github.com/google/gnostic: [v0.5.7-v3refs → v0.7.0](https://github.com/google/gnostic/compare/v0.5.7-v3refs...v0.7.0)
- github.com/google/go-cmp: [v0.5.9 → v0.6.0](https://github.com/google/go-cmp/compare/v0.5.9...v0.6.0)
- github.com/google/go-pkcs11: [v0.2.0 → c6f7932](https://github.com/google/go-pkcs11/compare/v0.2.0...c6f7932)
- github.com/google/martian/v3: [v3.1.0 → v3.3.2](https://github.com/google/martian/v3/compare/v3.1.0...v3.3.2)
- github.com/google/pprof: [94a9f03 → 4bb14d4](https://github.com/google/pprof/compare/94a9f03...4bb14d4)
- github.com/google/s2a-go: [v0.1.4 → v0.1.7](https://github.com/google/s2a-go/compare/v0.1.4...v0.1.7)
- github.com/google/uuid: [v1.3.0 → v1.5.0](https://github.com/google/uuid/compare/v1.3.0...v1.5.0)
- github.com/googleapis/enterprise-certificate-proxy: [v0.2.5 → v0.3.2](https://github.com/googleapis/enterprise-certificate-proxy/compare/v0.2.5...v0.3.2)
- github.com/klauspost/compress: [v1.13.6 → v1.15.9](https://github.com/klauspost/compress/compare/v1.13.6...v1.15.9)
- github.com/kr/pretty: [v0.3.0 → v0.3.1](https://github.com/kr/pretty/compare/v0.3.0...v0.3.1)
- github.com/kubernetes-csi/csi-proxy/client: [v1.1.1 → v1.1.3](https://github.com/kubernetes-csi/csi-proxy/client/compare/v1.1.1...v1.1.3)
- github.com/mattn/go-isatty: [v0.0.12 → v0.0.16](https://github.com/mattn/go-isatty/compare/v0.0.12...v0.0.16)
- github.com/onsi/ginkgo/v2: [v2.7.1 → v2.14.0](https://github.com/onsi/ginkgo/v2/compare/v2.7.1...v2.14.0)
- github.com/onsi/gomega: [v1.25.0 → v1.30.0](https://github.com/onsi/gomega/compare/v1.25.0...v1.30.0)
- github.com/pkg/sftp: [v1.10.1 → v1.13.1](https://github.com/pkg/sftp/compare/v1.10.1...v1.13.1)
- github.com/remyoudompheng/bigfft: [52369c6 → eec4a21](https://github.com/remyoudompheng/bigfft/compare/52369c6...eec4a21)
- github.com/rogpeppe/go-internal: [v1.9.0 → v1.10.0](https://github.com/rogpeppe/go-internal/compare/v1.9.0...v1.10.0)
- github.com/sirupsen/logrus: [v1.8.1 → v1.9.0](https://github.com/sirupsen/logrus/compare/v1.8.1...v1.9.0)
- github.com/spf13/afero: [v1.6.0 → v1.9.2](https://github.com/spf13/afero/compare/v1.6.0...v1.9.2)
- github.com/stretchr/testify: [v1.8.1 → v1.8.4](https://github.com/stretchr/testify/compare/v1.8.1...v1.8.4)
- github.com/xeipuuv/gojsonschema: [v1.1.0 → v1.2.0](https://github.com/xeipuuv/gojsonschema/compare/v1.1.0...v1.2.0)
- go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc: v0.20.0 → v0.46.1
- go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp: v0.20.0 → v0.46.1
- go.opentelemetry.io/otel/metric: v0.20.0 → v1.21.0
- go.opentelemetry.io/otel/sdk: v0.20.0 → v1.21.0
- go.opentelemetry.io/otel/trace: v0.20.0 → v1.21.0
- go.opentelemetry.io/otel: v0.20.0 → v1.21.0
- go.opentelemetry.io/proto/otlp: v0.7.0 → v1.0.0
- go.uber.org/goleak: v1.1.10 → v1.3.0
- golang.org/x/crypto: v0.11.0 → v0.18.0
- golang.org/x/exp: 6cc2880 → 334a238
- golang.org/x/image: cff245a → 723b81c
- golang.org/x/mod: v0.8.0 → v0.14.0
- golang.org/x/net: v0.12.0 → v0.20.0
- golang.org/x/oauth2: v0.10.0 → v0.16.0
- golang.org/x/sync: v0.3.0 → v0.6.0
- golang.org/x/sys: v0.10.0 → v0.16.0
- golang.org/x/term: v0.10.0 → v0.16.0
- golang.org/x/text: v0.11.0 → v0.14.0
- golang.org/x/time: 90d013b → v0.5.0
- golang.org/x/tools: v0.6.0 → v0.16.1
- golang.org/x/xerrors: 5ec99f8 → 04be3eb
- gonum.org/v1/gonum: 3d26580 → v0.11.0
- google.golang.org/api: v0.134.0 → v0.156.0
- google.golang.org/appengine: v1.6.7 → v1.6.8
- google.golang.org/genproto/googleapis/api: ccb25ca → 995d672
- google.golang.org/genproto/googleapis/bytestream: 659f7aa → 50ed04b
- google.golang.org/genproto/googleapis/rpc: 659f7aa → 50ed04b
- google.golang.org/genproto: ccb25ca → 995d672
- google.golang.org/grpc: v1.56.2 → v1.60.1
- google.golang.org/protobuf: v1.31.0 → v1.32.0
- honnef.co/go/tools: v0.0.1-2020.1.4 → v0.1.3
- k8s.io/klog/v2: v2.90.1 → v2.100.1
- k8s.io/mount-utils: v0.27.0-alpha.3 → v0.29.0-alpha.2
- k8s.io/utils: a36077c → 3b25d92
- modernc.org/mathutil: v1.0.0 → v1.5.0
- modernc.org/strutil: v1.0.0 → v1.1.3
- sigs.k8s.io/structured-merge-diff/v4: v4.2.1 → v4.4.1
- sigs.k8s.io/yaml: v1.3.0 → v1.4.0

### Removed
_Nothing has changed._
