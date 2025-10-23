# Kubernetes Driver Installation Guide

## Install Driver

### 1. Clone the driver to your local machine

```console
$ git clone https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver $GOPATH/src/sigs.k8s.io/gcp-compute-persistent-disk-csi-driver
```

### 2. [One-time per project] Set up or use an existing service account:

The driver requires a service account that has the following permissions and
roles to function properly:

```
compute.instances.get
compute.instances.attachDisk
compute.instances.detachDisk
roles/compute.storageAdmin
roles/iam.serviceAccountUser (see security note below)
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
can be used to create a new service account with all the required permissions.

#### Security Note: Service Account Impersonation

The CSI driver requires the `roles/iam.serviceAccountUser` role to impersonate node service accounts when attaching and detaching disks. This role can be configured in two ways:

* **Recommended (Scoped)**: Grant the role only for specific node service accounts
* **Default (Project-wide)**: Allow project-wide service account impersonation (less secure)

For improved security, specify the node service accounts that the CSI driver needs to impersonate using the `NODE_SERVICE_ACCOUNTS` environment variable. This limits the role to only the specified accounts. Without `NODE_SERVICE_ACCOUNTS`, the CSI driver can impersonate any service account in the project.

```console
$ NODE_SERVICE_ACCOUNTS="master-sa@project.iam.gserviceaccount.com,worker-sa@project.iam.gserviceaccount.com"  # Comma-separated list of node service accounts
```

For more details, see [How to remediate over privileged service account users](https://cloud.google.com/security-command-center/docs/how-to-remediate-security-health-analytics-findings#over_privileged_service_account_user).

#### Create service account for the CSI driver

```console
$ PROJECT=your-project-here                       # GCP project
$ GCE_PD_SA_NAME=my-gce-pd-csi-sa                 # Name of the service account to create
$ GCE_PD_SA_DIR=/my/safe/credentials/directory    # Directory to save the service account key
$ ./deploy/setup-project.sh
```

**Note**: The PD CSI Driver will be given the identity `my-gce-pd-csi-sa` during
deployment, all actions performed by the driver will be performed as the
specified service account

### 3. Deploy driver to Kubernetes Cluster

```console
$ NODE_SERVICE_ACCOUNTS="master-sa@project.iam.gserviceaccount.com,worker-sa@project.iam.gserviceaccount.com"  # Same as the setup-project.sh step
$ GCE_PD_SA_DIR=/my/safe/credentials/directory    # Directory to get the service account key
$ GCE_PD_DRIVER_VERSION=stable-master             # Driver version to deploy
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
iam.serviceAccounts.getIamPolicy
iam.serviceAccounts.setIamPolicy
iam.serviceAccounts.create
iam.serviceAccounts.delete
```

These permissions are not required if you already have a service account ready
for use by the PD Driver.

## Disabling particular CSI driver services

Traditionally, you run the CSI controllers with the GCE PD driver in the same Kubernetes cluster.
Though, there may be cases where you will only want to run a subset of the available driver services (for example, one scenario is running the controllers outside of the cluster they are serving (while the GCE PD driver still runs inside the served cluster), but there might be others scenarios).
The CSI driver consists out of these services:

* The **controller** service starts the GRPC server that serves `CreateVolume`, `DeleteVolume`, etc. It is depending on the GCP service account credentials and talks with the GCP API.
* The **identity** service is responsible to provide identity services like capability information of the CSI plugin.
* The **node** service implements the various operations for volumes that are run locally from the node, for example `NodePublishVolume`, `NodeStageVolume`, etc. It does not do operations like `CreateVolume` or `ControllerPublish`. Also, as it runs directly on the GCE instances, it is depending on the GCE metadata service.

The CSI driver has two command line flags, `--run-controller-service` and `--run-node-service` which both default to `true`.
You can disable the individual services by setting the respective flags to `false`.

Note: If you want to run the CSI controllers outside of the cluster you have to specify both the `zone` and `projectId` parameters in the GCE cloud provider config.
The `zone` is the name of one of the availability zones the served Kubernetes cluster is deployed to.
It is used to derive the GCP region and to discover the other availability zones in this region.
The `project-id` is the GCP project ID in which the controller is operating.
