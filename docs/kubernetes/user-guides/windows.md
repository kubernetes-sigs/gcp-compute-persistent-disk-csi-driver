# Kubernetes CSI Windows User Guide

>**Attention:** CSI Windows Beta is only available in the driver version v1.1.0+
>**Attention:** CSI Windows Alpha is only available in the driver version v1.0.0-v1.0.*

### Install CSI Proxy binary
CSI proxy can be installed as binary or run as a Windows service on each Windows node. Please see details on CSI Proxy [installation and usage page](https://github.com/kubernetes-csi/csi-proxy/blob/master/README.md#usage).

If you are using kube-up to start a GCE Kubernetes cluster, starting Kubernetes 1.19, [CSI Proxy Alpha](https://github.com/kubernetes-csi/csi-proxy/releases/tag/v0.1.0) binary is automatically installed during node start up. In Kubernetes 1.20, [CSI Proxy Beta.2 v0.2.2](https://github.com/kubernetes-csi/csi-proxy/releases/tag/v0.2.2) is installed and running as a windows service to improve its stablibity.

For GKE cluster, starting from 1.18, [CSI Proxy Beta](https://github.com/kubernetes-csi/csi-proxy/releases/tag/v0.2.2) will be installed automatically. GCE PD driver will be also automatically deployed as daemonSet on GKE. Please follow instruction here to create a [GKE Windows cluster](https://cloud.google.com/kubernetes-engine/docs/how-to/creating-a-cluster-windows).


### Install Driver with CSI Windows support

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
$ GCE_PD_DRIVER_VERSION=alpha                     # Currently alpha deploy Driver version with Windows Alpha support. Will add beta supporot to beta GCE_PD_DRIVER_VERSION deployment script.
$ ./deploy/kubernetes/deploy-driver.sh
```

3. Create a pod on Windows node

See [instructions](basic.md)