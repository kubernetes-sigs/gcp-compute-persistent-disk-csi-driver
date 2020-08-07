# Google Compute Engine Persistent Disk CSI Driver

WARNING: Manual deployment of this driver to your GKE cluster is not recommended. Instead users should use GKE to automatically deploy and manage the GCE PD CSI Driver (see [GKE Docs](https://cloud.google.com/kubernetes-engine/docs/how-to/persistent-volumes/gce-pd-csi-driver)).

DISCLAIMER: Manual deployment of the driver to your cluster is not officially supported by Google.

The Google Compute Engine Persistent Disk CSI Driver is a
[CSI](https://github.com/container-storage-interface/spec/blob/master/spec.md)
Specification compliant driver used by Container Orchestrators to manage the
lifecycle of Google Compute Engine Persistent Disks.

## Project Status

Status: GA
Latest stable image: `gcr.io/gke-release/gcp-compute-persistent-disk-csi-driver:v1.0.0-gke.0`

### Test Status

#### Kubernetes Integration

| Driver Version | Kubernetes Version | Test Status |
|----------------|--------------------|-------------|
| HEAD Latest | HEAD | [<img alt="Test Status" src="https://testgrid.k8s.io/q/summary/provider-gcp-compute-persistent-disk-csi-driver/Kubernetes%20Master%20Driver%20Latest/tests_status" />](https://testgrid.k8s.io/provider-gcp-compute-persistent-disk-csi-driver#Kubernetes%20Master%20Driver%20Latest) |
| 0.5.x Stable | HEAD | [<img alt="Test Status" src="https://testgrid.k8s.io/q/summary/provider-gcp-compute-persistent-disk-csi-driver/Kubernetes%20Master%20Driver%20Release%200.5/tests_status" />](https://testgrid.k8s.io/provider-gcp-compute-persistent-disk-csi-driver#Kubernetes%20Master%20Driver%20Release%200.5) |
| 0.4.x Stable | 1.13.5 | [<img alt="Test Status" src="https://testgrid.k8s.io/q/summary/provider-gcp-compute-persistent-disk-csi-driver/Kubernetes%20v1.13.5%20Driver%20Release%200.4/tests_status" />](https://testgrid.k8s.io/provider-gcp-compute-persistent-disk-csi-driver#Kubernetes%20v1.13.5%20Driver%20Release%200.4) |
| HEAD Latest | HEAD (Migration ON) | [<img alt="Test Status" src="https://testgrid.k8s.io/q/summary/provider-gcp-compute-persistent-disk-csi-driver/Migration%20Kubernetes%20Master%20Driver%20Latest/tests_status" />](https://testgrid.k8s.io/provider-gcp-compute-persistent-disk-csi-driver#Migration%20Kubernetes%20Master%20Driver%20Latest) |
| HEAD Stable | HEAD (Migration ON) | [<img alt="Test Status" src="https://testgrid.k8s.io/q/summary/provider-gcp-compute-persistent-disk-csi-driver/Migration%20Kubernetes%20Master%20Driver%20Stable/tests_status" />](https://testgrid.k8s.io/provider-gcp-compute-persistent-disk-csi-driver#Migration%20Kubernetes%20Master%20Driver%20Stable) |

### CSI Compatibility

This plugin is compatible with CSI versions [v1.2.0](https://github.com/container-storage-interface/spec/blob/v1.2.0/spec.md), [v1.1.0](https://github.com/container-storage-interface/spec/blob/v1.1.0/spec.md), and [v1.0.0](https://github.com/container-storage-interface/spec/blob/v1.0.0/spec.md)

### Kubernetes Compatibility

| GCE PD CSI Driver\Kubernetes Version | 1.10.5 - 1.11 | 1.12 | 1.13 | 1.14 | 1.15 | 1.16+ | 1.17+ |
|--------------------------------------|---------------|------|------|------|------|-------|-------|
| v0.1.x.alpha                         | yes           | no   | no   | no   | no   | no    | no    |
| v0.2.x (alpha)                       | no            | yes  | no   | no   | no   | no    | no    |
| v0.3.x (beta)                        | no            | no   | yes  | yes  | yes  | yes   | yes   |
| v0.4.x (beta)                        | no            | no   | yes  | yes  | yes  | yes   | yes   |
| v0.5.x (beta)                        | no            | no   | no   | yes  | yes  | yes   | yes   |
| v0.6.x (beta)                        | no            | no   | no   | yes  | yes  | yes   | yes   |
| v0.7.x (beta)                        | no            | no   | no   | no   | no   | yes   | yes   |
| v1.0.x (ga)                          | no            | no   | no   | no   | no   | no    | yes   |
| dev                                  | no            | no   | no   | no   | no   | yes   | yes   |

### Known Issues

See Github [Issues](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/issues)

## Plugin Features

### CreateVolume Parameters

| Parameter        | Values                    | Default       | Description                                                                                        |
|------------------|---------------------------|---------------|----------------------------------------------------------------------------------------------------|
| type             | `pd-ssd` OR `pd-standard` | `pd-standard` | Type allows you to choose between standard Persistent Disks  or Solid State Drive Persistent Disks |
| replication-type | `none` OR `regional-pd`   | `none`        | Replication type allows you to choose between Zonal Persistent Disks or Regional Persistent Disks  |
| disk-encryption-kms-key | Fully qualified resource identifier for the key to use to encrypt new disks. | Empty string. | Encrypt disk using Customer Managed Encryption Key (CMEK). See [GKE Docs](https://cloud.google.com/kubernetes-engine/docs/how-to/using-cmek#create_a_cmek_protected_attached_disk) for details. |

### Topology

This driver supports only one topology key:
`topology.gke.io/zone`
that represents availability by zone (e.g. `us-central1-c`, etc.).

### Features in Development

| Feature         | Stage | Min Kubernetes Master Version | Min Kubernetes Nodes Version | Min Driver Version | Deployment Overlay |
|-----------------|-------|-------------------------------|------------------------------|--------------------|--------------------|
| Snapshots       | Alpha | 1.13                          | Any                          | v0.3.0             | Alpha              |
| Snapshots       | Beta  | 1.17                          | Any                          | v1.0.0             | Stable             |
| Resize (Expand) | Alpha | 1.14                          | 1.14                         | v0.6.0             | Alpha              |
| Resize (Expand) | Beta  | 1.16                          | 1.16                         | v0.7.0             | Stable             |

### Future Features

See Github [Issues](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/issues)

## Driver Deployment
As part of the deployment process, the driver is deployed in a newly created namespace by default. The namespace will be deleted as part of the cleanup process.

Controller-level and node-level deployments will both have priorityClassName set, and the corresponding priority value is close to the maximum possible for user-created PriorityClasses.

## Further Documentation

[Local Development](docs/local-development.md)

### Kubernetes

[User Guides](docs/kubernetes/user-guides)

[Driver Development](docs/kubernetes/development.md)
