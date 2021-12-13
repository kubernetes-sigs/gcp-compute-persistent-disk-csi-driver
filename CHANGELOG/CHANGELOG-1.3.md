# v1.3.5 - Changelog since v1.3.4

## Changes by Kind

### Feature

- Add attach/detach back off (#847, @lizhuqi)
- Add volume cloning feature (#854, @amacaskill)
- Change to distroless base image (#870, @amacaskill)

### Bug or Regression

- Cherrypick #850 - Update go builder to 1.17 (#851, @saikat-royc)

### Uncategorized

- Doc and Image update for 1.3.4 release (#860, @saikat-royc)

## Dependencies

### Added
_Nothing has changed._

### Changed
_Nothing has changed._

### Removed
_Nothing has changed._

# v1.3.4 - Changelog since v1.3.3

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

# v1.3.3 - Changelog since v1.3.1

## Changes by Kind

### Bug or Regression

- Update debian image to buster-1.9.0. ([#841](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/841), [@mattcary](https://github.com/mattcary))

## Dependencies

### Added
_Nothing has changed._

### Changed
_Nothing has changed._

### Removed
_Nothing has changed._

# v1.3.1 - Changelog since v1.3.0

### Issues

- Fixes issue where `ControllerPublishVolume` is called repeatly if gke nodes are in different cloud zones than the gke controller ([#817](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/817), [@leiyiz](https://github.com/leiyiz))

## Dependencies

### Added
_Nothing has changed._

### Changed
_Nothing has changed._

### Removed
_Nothing has changed._

# v1.3.0 - Changelog since v1.2.2

### Feature

- A new `k8s-tag-cluster-id` command line option has been added. If specified, the resulting PD disk will be labeled with "kubernetes_io_cluster_<cluster ID>": "owned". ([#693](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/693), [@tsmetana](https://github.com/tsmetana))
- Add cloudbuild config to build gcp-compute-persistent-disk-csi-driver image ([#724](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/724), [@cpanato](https://github.com/cpanato))
- Added Support for Windows Server 2004 and 20H2. ([#691](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/691), [@jeremyje](https://github.com/jeremyje))
- Bumped csi-proxy client library to v1.0.0 ([#738](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/738), [@mauriciopoppe](https://github.com/mauriciopoppe))
- It is now possible to access snapshots and volumes across different projects. ([#782](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/782), [@christian-roggia](https://github.com/christian-roggia))
- Updating the following image versions in stable deployment specs:
  - csi-provisioner: v2.1.0
  - csi-attacher: v3.1.0
  - csi-resizer: v1.1.0
  - csi-snapshotter: v3.0.3
  - csi-node-driver-registrar: v2.1.0
  - Adding a liveness probe to restart a sidecar if it fails leader election health check. ([#699](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/699), [@verult](https://github.com/verult))
- Users will be able to set the storage locations for snapshots by specifying them in the snapshot class. ([#793](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/793), [@TeweiLuo](https://github.com/TeweiLuo))
- Disk labels support via CreateVolume (and hence StorageClass) parameters ([#718](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/718), [@mattcary](https://github.com/mattcary))

### Documentation

- Documentation for overlays ([#708](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/708), [@saikat-royc](https://github.com/saikat-royc))
- Update README for overlays ([#715](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/715), [@saikat-royc](https://github.com/saikat-royc))

### Failing Test

- V1 CSIDriver resources are deployed for 1.18+ clusters. ([#783](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/783), [@mattcary](https://github.com/mattcary))

### Bug or Regression

- Do not run controller service in node. ([#702](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/702), [@mattcary](https://github.com/mattcary))
- Fix a bug that CreateVolume should round up the request_bytes. ([#684](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/684), [@Jiawei0227](https://github.com/Jiawei0227))
- It is now possible to mount a volume with XFS filesystem and its restored snapshot. ([#788](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/788), [@jsafrane](https://github.com/jsafrane))

### Other (Cleanup or Flake)

- Emit GKE component version metric ([#719](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/719), [@saikat-royc](https://github.com/saikat-royc))

### Uncategorized

- Remove probe logging to reduce noise ([#682](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/682), [@Jiawei0227](https://github.com/Jiawei0227))

## Dependencies

### Added
- k8s.io/klog/v2: v2.4.0
- k8s.io/mount-utils: v0.20.6

### Changed
- github.com/Microsoft/go-winio: [v0.4.14 → v0.4.16](https://github.com/Microsoft/go-winio/compare/v0.4.14...v0.4.16)
- github.com/go-logr/logr: [v0.1.0 → v0.2.0](https://github.com/go-logr/logr/compare/v0.1.0...v0.2.0)
- github.com/kr/pretty: [v0.1.0 → v0.2.0](https://github.com/kr/pretty/compare/v0.1.0...v0.2.0)
- github.com/kubernetes-csi/csi-proxy/client: [v0.2.2 → v1.0.0](https://github.com/kubernetes-csi/csi-proxy/client/compare/v0.2.2...v1.0.0)
- github.com/pkg/errors: [v0.8.1 → v0.9.1](https://github.com/pkg/errors/compare/v0.8.1...v0.9.1)
- github.com/stretchr/testify: [v1.4.0 → v1.6.1](https://github.com/stretchr/testify/compare/v1.4.0...v1.6.1)
- gopkg.in/check.v1: 788fd78 → 41f04d3
- gopkg.in/yaml.v3: 674ba3e → 9f266ea
- k8s.io/utils: a9aa75a → 67b214c

### Removed
_Nothing has changed._
