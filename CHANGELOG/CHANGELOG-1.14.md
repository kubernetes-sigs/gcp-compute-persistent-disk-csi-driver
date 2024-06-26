# v1.14.0 - Changelog since v1.13.6

## Changes by Kind

### Feature

- Add support for provisioning multi-zone volume handles ([#1733](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1733), [@pwschuurman](https://github.com/pwschuurman))
- Users can now attach existing tags to GCP Compute Disk, Image, Snapshot resources created by the driver. The driver now accepts a new argument, `--extra-tags`, and a list of tags can be provided to the driver using this argument. The argument is optional, and if the tags are not provided, then there is no change in the existing behavior. ([#1377](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1377), [@arkadeepsen](https://github.com/arkadeepsen))

### Bug or Regression

- Reassign error returned from validateStoragePools so InvalidArgument is recorded ([#1710](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1710), [@amacaskill](https://github.com/amacaskill))
- Remove disable device call on node unstage. This call was producing spurious error messages and was not effective. ([#1689](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1689), [@mattcary](https://github.com/mattcary))
- Update debian base image from bullseye to bookworm to fix CVE-2024-33600, CVE-2024-33602, CVE-2024-2961, CVE-2024-33601, CVE-2024-33599 ([#1694](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1694), [@Sneha-at](https://github.com/Sneha-at))

### Other (Cleanup or Flake)

- Properly parse comma separated input flags to filter out empty strings ([#1730](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1730), [@pwschuurman](https://github.com/pwschuurman))
- Return Unavailable for 'connection reset by peer' errors. ([#1720](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1720), [@amacaskill](https://github.com/amacaskill))
- Upgrade google.golang.org/api from 172.0 -> 182.0 ([#1725](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1725), [@pwschuurman](https://github.com/pwschuurman))

### Uncategorized

- Error codes extracted from errors part of compute engine api are now exposed with correct http errors instead of being classified as Unknown. ([#1708](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1708), [@hime](https://github.com/hime))
- The --instances-list-filters command line flag allows the driver to filter on response properties when listing VMs from the GCE API in ListVolumes RPC.
  The --use-instance-api-to-list-volumes-published-nodes command line flag allows the driver to use the instances.list API instead of disks.list API when determining PublishedNodeIs in ListVolumes RPC.
    ```
  
  **Testing**:
  
  Validated existing behavior with: ([#1696](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1696), [@pwschuurman](https://github.com/pwschuurman))
- Update logging on multi-zone feature support for volume snapshot and resize ([#1718](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1718), [@hungnguyen243](https://github.com/hungnguyen243))

## Dependencies

### Added
- cloud.google.com/go/auth/oauth2adapt: v0.2.2
- cloud.google.com/go/auth: v0.5.1
- github.com/chromedp/cdproto: [3cf4e6d](https://github.com/chromedp/cdproto/tree/3cf4e6d)
- github.com/chromedp/chromedp: [v0.9.2](https://github.com/chromedp/chromedp/tree/v0.9.2)
- github.com/chromedp/sysutil: [v1.0.0](https://github.com/chromedp/sysutil/tree/v1.0.0)
- github.com/go-task/slim-sprig/v3: [v3.0.0](https://github.com/go-task/slim-sprig/v3/tree/v3.0.0)
- github.com/gobwas/httphead: [v0.1.0](https://github.com/gobwas/httphead/tree/v0.1.0)
- github.com/gobwas/pool: [v0.2.1](https://github.com/gobwas/pool/tree/v0.2.1)
- github.com/gobwas/ws: [v1.2.1](https://github.com/gobwas/ws/tree/v1.2.1)
- golang.org/x/telemetry: f48c80b

### Changed
- cloud.google.com/go/accessapproval: v1.7.4 → v1.7.7
- cloud.google.com/go/accesscontextmanager: v1.8.4 → v1.8.7
- cloud.google.com/go/aiplatform: v1.58.0 → v1.67.0
- cloud.google.com/go/analytics: v0.22.0 → v0.23.2
- cloud.google.com/go/apigateway: v1.6.4 → v1.6.7
- cloud.google.com/go/apigeeconnect: v1.6.4 → v1.6.7
- cloud.google.com/go/apigeeregistry: v0.8.2 → v0.8.5
- cloud.google.com/go/appengine: v1.8.4 → v1.8.7
- cloud.google.com/go/area120: v0.8.4 → v0.8.7
- cloud.google.com/go/artifactregistry: v1.14.6 → v1.14.9
- cloud.google.com/go/asset: v1.17.0 → v1.19.1
- cloud.google.com/go/assuredworkloads: v1.11.4 → v1.11.7
- cloud.google.com/go/automl: v1.13.4 → v1.13.7
- cloud.google.com/go/baremetalsolution: v1.2.3 → v1.2.6
- cloud.google.com/go/batch: v1.7.0 → v1.8.6
- cloud.google.com/go/beyondcorp: v1.0.3 → v1.0.6
- cloud.google.com/go/bigquery: v1.58.0 → v1.61.0
- cloud.google.com/go/billing: v1.18.0 → v1.18.5
- cloud.google.com/go/binaryauthorization: v1.8.0 → v1.8.3
- cloud.google.com/go/certificatemanager: v1.7.4 → v1.8.1
- cloud.google.com/go/channel: v1.17.4 → v1.17.7
- cloud.google.com/go/cloudbuild: v1.15.0 → v1.16.1
- cloud.google.com/go/clouddms: v1.7.3 → v1.7.6
- cloud.google.com/go/cloudtasks: v1.12.4 → v1.12.8
- cloud.google.com/go/compute/metadata: v0.2.3 → v0.3.0
- cloud.google.com/go/compute: v1.23.4 → v1.27.0
- cloud.google.com/go/contactcenterinsights: v1.12.1 → v1.13.2
- cloud.google.com/go/container: v1.29.0 → v1.35.1
- cloud.google.com/go/containeranalysis: v0.11.3 → v0.11.6
- cloud.google.com/go/datacatalog: v1.19.2 → v1.20.1
- cloud.google.com/go/dataflow: v0.9.4 → v0.9.7
- cloud.google.com/go/dataform: v0.9.1 → v0.9.4
- cloud.google.com/go/datafusion: v1.7.4 → v1.7.7
- cloud.google.com/go/datalabeling: v0.8.4 → v0.8.7
- cloud.google.com/go/dataplex: v1.14.0 → v1.16.0
- cloud.google.com/go/dataproc/v2: v2.3.0 → v2.4.2
- cloud.google.com/go/dataqna: v0.8.4 → v0.8.7
- cloud.google.com/go/datastore: v1.15.0 → v1.17.0
- cloud.google.com/go/datastream: v1.10.3 → v1.10.6
- cloud.google.com/go/deploy: v1.17.0 → v1.19.0
- cloud.google.com/go/dialogflow: v1.48.1 → v1.53.0
- cloud.google.com/go/dlp: v1.11.1 → v1.13.0
- cloud.google.com/go/documentai: v1.23.7 → v1.28.1
- cloud.google.com/go/domains: v0.9.4 → v0.9.7
- cloud.google.com/go/edgecontainer: v1.1.4 → v1.2.1
- cloud.google.com/go/essentialcontacts: v1.6.5 → v1.6.8
- cloud.google.com/go/eventarc: v1.13.3 → v1.13.6
- cloud.google.com/go/filestore: v1.8.0 → v1.8.3
- cloud.google.com/go/firestore: v1.14.0 → v1.15.0
- cloud.google.com/go/functions: v1.15.4 → v1.16.2
- cloud.google.com/go/gkebackup: v1.3.4 → v1.5.0
- cloud.google.com/go/gkeconnect: v0.8.4 → v0.8.7
- cloud.google.com/go/gkehub: v0.14.4 → v0.14.7
- cloud.google.com/go/gkemulticloud: v1.1.0 → v1.2.0
- cloud.google.com/go/gsuiteaddons: v1.6.4 → v1.6.7
- cloud.google.com/go/iam: v1.1.5 → v1.1.8
- cloud.google.com/go/iap: v1.9.3 → v1.9.6
- cloud.google.com/go/ids: v1.4.4 → v1.4.7
- cloud.google.com/go/iot: v1.7.4 → v1.7.7
- cloud.google.com/go/kms: v1.15.5 → v1.17.1
- cloud.google.com/go/language: v1.12.2 → v1.12.5
- cloud.google.com/go/lifesciences: v0.9.4 → v0.9.7
- cloud.google.com/go/logging: v1.9.0 → v1.10.0
- cloud.google.com/go/longrunning: v0.5.4 → v0.5.7
- cloud.google.com/go/managedidentities: v1.6.4 → v1.6.7
- cloud.google.com/go/maps: v1.6.3 → v1.10.0
- cloud.google.com/go/mediatranslation: v0.8.4 → v0.8.7
- cloud.google.com/go/memcache: v1.10.4 → v1.10.7
- cloud.google.com/go/metastore: v1.13.3 → v1.13.6
- cloud.google.com/go/monitoring: v1.17.0 → v1.19.0
- cloud.google.com/go/networkconnectivity: v1.14.3 → v1.14.6
- cloud.google.com/go/networkmanagement: v1.9.3 → v1.13.2
- cloud.google.com/go/networksecurity: v0.9.4 → v0.9.7
- cloud.google.com/go/notebooks: v1.11.2 → v1.11.5
- cloud.google.com/go/optimization: v1.6.2 → v1.6.5
- cloud.google.com/go/orchestration: v1.8.4 → v1.9.2
- cloud.google.com/go/orgpolicy: v1.12.0 → v1.12.3
- cloud.google.com/go/osconfig: v1.12.4 → v1.12.7
- cloud.google.com/go/oslogin: v1.13.0 → v1.13.3
- cloud.google.com/go/phishingprotection: v0.8.4 → v0.8.7
- cloud.google.com/go/policytroubleshooter: v1.10.2 → v1.10.5
- cloud.google.com/go/privatecatalog: v0.9.4 → v0.9.7
- cloud.google.com/go/pubsub: v1.34.0 → v1.38.0
- cloud.google.com/go/recaptchaenterprise/v2: v2.9.0 → v2.13.0
- cloud.google.com/go/recommendationengine: v0.8.4 → v0.8.7
- cloud.google.com/go/recommender: v1.12.0 → v1.12.3
- cloud.google.com/go/redis: v1.14.1 → v1.15.0
- cloud.google.com/go/resourcemanager: v1.9.4 → v1.9.7
- cloud.google.com/go/resourcesettings: v1.6.4 → v1.6.7
- cloud.google.com/go/retail: v1.14.4 → v1.16.2
- cloud.google.com/go/run: v1.3.3 → v1.3.7
- cloud.google.com/go/scheduler: v1.10.5 → v1.10.8
- cloud.google.com/go/secretmanager: v1.11.4 → v1.13.1
- cloud.google.com/go/security: v1.15.4 → v1.17.0
- cloud.google.com/go/securitycenter: v1.24.3 → v1.30.0
- cloud.google.com/go/servicedirectory: v1.11.3 → v1.11.7
- cloud.google.com/go/shell: v1.7.4 → v1.7.7
- cloud.google.com/go/spanner: v1.55.0 → v1.63.0
- cloud.google.com/go/speech: v1.21.0 → v1.23.1
- cloud.google.com/go/storage: v1.29.0 → v1.40.0
- cloud.google.com/go/storagetransfer: v1.10.3 → v1.10.6
- cloud.google.com/go/talent: v1.6.5 → v1.6.8
- cloud.google.com/go/texttospeech: v1.7.4 → v1.7.7
- cloud.google.com/go/tpu: v1.6.4 → v1.6.7
- cloud.google.com/go/trace: v1.10.4 → v1.10.7
- cloud.google.com/go/translate: v1.10.0 → v1.10.3
- cloud.google.com/go/video: v1.20.3 → v1.20.6
- cloud.google.com/go/videointelligence: v1.11.4 → v1.11.7
- cloud.google.com/go/vision/v2: v2.7.5 → v2.8.2
- cloud.google.com/go/vmmigration: v1.7.4 → v1.7.7
- cloud.google.com/go/vmwareengine: v1.0.3 → v1.1.3
- cloud.google.com/go/vpcaccess: v1.7.4 → v1.7.7
- cloud.google.com/go/webrisk: v1.9.4 → v1.9.7
- cloud.google.com/go/websecurityscanner: v1.6.4 → v1.6.7
- cloud.google.com/go/workflows: v1.12.3 → v1.12.6
- cloud.google.com/go: v0.112.0 → v0.114.0
- github.com/chzyer/readline: [2972be2 → v1.5.1](https://github.com/chzyer/readline/compare/2972be2...v1.5.1)
- github.com/cncf/xds/go: [0fa0005 → 8a4994d](https://github.com/cncf/xds/go/compare/0fa0005...8a4994d)
- github.com/go-task/slim-sprig: [52ccab3 → 348f09d](https://github.com/go-task/slim-sprig/compare/52ccab3...348f09d)
- github.com/google/martian/v3: [v3.3.2 → v3.3.3](https://github.com/google/martian/v3/compare/v3.3.2...v3.3.3)
- github.com/google/pprof: [4bb14d4 → a892ee0](https://github.com/google/pprof/compare/4bb14d4...a892ee0)
- github.com/googleapis/gax-go/v2: [v2.12.3 → v2.12.4](https://github.com/googleapis/gax-go/v2/compare/v2.12.3...v2.12.4)
- github.com/ianlancetaylor/demangle: [28f6c0f → bd984b5](https://github.com/ianlancetaylor/demangle/compare/28f6c0f...bd984b5)
- github.com/onsi/ginkgo/v2: [v2.14.0 → v2.19.0](https://github.com/onsi/ginkgo/v2/compare/v2.14.0...v2.19.0)
- github.com/onsi/gomega: [v1.30.0 → v1.33.1](https://github.com/onsi/gomega/compare/v1.30.0...v1.33.1)
- github.com/stretchr/testify: [v1.8.4 → v1.9.0](https://github.com/stretchr/testify/compare/v1.8.4...v1.9.0)
- go.opentelemetry.io/otel/sdk: v1.21.0 → v1.24.0
- golang.org/x/crypto: v0.22.0 → v0.24.0
- golang.org/x/mod: v0.14.0 → v0.18.0
- golang.org/x/net: v0.24.0 → v0.26.0
- golang.org/x/oauth2: v0.18.0 → v0.21.0
- golang.org/x/sync: v0.6.0 → v0.7.0
- golang.org/x/sys: v0.19.0 → v0.21.0
- golang.org/x/term: v0.19.0 → v0.21.0
- golang.org/x/text: v0.14.0 → v0.16.0
- golang.org/x/tools: v0.16.1 → e35e4cc
- google.golang.org/api: v0.172.0 → v0.183.0
- google.golang.org/genproto/googleapis/api: a219d84 → d264139
- google.golang.org/genproto/googleapis/bytestream: 94a12d6 → 5315273
- google.golang.org/genproto/googleapis/rpc: 94a12d6 → 5315273
- google.golang.org/genproto: ef43131 → 5315273
- google.golang.org/grpc: v1.62.1 → v1.64.0
- google.golang.org/protobuf: v1.33.0 → v1.34.1
- k8s.io/klog/v2: v2.100.1 → v2.120.1
- k8s.io/mount-utils: v0.29.0-alpha.2 → v0.30.1

### Removed
_Nothing has changed._
