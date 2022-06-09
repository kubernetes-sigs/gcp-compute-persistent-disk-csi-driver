# Google Compute Engine Persistent Disk CSI Driver

WARNING: Manual deployment of this driver to your GKE cluster is not recommended. Instead users should use GKE to automatically deploy and manage the GCE PD CSI Driver (see [GKE Docs](https://cloud.google.com/kubernetes-engine/docs/how-to/persistent-volumes/gce-pd-csi-driver)).

DISCLAIMER: Manual deployment of the driver to your cluster is not officially supported by Google.

The Google Compute Engine Persistent Disk CSI Driver is a
[CSI](https://github.com/container-storage-interface/spec/blob/master/spec.md)
Specification compliant driver used by Container Orchestrators to manage the
lifecycle of Google Compute Engine Persistent Disks.

## Project Status

Status: GA
Latest stable image: `k8s.gcr.io/cloud-provider-gcp/gcp-compute-persistent-disk-csi-driver:v1.7.1`

### Test Status

#### Kubernetes Integration

| Driver Version | Kubernetes Version | Test Status |
|----------------|--------------------|-------------|
| HEAD Latest | HEAD | [<img alt="Test Status" src="https://testgrid.k8s.io/q/summary/provider-gcp-compute-persistent-disk-csi-driver/Kubernetes%20Master%20Driver%20Latest/tests_status" />](https://testgrid.k8s.io/provider-gcp-compute-persistent-disk-csi-driver#Kubernetes%20Master%20Driver%20Latest) |
| 0.7.x stable | HEAD | [<img alt="Test Status" src="https://testgrid.k8s.io/q/summary/provider-gcp-compute-persistent-disk-csi-driver/Kubernetes%20Master%20Driver%20Release%200.7/tests_status" />](https://https://testgrid.k8s.io/provider-gcp-compute-persistent-disk-csi-driver#Kubernetes%20Master%20Driver%20Release%200.7) |
| HEAD Latest | HEAD (Migration ON) | [<img alt="Test Status" src="https://testgrid.k8s.io/q/summary/provider-gcp-compute-persistent-disk-csi-driver/Migration%20Kubernetes%20Master%20Driver%20Latest/tests_status" />](https://testgrid.k8s.io/provider-gcp-compute-persistent-disk-csi-driver#Migration%20Kubernetes%20Master%20Driver%20Latest) |
| HEAD stable-master | HEAD (Migration ON) | [<img alt="Test Status" src="https://testgrid.k8s.io/q/summary/provider-gcp-compute-persistent-disk-csi-driver/Migration%20Kubernetes%20Master%20Driver%20Stable/tests_status" />](https://testgrid.k8s.io/provider-gcp-compute-persistent-disk-csi-driver#Migration%20Kubernetes%20Master%20Driver%20Stable) |

### CSI Compatibility

This plugin is compatible with CSI versions [v1.2.0](https://github.com/container-storage-interface/spec/blob/v1.2.0/spec.md), [v1.1.0](https://github.com/container-storage-interface/spec/blob/v1.1.0/spec.md), and [v1.0.0](https://github.com/container-storage-interface/spec/blob/v1.0.0/spec.md)

### Kubernetes Compatibility
The following table captures the compatibility matrix of the core persistent disk driver binary
`gke.gcr.io/gcp-compute-persistent-disk-csi-driver`

| GCE PD CSI Driver\Kubernetes Version | 1.17+ |
|--------------------------------------|-------|
| v0.7.x (beta)                        | yes   |
| v1.0.x (ga)                          | yes   |
| dev                                  | yes   |

The manifest bundle which captures all the driver components (driver pod which includes the containers csi-provisioner, csi-resizer, csi-snapshotter, gce-pd-driver, csi-driver-registrar;
csi driver object, rbacs, pod security policies etc) can be picked up from the master branch [overlays](deploy/kubernetes/overlays) directory. We structure the overlays directory, per minor version of kubernetes because not all driver components can be used with all kubernetes versions. 

Example:

`stable-1-21` overlays bundle can be used to deploy all the components of the driver on kubernetes 1.21.

`stable-master` overlays bundle can be used to deploy all the components of the driver on kubernetes master.

For more details about per k8s minor version overlays, please check this [doc](docs/release/overlays.md)

### Known Issues

See Github [Issues](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/issues)

## Plugin Features

### CreateVolume Parameters

| Parameter        | Values                    | Default       | Description                                                                                        |
|------------------|---------------------------|---------------|----------------------------------------------------------------------------------------------------|
| type             | Any PD type (see [GCP documentation](https://cloud.google.com/compute/docs/disks#disk-types)), eg `pd-ssd` `pd-balanced` | `pd-standard` | Type allows you to choose between standard Persistent Disks  or Solid State Drive Persistent Disks |
| replication-type | `none` OR `regional-pd`   | `none`        | Replication type allows you to choose between Zonal Persistent Disks or Regional Persistent Disks  |
| disk-encryption-kms-key | Fully qualified resource identifier for the key to use to encrypt new disks. | Empty string. | Encrypt disk using Customer Managed Encryption Key (CMEK). See [GKE Docs](https://cloud.google.com/kubernetes-engine/docs/how-to/using-cmek#create_a_cmek_protected_attached_disk) for details. |
| labels           | `key1=value1,key2=value2` |               | Labels allow you to assign custom [GCE Disk labels](https://cloud.google.com/compute/docs/labeling-resources). |

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

## Further Documentation

[Local Development](docs/local-development.md)

For releasing new versions of this driver, googlers should consult [go/pdcsi-oss-release-process](go/pdcsi-oss-release-process).

### Kubernetes

[User Guides](docs/kubernetes/user-guides)

[Driver Development](docs/kubernetes/development.md)
