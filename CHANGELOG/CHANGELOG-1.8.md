# v1.8.2 - Changelog since v1.8.1


## Changes by Kind

### Other (Cleanup or Flake)

- Update to go 1.19.4 ([#1103](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1103), [@msau42](https://github.com/msau42))
- limit grpc loging info to a configurable char limit ([#1111](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1111), [@leiyiz](https://github.com/leiyiz))
- Upgrade klog v1 to v2 and fix error wrapping & separate user errors from internal errors ([#1115](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1115), [@sunnylovestiramisu](https://github.com/sunnylovestiramisu))
- Add debugging log for the mapping of a PD name to /dev/* path ([#1115](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1115), [@sunnylovestiramisu](https://github.com/sunnylovestiramisu))


# v1.8.1 - Changelog since v1.8.0

## Changes by Kind

### Bug or Regression

- Add udevadm binary in the container image. ([#1097](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1097), [@mattcary](https://github.com/mattcary))

## Dependencies

_Nothing has changed._

# v1.8.0 - Changelog since v.1.7.3

## Changes by Kind

### Feature

- Add support for setting snapshot labels ([#1017](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1017), [@sagor999](https://github.com/sagor999))
- Go builder updated from 1.18.4 to 1.19.1. ([#1048](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1048), [@kon-angelo](https://github.com/kon-angelo))

### Bug or Regression

- Disable devices in node unstage prior to detaching. ([#1051](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1051), [@mattcary](https://github.com/mattcary))
- Enforce implicit pagination limit of 500 of the ListVolumesResponse#Entry field when ListVolumesRequest#max_entries is not set ([#999](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/999), [@pwschuurman](https://github.com/pwschuurman))
- Fixed issue where Regional disks are repeatedly queued for re-attaching and consuming api quota ([#1050](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1050), [@leiyiz](https://github.com/leiyiz))

### Uncategorized

- Migrate from github.com/golang/protobuf to google.golang.org/protobuf ([#1027](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1027), [@leiyiz](https://github.com/leiyiz))
