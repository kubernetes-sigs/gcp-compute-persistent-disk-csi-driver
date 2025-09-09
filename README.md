# Google Compute Engine Persistent Disk CSI Driver

WARNING: Manual deployment of this driver to your GKE cluster is not recommended. Instead users should use GKE to automatically deploy and manage the GCE PD CSI Driver (see [GKE Docs](https://cloud.google.com/kubernetes-engine/docs/how-to/persistent-volumes/gce-pd-csi-driver)).

DISCLAIMER: Manual deployment of the driver to your cluster is not officially supported by Google.

The Google Compute Engine Persistent Disk CSI Driver is a
[CSI](https://github.com/container-storage-interface/spec/blob/master/spec.md)
Specification compliant driver used by Container Orchestrators to manage the
lifecycle of Google Compute Engine Persistent Disks.

## Project Status

Status: GA
Latest stable image: `registry.k8s.io/cloud-provider-gcp/gcp-compute-persistent-disk-csi-driver:v1.20.0`

### Test Status

#### Kubernetes Integration

| Driver Version | Kubernetes Version | Test Status |
|----------------|--------------------|-------------|
| HEAD Latest | HEAD | [<img alt="Test Status" src="https://testgrid.k8s.io/q/summary/provider-gcp-compute-persistent-disk-csi-driver/Kubernetes%20Master%20Driver%20Latest/tests_status" />](https://testgrid.k8s.io/provider-gcp-compute-persistent-disk-csi-driver#Kubernetes%20Master%20Driver%20Latest) |
| HEAD stable-master | HEAD (Migration ON) | [<img alt="Test Status" src="https://testgrid.k8s.io/q/summary/provider-gcp-compute-persistent-disk-csi-driver/Kubernetes%20Master%20Driver%20Latest%20Release%20Candidate/tests_status" />](https://testgrid.k8s.io/provider-gcp-compute-persistent-disk-csi-driver#Kubernetes%20Master%20Driver%20Latest%20Release%20Candidate) |

### CSI Compatibility

This plugin is compatible with CSI versions [v1.2.0](https://github.com/container-storage-interface/spec/blob/v1.2.0/spec.md), [v1.1.0](https://github.com/container-storage-interface/spec/blob/v1.1.0/spec.md), and [v1.0.0](https://github.com/container-storage-interface/spec/blob/v1.0.0/spec.md)

### Kubernetes Version Recommendations

The latest stable of this driver is recommended for the latest stable Kubernetes
version. For previous Kubernetes versions, we recommend the following driver
versions.

| Kubernetes Version | PD CSI Driver Version |
|--------------------|-----------------------|
| HEAD               | v1.13.x               |
| 1.29               | v1.12.x               |
| 1.28               | v1.12.x               |
| 1.27               | v1.10.x               |

The manifest bundle which captures all the driver components (driver pod which includes the containers csi-provisioner, csi-resizer, csi-snapshotter, gce-pd-driver, csi-driver-registrar;
csi driver object, rbacs, pod security policies etc) for the lastest stable
release can be picked up from the master branch [overlays](deploy/kubernetes/overlays) directory.

### Known Issues

See Github [Issues](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/issues)

## Plugin Features

### CreateVolume Parameters

| Parameter                   | Values                    | Default       | Description                                                                                        |
|-----------------------------|---------------------------|---------------|----------------------------------------------------------------------------------------------------|
| type                        | Any PD type (see [GCP documentation](https://cloud.google.com/compute/docs/disks#disk-types)), eg `pd-ssd` `pd-balanced` | `pd-standard` | Type allows you to choose between standard Persistent Disks  or Solid State Drive Persistent Disks |
| replication-type            | `none` OR `regional-pd`   | `none`        | Replication type allows you to choose between Zonal Persistent Disks or Regional Persistent Disks  |
| disk-encryption-kms-key     | Fully qualified resource identifier for the key to use to encrypt new disks. | Empty string. | Encrypt disk using Customer Managed Encryption Key (CMEK). See [GKE Docs](https://cloud.google.com/kubernetes-engine/docs/how-to/using-cmek#create_a_cmek_protected_attached_disk) for details. |
| labels                      | `key1=value1,key2=value2` |               | Labels allow you to assign custom [GCE Disk labels](https://cloud.google.com/compute/docs/labeling-resources). |
| provisioned-iops-on-create  | string (int64 format). Values typically between 10,000 and 120,000 |               | Indicates how many IOPS to provision for the disk. See the [Extreme persistent disk documentation](https://cloud.google.com/compute/docs/disks/extreme-persistent-disk) for details, including valid ranges for IOPS. |
| provisioned-throughput-on-create  | string (int64 format). Values typically between 1 and 7,124 mb per second |               | Indicates how much throughput to provision for the disk. See the [hyperdisk documentation]([TBD](https://cloud.google.com/kubernetes-engine/docs/how-to/persistent-volumes/hyperdisk#create)) for details, including valid ranges for throughput. |
| resource-tags               | `<parent_id1>/<tag_key1>/<tag_value1>,<parent_id2>/<tag_key2>/<tag_value2>` |               | Resource tags allow you to attach user-defined tags to each Compute Disk, Image and Snapshot. See [Tags overview](https://cloud.google.com/resource-manager/docs/tags/tags-overview), [Creating and managing tags](https://cloud.google.com/resource-manager/docs/tags/tags-creating-and-managing). |
| use-allowed-disk-topologies | `true` or `false`         | `false`       | Allows the use of specific disk topologies for provisioning. Must be used in combination with the `--disk-topology=true` flag on PDCSI binary to yield disk support labels in PV NodeAffinity blocks. |

### Topology

This driver supports only one topology key:
`topology.gke.io/zone`
that represents availability by zone (e.g. `us-central1-c`, etc.).

### CSI Windows Support

GCE PD driver starts to support CSI Windows with [CSI Proxy] (https://github.com/kubernetes-csi/csi-proxy). It requires csi-proxy.exe to be installed on every Windows node. Please see more details in CSI Windows page (docs/kubernetes/user-guides/windows.md)

### Features in Development

| Feature         | Stage | Min Kubernetes Master Version | Min Kubernetes Nodes Version | Min Driver Version | Deployment Overlay |
|-----------------|-------|-------------------------------|------------------------------|--------------------|--------------------|
| Snapshots       | GA    | 1.17                          | Any                          | v1.0.0             | stable-1-21, stable-1-22, stable-1-23, stable-master |
| Clones          | GA    | 1.18                          | Any                          | v1.4.0             | stable-1-21, stable-1-22, stable-1-23, stable-master |
| Resize (Expand) | Beta  | 1.16                          | 1.16                         | v0.7.0             | stable-1-21, stable-1-22, stable-1-23, stable-master |
| Windows*        | GA    | 1.19                          | 1.19                         | v1.1.0             | stable-1-21, stable-1-22, stable-1-23, stable-master |

\* For Windows, it is recommended to use this driver with CSI proxy v0.2.2+. The master version of driver requires disk v1beta2 group, which is only available in CSI proxy v0.2.2+

### Future Features

See Github [Issues](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/issues)

## Driver Deployment
As part of the deployment process, the driver is deployed in a newly created namespace by default. The namespace will be deleted as part of the cleanup process.

Controller-level and node-level deployments will both have priorityClassName set, and the corresponding priority value is close to the maximum possible for user-created PriorityClasses.

## Notes on filesystems

As noted in [GCP PD documentation](https://cloud.google.com/kubernetes-engine/docs/how-to/persistent-volumes/gce-pd-csi-driver), `ext4` and `xfs` are officially supported. `btrfs` support is experimental:
- As of writing, Ubuntu VM images support btrfs, but [COS does not](https://cloud.google.com/container-optimized-os/docs/concepts/supported-filesystems).

`btrfs` filesystem accepts the following "special" mount options and the sysfs paths they target:

| Setting                                          | Sysfs path                                                        | Value          | Default                 | Supported on | Notes |
|--------------------------------------------------|-------------------------------------------------------------------|----------------|-------------------------|----------------------------------------------|-------|
| `btrfs-allocation-data-bg_reclaim_threshold`     | `/sys/fs/btrfs/FS-UUID/allocation/data/bg_reclaim_threshold`      | 0–99 (percent) | 0 (disabled)            | Linux v5.19+ | Triggers background reclaim for DATA block groups when usage drops to the threshold. |
| `btrfs-allocation-metadata-bg_reclaim_threshold` | `/sys/fs/btrfs/FS-UUID/allocation/metadata/bg_reclaim_threshold`  | 0–99 (percent) | 0 (disabled)            | Linux v5.19+ | Same as above, for METADATA block groups. |
| `btrfs-allocation-data-dynamic_reclaim`          | `/sys/fs/btrfs/FS-UUID/allocation/data/dynamic_reclaim`           | `0` or `1`     | 0 (off)                 | Linux v6.11+ | Heuristic reclaim that addresses [some concerns](https://web.git.kernel.org/pub/scm/linux/kernel/git/torvalds/linux.git/commit/?id=f5ff64ccf7bb7274ed66b0d835b2f6ae10af5d7a) of `bg_reclaim_threshold`. |
| `btrfs-bdi-read_ahead_kb`                        | `/sys/fs/btrfs/FS-UUID/bdi/read_ahead_kb`                         | integer kB ≥ 0 | kernel/device dependent | Linux v5.9+  | Per-BDI readahead. Powers of two are commonly used. |

See more in the [in btrfs docs](https://btrfs.readthedocs.io/en/latest/ch-sysfs.html#uuid-allocations-data-metadata-system).

## Further Documentation

[Local Development](docs/kubernetes/development.md)

For releasing new versions of this driver, googlers should consult [go/pdcsi-oss-release-process](go/pdcsi-oss-release-process).

### Kubernetes

[User Guides](docs/kubernetes/user-guides)

[Driver Development](docs/kubernetes/development.md)
