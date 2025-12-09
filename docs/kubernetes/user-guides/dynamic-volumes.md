# Dynamic Volumes User Guide

Dynamic Volumes allow GKE storage definitions to adapt seamlessly across different machine type generations. This feature enables you to define a single disk configuration that dynamically selects the appropriate disk type based on node availability and machine type compatibility. This is essential for clusters that mix VM generations (e.g., N2, N3, and N4 VMs) where disk support varies.

> **Important:** Support for GKE Standard and Autopilot clusters is planned for Q1 2026. It is available for [`manual deployments`](./driver-install.md) of the GCE PD CSI Driver. See details below on how to enable and use this feature. 

---

## Enabling Dynamic Volumes

To use Dynamic Volumes, you must enable the feature flag on the driver and ensure your nodes are properly labeled.

> **Note:** Dynamic Volumes are only available in driver version [v1.23.0+](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/releases/tag/v1.23.0).

### 1. Update Command Line Flags
You must add the `--dynamic-volumes=true` flag to both the PD CSI **Controller** and **Node** instances.

In addition to the feature flag, it is highly recommended to add the `--disk-topology=true` flag to the PD CSI **Controller** to ensure the scheduler respects disk type constraints.

### 2. Apply Disk Support Labels
For the PD CSI driver to accurately determine the appropriate disk type for a node, you must apply support labels to every Node object in the cluster.

The label format is: `"disk-type.gke.io/{DISK_TYPE}": "true"`. For example, a Node that supports hyperdisk-balanced should have the following label: `"disk-type.gke.io/hyperdisk-balanced": "true"`.

> **Tip:** You can use the helper script located at [`deploy/disk_type_labels.sh`](../../../deploy/disk_type_labels.sh) to generate the correct labels for a specific machine type.

> **Recommendation**: Use these labels with the `disk-topology` feature enabled and [use-allowed-disk-topology](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/hyperdisk#supported_node_scheduling) parameter to improve stateful workload placement. 

---

## Provisioning Dynamic Volumes

To provision a volume using this feature, create a `StorageClass` with the parameter `type: dynamic`. You can further configure the disk type selection with the parameters described below.

### Dynamic StorageClass Parameters

| Parameter | Description | Default |
| :--- | :--- | :--- |
| `type` | Set to `dynamic` to enable the feature. | N/A |
| `use-allowed-disk-topology` | (Recommended) If `true`, schedules pods only on nodes that support the provisioned disk type. See [Apply Support Labels](#applying-disk-support-labels) for more information. | `false` |
| `pd-type` | (Optional) The specific PD disk type to use. | `pd-balanced` |
| `hyperdisk-type` | (Optional) The specific Hyperdisk type to use. | `hyperdisk-balanced` |
| `disk-type-preference`| (Optional) Overrides the default preference for `hyperdisk-type` when a node supports both options. | `hyperdisk-type` |

> **Note:** Parameters not applicable to the selected disk type will be ignored during provisioning.

### Example Configuration

Below is a `StorageClass` configured for dynamic volumes. It attempts to use Hyperdisk Balanced where available, falling back to PD Balanced otherwise.

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: dynamic-volume
provisioner: pd.csi.storage.gke.io
volumeBindingMode: WaitForFirstConsumer
allowVolumeExpansion: true
parameters:
  type: dynamic
  pd-type: pd-balanced
  hyperdisk-type: hyperdisk-balanced
  use-allowed-disk-topology: "true"
  provisioned-throughput-on-create: "250Mi"
  provisioned-iops-on-create: "3000"