# v1.1.0 - Changelog since v1.0.0

## Changes by Kind

## Feature

- Improved Windows Support
  - Update driver to use CSI proxy beta for Windows ([#607](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/607), [@jingxu97](https://github.com/jingxu97))
  - Add volume expansion support for Windows in GCE PD CSI driver ([#637](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/637), [@jingxu97](https://github.com/jingxu97))
  - Add defensive check for Windows. GCE PD CSI driver only support ntfs for Windows. If other fstype is passed, return error. ([#641](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/641), [@jingxu97](https://github.com/jingxu97))
  - Modify NodeUnstageVolume call for Windows to use csi_proxy dismount call. With CSI proxy v0.2.2+, this will also result in flush of data cache before mount point removal. ([#633](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/633), [@jingxu97](https://github.com/jingxu97))
  - Add VolumeStats for Windows ([#627](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/627), [@jingxu97](https://github.com/jingxu97))

## Bug or Regression

- Add PSP for the controller Deployment  ([#623](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/623), [@george-angel](https://github.com/george-angel))
- Update GCE PD CSI Driver Docker base image to `k8s.gcr.io/build-image/debian-base-amd64:v2.1.3` (previously `gcr.io/google-containers/debian-base-amd64:v2.0.0`) to address CVEs. ([#596](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/596), [@saad-ali](https://github.com/saad-ali))
  - Also cherry picked to 1.0.1.

## Tests

- PD CSI e2e test infra should take GKE node version as an optional input argument. ([#603](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/603), [@saikat-royc](https://github.com/saikat-royc))
- Collect managed pd csi driver logs from node ([#619](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/619), [@saikat-royc](https://github.com/saikat-royc))
- Enable dump GKE node logs ([#635](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/635), [@saikat-royc](https://github.com/saikat-royc))
- Enable volume expansion test for GKE managed driver ([#584](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/584), [@saikat-royc](https://github.com/saikat-royc))
- Provide a knob to run intree and csi plugin tests ([#629](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/629), [@saikat-royc](https://github.com/saikat-royc))
- Fix CI script focus string ([#630](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/630), [@saikat-royc](https://github.com/saikat-royc))
- Build only linux container image for tests on Linux  ([#636](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/636), [@jingxu97](https://github.com/jingxu97))

## Dependencies

### Added
- google.golang.org/protobuf: v1.25.0

### Changed
- github.com/golang/protobuf: [v1.3.4 → v1.4.1](https://github.com/golang/protobuf/compare/v1.3.4...v1.4.1)
- github.com/google/go-cmp: [v0.3.1 → v0.5.0](https://github.com/google/go-cmp/compare/v0.3.1...v0.5.0)
- github.com/kubernetes-csi/csi-proxy/client: [9eff164 → v0.2.1](https://github.com/kubernetes-csi/csi-proxy/client/compare/9eff164...v0.2.1)
- golang.org/x/mod: c90efee → 4bf6d31
- golang.org/x/tools: 6862ede → 5eefd05
- golang.org/x/xerrors: 1b5146a → 9bdfabe
- google.golang.org/genproto: 6bbd007 → cb27e3a
- honnef.co/go/tools: v0.0.1-2020.1.3 → v0.0.1-2019.2.2
- mvdan.cc/xurls/v2: v2.1.0 → v2.0.0

### Removed
- golang.org/x/tools/gopls: v0.3.3
