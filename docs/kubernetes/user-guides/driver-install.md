# Kubernetes Driver Installation Guide

## Install Driver

1. Clone the driver to your local machine

```console
$ git clone https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver $GOPATH/src/sigs.k8s.io/gcp-compute-persistent-disk-csi-driver
```

2. [One-time per project] Set up or use an existing service account:

The driver requires a service account that has the following permissions and
roles to function properly:

```
compute.instances.get
compute.instances.attachDisk
compute.instances.detachDisk
roles/compute.storageAdmin
roles/iam.serviceAccountUser 
```

If there is a pre-existing service account with these roles for use then the
service account key must be downloaded and made discoverable through environment
variable

```
$ gcloud iam service-accounts keys create "/my/safe/credentials/directory/cloud-sa.json" --iam-account "${my-iam-name}" --project "${my-project-name}"
$ GCE_PD_SA_DIR=/my/safe/credentials/directory
```

**Note**: The service account key *must* be named `cloud-sa.json` at driver deploy time

However, if there is no pre-existing service account for use the provided script
can be used to create a new service account with all the required permissions:

```console
$ PROJECT=your-project-here                       # GCP project
$ GCE_PD_SA_NAME=my-gce-pd-csi-sa                 # Name of the service account to create
$ GCE_PD_SA_DIR=/my/safe/credentials/directory    # Directory to save the service account key
$ ./deploy/setup-project.sh
```

**Note**: The PD CSI Driver will be given the identity `my-gce-pd-csi-sa` during
deployment, all actions performed by the driver will be performed as the
specified service account

3. Deploy driver to Kubernetes Cluster

```console
$ GCE_PD_SA_DIR=/my/safe/credentials/directory    # Directory to get the service account key
$ GCE_PD_DRIVER_VERSION=stable                    # Driver version to deploy
$ ./deploy/kubernetes/deploy-driver.sh
```

## GCP Permissions Required

The `setup-project.sh` script only needs to be run once per project to generate
a service account for the driver. The user or service account running this
script needs the following permissions:

```
iam.serviceAccounts.list
iam.serviceAccountKeys.create
iam.roles.create
iam.roles.get
iam.roles.update
```

If a service account provided to `setup-project.sh` does not already exist the
additional permissions are required in order to create the new service account:

```
resourcemanager.projects.getIamPolicy
resourcemanager.projects.setIamPolicy
iam.serviceAccounts.create
iam.serviceAccounts.delete
```

These permissions are not required if you already have a service account ready
for use by the PD Driver.