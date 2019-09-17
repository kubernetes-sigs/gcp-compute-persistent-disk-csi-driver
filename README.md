# GCP Compute Persistent Disk CSI Driver

WARNING: This driver is beta and should not be used in performance critical applications

DISCLAIMER: This is not an officially supported Google product

The GCP Compute Persistent Disk CSI Driver is a
[CSI](https://github.com/container-storage-interface/spec/blob/master/spec.md)
Specification compliant driver used by Container Orchestrators to manage the
lifecycle of Google Compute Engine Persistent Disks.

## Project Status

Status: Beta
Latest stable image: `gcr.io/gke-release/gcp-compute-persistent-disk-csi-driver:v0.5.1-gke.0`

### Test Status

#### Kubernetes Integration

| Driver Version | Kubernetes Version | Test Status |
|----------------|--------------------|-------------|
| HEAD Latest | HEAD | [<img alt="Test Status" src="https://testgrid.k8s.io/q/summary/provider-gcp-compute-persistent-disk-csi-driver/Kubernetes%20Master%20Driver%20Latest/tests_status" />](https://testgrid.k8s.io/provider-gcp-compute-persistent-disk-csi-driver#Kubernetes%20Master%20Driver%20Latest) |
| 0.5.x Stable | HEAD | [<img alt="Test Status" src="https://testgrid.k8s.io/q/summary/provider-gcp-compute-persistent-disk-csi-driver/Kubernetes%20Master%20Driver%20Release%200.5/tests_status" />](https://testgrid.k8s.io/provider-gcp-compute-persistent-disk-csi-driver#Kubernetes%20Master%20Driver%20Release%200.5) |
| 0.4.x Stable | HEAD | [<img alt="Test Status" src="https://testgrid.k8s.io/q/summary/provider-gcp-compute-persistent-disk-csi-driver/Kubernetes%20Master%20Driver%20Release%200.4/tests_status" />](https://testgrid.k8s.io/provider-gcp-compute-persistent-disk-csi-driver#Kubernetes%20Master%20Driver%20Release%200.4) |
| 0.4.x Stable | 1.13.5 | [<img alt="Test Status" src="https://testgrid.k8s.io/q/summary/provider-gcp-compute-persistent-disk-csi-driver/Kubernetes%20v1.13.5%20Driver%20Release%200.4/tests_status" />](https://testgrid.k8s.io/provider-gcp-compute-persistent-disk-csi-driver#Kubernetes%20v1.13.5%20Driver%20Release%200.4) |
| 0.3.x Stable | HEAD | [<img alt="Test Status" src="https://testgrid.k8s.io/q/summary/provider-gcp-compute-persistent-disk-csi-driver/Kubernetes%20Master%20Driver%20Release%200.3/tests_status" />](https://testgrid.k8s.io/provider-gcp-compute-persistent-disk-csi-driver#Kubernetes%20Master%20Driver%20Release%200.3) |
| HEAD Latest | HEAD (Migration ON) | [<img alt="Test Status" src="https://testgrid.k8s.io/q/summary/provider-gcp-compute-persistent-disk-csi-driver/Migration%20Kubernetes%20Master%20Driver%20Latest/tests_status" />](https://testgrid.k8s.io/provider-gcp-compute-persistent-disk-csi-driver#Migration%20Kubernetes%20Master%20Driver%20Latest) |
| HEAD Stable | HEAD (Migration ON) | [<img alt="Test Status" src="https://testgrid.k8s.io/q/summary/provider-gcp-compute-persistent-disk-csi-driver/Migration%20Kubernetes%20Master%20Driver%20Stable/tests_status" />](https://testgrid.k8s.io/provider-gcp-compute-persistent-disk-csi-driver#Migration%20Kubernetes%20Master%20Driver%20Stable) |

### CSI Compatibility

This plugin is compatible with CSI versions [v1.0.0](https://github.com/container-storage-interface/spec/blob/v1.0.0/spec.md)

### Kubernetes Compatibility

| GCE PD CSI Driver\Kubernetes Version | 1.10.5 - 1.11 | 1.12 | 1.13 | 1.14 | 1.15+|
|--------------------------------------|---------------|------|------|------|------|
| v0.1.x.alpha                         | yes           | no   | no   | no   | no   |
| v0.2.x (alpha)                       | no            | yes  | no   | no   | no   |
| v0.3.x (beta)                        | no            | no   | yes  | yes  | yes  |
| v0.4.x (beta)                        | no            | no   | yes  | yes  | yes  |
| v0.5.x (beta)                        | no            | no   | no   | yes  | yes  |
| dev                                  | no            | no   | no   | no   | yes  |

### Known Issues

See Github [Issues](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/issues)

## Plugin Features

### CreateVolume Parameters

| Parameter        | Values                    | Default       | Description                                                                                        |
|------------------|---------------------------|---------------|----------------------------------------------------------------------------------------------------|
| type             | `pd-ssd` OR `pd-standard` | `pd-standard` | Type allows you to choose between standard Persistent Disks  or Solid State Drive Persistent Disks |
| replication-type | `none` OR `regional-pd`   | `none`        | Replication type allows you to choose between Zonal Persistent Disks or Regional Persistent Disks  |

### Topology

This driver supports only one topology key:
`topology.gke.io/zone`
that represents availability by zone.

### Features in Development

| Feature         | Stage | Min Kubernetes Master Version | Min Kubernetes Nodes Version | Min Driver Version | Deployment Overlay |
|-----------------|-------|-------------------------------|------------------------------|--------------------|--------------------|
| Topology        | Beta  | 1.14                          | 1.14                         | v0.5.0             | Stable             |
| Snapshots       | Alpha | 1.13                          | Any                          | v0.3.0             | Alpha              |
| Resize (Expand) | Alpha | 1.14                          | 1.14                         | dev                | Alpha              |

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
