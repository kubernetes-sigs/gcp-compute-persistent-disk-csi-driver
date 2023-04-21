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

