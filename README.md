WARNING: This driver is in ALPHA currently. This means that there may be
potentially backwards compatability breaking changes moving forward. Do NOT use
this drive in a production environment in its current state.

DISCLAIMER: This is not an officially supported Google product

Kubernetes Note: setup-cluster.yaml depends on the existence of cluster-roles
system:csi-external-attacher and system:csi-external-provisioner which are in
Kubernetes version 1.10.5+

# GCP Compute Persistent Disk CSI Driver

The GCP Compute Persistent Disk CSI Driver is a
[CSI](https://github.com/container-storage-interface/spec/blob/master/spec.md)
Specification compliant driver used by Container Orchestrators to manage the
lifecycle of Google Compute Engine Persistent Disks.

## Project Status
Status: Alpha
Latest image: `gcr.io/google-containers/volume-csi/gcp-compute-persistent-disk-csi-driver:v0.1.0.alpha`

### CSI Compatibility
This plugin is compatible with CSI versions [v0.2.0](https://github.com/container-storage-interface/spec/blob/v0.2.0/spec.md) and [v0.3.0](https://github.com/container-storage-interface/spec/blob/v0.3.0/spec.md)

### Kubernetes Compatibility
This plugin can be used as-is beginning with Kubernetes v1.10.5

### Known Issues
See Github [Issues](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/issues)

## Plugin Features
### CreateVolume Parameters
| Parameter          | Values               | Default     | Description                                                                                                                 |
|--------------------|----------------------|-------------|-----------------------------------------------------------------------------------------------------------------------------|
| "type"             | pd-ssd OR pd-standard | pd-standard | Type allows you to choose between standard Persistent Disks  or Solid State Drive Persistent Disks                          |
| "replication-type" | none OR regional-pd   | none        | Replication type allows you to choose between standard zonal Persistent Disks or highly available Regional Persistent Disks |

### Future Features
See Github [Issues](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/issues)

### Topology
This driver supports only one topology key:
`com.google.topology/zone`
that represents availability by zone.

## Kubernetes User Guide
### Install Driver
1. [One-time per project] Create GCP service account for the CSI driver and set required roles
```
$ PROJECT=your-project-here                       # GCP project
$ GCE_PD_SA_NAME=my-gce-pd-csi-sa                 # Name of the service account to create
$ GCE_PD_SA_DIR=/my/safe/credentials/directory    # Directory to save the service account key
$ ./deploy/setup-project.sh
```

2. Deploy driver to Kubernetes Cluster
```
$ GCE_PD_SA_DIR=/my/safe/credentials/directory    # Directory to get the service account key
$ GCE_PD_DRIVER_VERSION=stable                    # Driver version to deploy
$ ./deploy/kubernetes/deploy-driver.sh
```

### Zonal Example
1. Create example Zonal Storage Class
```
$ kubectl apply -f ./examples/kubernetes/demo-zonal-sc.yaml
```

2. Create example PVC and Pod
```
$ kubectl apply -f ./examples/kubernetes/demo-pod.yaml
```

3. Verify PV is created and bound to PVC
```
$ kubectl get pvc
NAME      STATUS    VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
podpvc     Bound     pvc-e36abf50-84f3-11e8-8538-42010a800002   10Gi       RWO            csi-gce-pd     9s
```

4. Verify pod is created and in `RUNNING` state (it may take a few minutes to get to running state)
```
$ kubectl get pods
NAME                      READY     STATUS    RESTARTS   AGE
web-server                1/1       Running   0          1m
```

## Kubernetes Development

### Manual
To build and install a development version of the driver:
```
$ GCE_PD_CSI_STAGING_IMAGE=gcr.io/path/to/driver/image:dev   # Location to push dev image to
$ make push-container

# Modify controller.yaml and node.yaml in ./deploy/kubernetes/dev to use dev image
$ GCE_PD_DRIVER_VERSION=dev
$ ./deploy/kubernetes/deploy-driver.sh
```

To bring down driver:
```
$ ./deploy/kubernetes/delete-driver.sh
```

## Testing
Running E2E Tests:
```
$ PROJECT=my-project                               # GCP Project to run tests in
$ IAM_NAME=my-iam@project.iam.gserviceaccount.com  # Existing IAM Account with GCE PD CSI Driver Permissions
$ ./test/run-e2e-local.sh
```

Running Sanity Tests:
```
$ ./test/run-sanity.sh
```

Running Unit Tests:
```
$ ./test/run-unit.sh
```

## Dependency Management
Use [dep](https://github.com/golang/dep)
```
$ dep ensure
```

To modify dependencies or versions change `./Gopkg.toml`