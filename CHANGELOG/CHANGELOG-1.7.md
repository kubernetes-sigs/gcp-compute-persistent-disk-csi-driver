# v1.7.5 - Changelog since v.1.7.4

## Changes by Kind

### Other (Cleanup or Flake)

- go version updates ([#1158](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1158), [@saikat-royc](https://github.com/saikat-royc))
- Fix for CVEs - update base image ([#1162](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1162), [@saikat-royc](https://github.com/sunnylovestiramisu))

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
