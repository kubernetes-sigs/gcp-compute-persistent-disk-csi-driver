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
