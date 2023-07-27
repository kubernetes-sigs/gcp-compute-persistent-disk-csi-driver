# v1.8.8 - Changelog since v1.8.7

## Changes by Kind

### Uncategorized

- Add disk type for all operations metrics. ([#1297](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1297), [@k8s-infra-cherrypick-robot](https://github.com/k8s-infra-cherrypick-robot))
- Fix provisioned-iops-on-create passing logic ([#1284](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1284), [@sunnylovestiramisu](https://github.com/sunnylovestiramisu))


# v1.8.7 - Changelog since v1.8.6

### Bug or Regression

- Fix provisioned-iops-on-create passing logic([#1284](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1284))
- Bugfix for empty disk type being registered in metric for Create volume function. ([#1269](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1269), [@k8s-infra-cherrypick-robot](https://github.com/k8s-infra-cherrypick-robot))

### Cleanup

- #1079: Add provisionedIops for pd-extreme
  #1101: Add provisionedThroughput for hyperdisk
  #1240: Change iops params directly convert string to int64 ([#1241](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1241), [@sunnylovestiramisu](https://github.com/sunnylovestiramisu))
- #1150: fix bug where volume cloning topology requirements are
  #1232: Use errors.As so we can detect wrapped errors, and check for
  #1227: Adding new metric pdcsi_operation_errors to fetch error ([#1244](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1244), [@sunnylovestiramisu](https://github.com/sunnylovestiramisu))
- Update go version to 1.19.10 ([#1271](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1271), [@saikat-royc](https://github.com/saikat-royc))

# v1.8.6 - Changelog since v1.8.5

## Changes by Kind

### Cleanup

- Updates error message to be more user friendly when PD CSI Driver encounters an disk type UNSUPPORTED_OPERATION on ControllerPublishVolume ([#1223](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1223), [@pwschuurman](https://github.com/pwschuurman))

# v1.8.5 - Changelog since v1.8.4

## Changes by Kind

### Bug or Regression

- Add missing libraries, libbsd and libmd, that are dependencies for XFS volume expansion. ([#1204](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1204), [@nberlee](https://github.com/nberlee))

# v1.8.4 - Changelog since v1.8.2


## Changes by Kind

### Other (Cleanup or Flake)

- go version updates ([#1158](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1158), [@saikat-royc](https://github.com/saikat-royc))
- Fix for CVEs - update base image ([#1162](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1162), [@sunnylovestiramisu](https://github.com/sunnylovestiramisu))
- Fix missing shared library libedit.so.2 caused from updating base image in [#1162](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1162) ([#1177](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1177), [@sunnylovestiramisu ](https://github.com/sunnylovestiramisu))

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
