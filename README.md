WARNING: This driver is beta and should not be used in performance critical applications

DISCLAIMER: This is not an officially supported Google product

# GCP Compute Persistent Disk CSI Driver

The GCP Compute Persistent Disk CSI Driver is a
[CSI](https://github.com/container-storage-interface/spec/blob/master/spec.md)
Specification compliant driver used by Container Orchestrators to manage the
lifecycle of Google Compute Engine Persistent Disks.

## Project Status
Status: Beta
Latest stable image: `gcr.io/gke-release/gcp-compute-persistent-disk-csi-driver:v0.4.0-gke.0`

### Test Status

#### Kubernetes Integration

[<img alt="Test Status" src="https://testgrid.k8s.io/q/summary/sig-gcp-compute-persistent-disk-csi-driver/Kubernetes%20Master%20Driver%20Stable/tests_status" />](https://testgrid.k8s.io/sig-gcp-compute-persistent-disk-csi-driver#Kubernetes%20Master%20Driver%20Stable)

### CSI Compatibility

This plugin is compatible with CSI versions [v1.0.0](https://github.com/container-storage-interface/spec/blob/v1.0.0/spec.md)

### Kubernetes Compatibility

| GCE PD CSI Driver\Kubernetes Version | 1.10.5 - 1.11 | 1.12 | 1.13 | 1.14+
|--------------------------------------|---------------|------|------|------|
| v0.1.0.alpha                         | yes           | no   | no   | no   |
| v0.2.0 (alpha)                       | no            | yes  | no   | no   |
| v0.3.0 (beta)                        | no            | no   | yes  | yes  |
| v0.4.0 (beta)                        | no            | no   | yes  | yes  |
| dev                                  | no            | no   | no   | yes  |

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

### Kubernetes Beta Features

* Topology: Requires K8s 1.14+ on Master and Nodes and PD driver v0.5.0+

### Kubernetes Alpha Features

* Snapshots: Requires K8s 1.13+ on Master and PD driver v0.3.0+ with the alpha
  overlay

### Future Features

See Github [Issues](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/issues)

## Further Documentation

[Local Development](docs/local-development.md)

### Kubernetes

[User Guides](docs/kubernetes/user-guides)

[Driver Development](docs/kubernetes/development.md)
