# v1.5.1 - Changelog since v1.5.0

## Changes by Kind

### Bug or Regression

- Fix bug in original disk-image implementation that was fixed in test PR ([#955](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/955), [@mattcary](https://github.com/mattcary))

# v1.5.0 - Changelog since v1.4.1

## Changes by Kind

### Feature

- Add parameters to VolumeSnapshotClass for disk image config. ([#926](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/926), [@luohao](https://github.com/luohao))

### Bug or Regression

- Fix ControllerUnpublish backoff ([#953](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/953), [@saikat-royc](https://github.com/saikat-royc))

### Documentation

- Adds documentation for how to use the PD CSI Driver overlays for testing and deploying the driver. ([#932](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/932), [@amacaskill](https://github.com/amacaskill))

## Dependencies

### Added
_Nothing has changed._

### Changed
_Nothing has changed._

### Removed
- github.com/GoogleCloudPlatform/guest-configs: [a0dacef](https://github.com/GoogleCloudPlatform/guest-configs/tree/a0dacef)
