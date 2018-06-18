WARNING: This driver is in ALPHA currently. This means that there may be
potentially backwards compatability breaking changes moving forward. Do NOT use
this drive in a production environment in its current state.

# GCP Compute Persistent Disk CSI Driver

The GCP Compute Persistent Disk CSI Driver is a
[CSI](https://github.com/container-storage-interface/spec/blob/master/spec.md)
Specification compliant driver used by Container Orchestrators to manage the
lifecycle of Google Compute Engine Persistent Disks.

## Installing
Templates and further information for installing this driver on Kubernetes are
in deploy/kubernetes/