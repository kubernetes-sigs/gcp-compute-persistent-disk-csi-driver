# v1.2.6 - Changelog since v1.2.5

## Changes by Kind

### Uncategorized

- Cherry-pick #930: Update golang version to 1.17.8 for building drivers. ([#938](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/938), [@pwschuurman](https://github.com/pwschuurman))

## Dependencies

### Added
_Nothing has changed._

### Changed
_Nothing has changed._

### Removed
_Nothing has changed._

# v1.2.5 - Changelog since v1.2.4

- Update base image to buster-1.10

# v1.2.4 - Changelog since v1.2.3

## Changes by Kind

### Bug or Regression

- Update go builder to 1.17 ([#850](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/850), [@mattcary](https://github.com/mattcary))

## Dependencies

### Added
_Nothing has changed._

### Changed
_Nothing has changed._

### Removed
_Nothing has changed._

# v1.2.3 - Changelog since v1.2.1

## Changes by Kind

### Bug or Regression

- Update debian images to buster-v1.9.0 ([#839](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/839), [@mattcary](https://github.com/mattcary))

### Uncategorized

- It is now possible to mount a volume with XFS filesystem and its restored snapshot. ([#838](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/838), [@leiyiz](https://github.com/leiyiz))

## Dependencies

### Added
_Nothing has changed._

### Changed
_Nothing has changed._

### Removed
_Nothing has changed._

# v1.2.2 - Changelog since v1.2.1

## Changes by Kind

### Feature

- Update base image to buster-1.6.0 ([#760](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/760), [@cpanato](https://github.com/cpanato))

- Update base image to buster-1.5.0. ([#755](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/755), [@mattcary](https://github.com/mattcary))

## Dependencies

### Added
_Nothing has changed._

### Changed
_Nothing has changed._

### Removed
_Nothing has changed._

# v1.2.1 - Changelog since v1.2.0

## Tests

- Update kustomize to 3.9.4 ([703](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/703), [@saikat-royc](https://github.com/saikat-royc))
- Fix cluster list parsing for latest gcloud version ([720](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/720), [@verult](https://github.com/verult))

## Other

- Remove Probe logging ([682](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/682), [@Jiawei0227](https://github.com/Jiawei0227))
- Round up pdcsi driver size in CreateVolume ([684](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/684), [@Jiawei0227](https://github.com/Jiawei0227))
- Add gce disk labels support via create volume parameters ([718](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/718), [@mattcary](https://github.com/mattcary))
- Emit GKE PDCSI component version metric ([719](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/719), [@saikat-royc](https://github.com/saikat-royc))
- Add cloudbuild configuration to build the image gcp-compute-persistent-disk-csi-driver ([734](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/734), [@cpanato](https://github.com/cpanato))
- Bump go to the latest 1.13 available in Dockerfile ([734](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/734), [@cpanato](https://github.com/cpanato))
- Log GKE component version for the PD CSI driver ([757](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/757), [@saikat-royc](https://github.com/saikat-royc))

# v1.2.0 - Changelog since v1.1.0

## Features

- Improved Windows Support
  - Add Disk online/offline logic in nodeStageVolume/nodeUnstageVolume calls for Windows. This requires the CSI Proxy `disk.v1beta2` group, which is only available in CSI proxy v0.2.2+. ([#661](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/661), [@jingxu97](https://github.com/jingxu97))

## Bugs or Regressions

- Fix "volume is mounted" check during NodePublishVolume for Windows ([#666](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/666), [@jingxu97](https://github.com/jingxu97))
- Add empty string check on returned volumeIds to avoid nil pointer panic ([#673](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/673), [@jingxu97](https://github.com/jingxu97))

## Tests

- Update kustomizer to 3.8.6. ([#661](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/661), [@jingxu97](https://github.com/jingxu97))
- Updates named pipe path in node.yaml for base deployment overlay to use CSI proxy disk v1beta2 (previously v1beta1) for Windows. ([#669](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/669), [@jingxu97](https://github.com/jingxu97))
- Fix node version check for node skew tests. ([#645](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/645), [@saikat-royc](https://github.com/saikat-royc))
- Add run-k8s-integration-ci.sh that the prow driver now expects to be in each release. ([#655](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/655), [@mattcary](https://github.com/mattcary))
- Use node version on GKE when detecting XFS compatibility. ([#656](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/656), [@mattcary](https://github.com/mattcary))
- Skip Pod fsgroupchange policy tests for < 1.20 k8s. ([#667](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/667), [@saikat-royc](https://github.com/saikat-royc))
- Shorten the GKE cluster name. ([#671](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/671), [@saikat-royc](https://github.com/saikat-royc))

## Documentation

No notable changes.

## Dependencies

### Added
_Nothing has changed._

### Changed
- github.com/kubernetes-csi/csi-proxy/client: [v0.2.1 → v0.2.2](https://github.com/kubernetes-csi/csi-proxy/client/compare/v0.2.1...v0.2.2)

### Removed
_Nothing has changed._
