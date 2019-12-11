# Kubernetes Driver Installation Guide

## Install Driver

1. Clone the driver to your local machine
```
$ git clone https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver $GOPATH/src/sigs.k8s.io/gcp-compute-persistent-disk-csi-driver
```

2. [One-time per project] Create GCP service account for the CSI driver and set required roles
```
$ PROJECT=your-project-here                       # GCP project
$ GCE_PD_SA_NAME=my-gce-pd-csi-sa                 # Name of the service account to create
$ GCE_PD_SA_DIR=/my/safe/credentials/directory    # Directory to save the service account key
$ ./deploy/setup-project.sh
```

3. Deploy driver to Kubernetes Cluster
```
$ GCE_PD_SA_DIR=/my/safe/credentials/directory    # Directory to get the service account key
$ GCE_PD_DRIVER_VERSION=stable                    # Driver version to deploy
$ ./deploy/kubernetes/deploy-driver.sh
```