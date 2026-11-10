# Persistent Disk to Hyperdisk Conversion User Guide

>**Attention:** This feature is available at v1.29.0 and later.

>**Attention:** Disk type conversion uses the Kubernetes `VolumeAttributesClass` (`v1`) API (`ModifyVolume`). See [PR #2445](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/2445) for the driver implementation.

---

## Configure the PD CSI Driver

To enable Persistent Disk to Hyperdisk conversion and dynamic performance provisioning via `VolumeAttributesClass`, configure the `gce-pd-driver` container in the `csi-gce-pd-controller` Deployment with the following command-line flags:

| Flag | Description | Example / Recommended Value |
| :--- | :--- | :--- |
| `--enable-disk-type-conversion` | Enables in-place disk type conversion when a `VolumeAttributesClass` specifies a `type` parameter that differs from the volume's current GCE disk type. | `--enable-disk-type-conversion=true` |
| `--supports-dynamic-iops-provisioning` | Comma-separated list of disk types that support dynamic IOPS configuration via `VolumeAttributesClass` (`parameters.iops`), validated both when modifying an existing Hyperdisk and when converting a disk to a target type with custom `iops`. | `--supports-dynamic-iops-provisioning=hyperdisk-balanced,hyperdisk-extreme` |
| `--supports-dynamic-throughput-provisioning` | Comma-separated list of disk types that support dynamic throughput configuration via `VolumeAttributesClass` (`parameters.throughput`), validated both when modifying an existing Hyperdisk and when converting a disk to a target type with custom `throughput`. | `--supports-dynamic-throughput-provisioning=hyperdisk-balanced,hyperdisk-throughput,hyperdisk-ml` |

### Example Kustomize Patch (`deploy/kubernetes/overlays/dev/driver-args.yaml`)

```yaml
- op: add
  path: /spec/template/spec/containers/0/args/-
  value: --supports-dynamic-throughput-provisioning=hyperdisk-balanced,hyperdisk-throughput,hyperdisk-ml
- op: add
  path: /spec/template/spec/containers/0/args/-
  value: --supports-dynamic-iops-provisioning=hyperdisk-balanced,hyperdisk-extreme
- op: add
  path: /spec/template/spec/containers/0/args/-
  value: --enable-disk-type-conversion=true
```

---

## Prepare for Persistent Volume Type Conversion

### Step 1. Ensure availability of compatible nodes

**Ensure that after disk type conversion, workloads using the converted volumes can be scheduled onto provisioned node pools** whose machine types [support](https://docs.cloud.google.com/compute/docs/disks/hyperdisks#machine-type-support) the target Hyperdisk type (for example, `c3-standard-4` or `n4d-standard-4`, which carry the label `disk-type.gke.io/hyperdisk-balanced: "true"`). Machine families such as `e2` or `n2` only support Persistent Disk types and cannot attach Hyperdisk volumes after conversion.

### Step 2. Prevent unintended volume deletion with `Retain` reclaim policy

**Ensure that your persistent volumes have `Retain` persistent volume reclaim policy** to prevent their unintended deletion.

Update persistent volume reclaim policy to `Retain`:

```bash
kubectl patch pv <pv-name> -p '{"spec":{"persistentVolumeReclaimPolicy":"Retain"}}'
```

### Step 3. Create a Hyperdisk-based storage class

Create a Hyperdisk-based storage class that you plan to upgrade your volumes to and for creating new volumes.

**Specify target storage class**, for example, `hyperdisk-balanced-rwo` storage class like the following:

```yaml
# Save this storage class to file: sc-hd-balanced.yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: hyperdisk-balanced-rwo # The storage class from this example is used in the examples below
provisioner: pd.csi.storage.gke.io
parameters:
  type: hyperdisk-balanced
  use-allowed-disk-topology: "true" # ensures that the PV is only scheduled on machines supporting Hyperdisk (3rd and 4th gen, such as N4 or N4D)
volumeBindingMode: WaitForFirstConsumer
allowVolumeExpansion: true
reclaimPolicy: Retain
```

The parameter `use-allowed-disk-topology: "true"` enables [supported node scheduling](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/hyperdisk#supported_node_scheduling). When the parameter is enabled, GKE instructs Kubernetes to schedule Pods with the `PersistentVolume` of this storage class only on machines supporting the configured disk type by setting the necessary `nodeAffinity` selectors for the `PersistentVolume` object on its creation automatically.

**Deploy the target storage class** to cluster:

```bash
kubectl apply -f sc-hd-balanced.yaml
```

### Step 4. Specify and deploy target persistent volume configuration

You may specify target persistent volume configuration in a `VolumeAttributesClass` resource.

**Specify the target volume configuration** and save it to `vac-hyperdisk.yaml`:

```yaml
# Save this configuration to file: vac-hyperdisk.yaml
apiVersion: storage.k8s.io/v1
kind: VolumeAttributesClass
metadata:
  name: vac-hyperdisk
driverName: pd.csi.storage.gke.io
parameters:
  iops: "3000"               # optional: requires target type in --supports-dynamic-iops-provisioning
  throughput: "140Mi"        # optional: requires target type in --supports-dynamic-throughput-provisioning
  type: hyperdisk-balanced   # target disk type
```

You may optionally specify the desired throughput and IOPS values in the parameters of `VolumeAttributesClass`. If you don't set throughput and IOPS parameters, GCE will apply [default](https://docs.cloud.google.com/compute/docs/disks/hyperdisk-performance#default-perf) values that you can later customize using `VolumeAttributesClass`.

**Make the target volume configuration available** for use in the cluster:

```bash
kubectl apply -f vac-hyperdisk.yaml
```

### Step 5 (recommended). Create an updated version of your workload controller configuration

Updating your workload controller configuration (e.g. `StatefulSet`, `Deployment`) with the target storage class will ensure that Kubernetes will provision the volumes of the target disk type for new replicas.

**Save the original workload controller configuration** in a backup file if you don't have it yet:

```bash
# Save the original StatefulSet configuration to file: original-statefulset.yaml
kubectl get statefulset <statefulset-name> -o yaml > original-statefulset.yaml
```

**Create a file with the updated workload controller configuration** (e.g. `updated-statefulset.yaml`), where you set `volumeClaimTemplates[].spec.storageClassName` to the target Hyperdisk-based storage class (e.g. `hyperdisk-balanced-rwo`).

> **Note: Do not apply the updated configuration yet**, you will apply it after disk conversion. Once applied, the updated storage class in the workload controller configuration will have no impact on the existing replicas with persistent disk volumes. When you increase the replica count, any new Pods will use the updated template and automatically provision their persistent volume claims (PVCs) using the new storage class.

### Step 6 (recommended). Create disk snapshots

You may reduce conversion time by up to 50% by creating a [standard snapshot](https://docs.cloud.google.com/compute/docs/disks/snapshots) of a disk prior to conversion. The conversion operation will only upload incremental disk changes to the snapshot, saving time that way. You can create a snapshot without disrupting your workload while the disk is attached. You can take crash-consistent or application consistent snapshots depending on your workload needs.

We recommend migrating up to 30 TiB simultaneously in aggregate provisioned disk size to minimize conversion time per disk.

We recommend keeping snapshots around for a safe period (e.g. two months) to ensure that you have a rollback mechanism after the Persistent disks are upgraded. This enables you to create a Persistent Disk from a snapshot, if needed. You will also be able to convert Hyperdisks back to Persistent Disks if you need to preserve the latest data written to a Hyperdisk after the disk upgrade.

The best practice approach is to continue taking snapshots regularly from a detached disk for data consistency, including after upgrading the disk to Hyperdisk. The snapshots allow restoring data to a disk of a desired type when you need it. Your [snapshot schedule](https://docs.cloud.google.com/compute/docs/disks/scheduled-snapshots) will continue to work after disk conversion.

### Step 7 (optional). Create extra temporary replicas to maintain necessary serving capacity

If your application allows creating additional replicas, you can create extra replicas to maintain the necessary serving capacity during the volumes' upgrade.

For example, if your `StatefulSet` (e.g. `my-db-ss`) has 3 replicas and you would like to upgrade one replica at a time, you can maintain 3 serving replicas in the following way:

1. Update your `StatefulSet` size to 4 replicas so that it creates one extra replica (e.g. `my-db-ss-3`).
2. Wait until the new replica is hydrated and starts serving.
3. Upgrade the volumes of the original 3 replicas (e.g. `my-db-ss-0`, `my-db-ss-1`, and `my-db-ss-2`) one by one using the instructions below.
4. Update the `StatefulSet` size back to 3 replicas so that it deletes the new temporary replica (in this example, `my-db-ss-3`).

---

## Convert Persistent Volume

You can avoid downtime in replicated applications by migrating one or a subset of replicas at a time, while other replicas continue serving. The exact steps need to account for how a specific application handles replication and serving from the replicas.

Disk conversion is an atomic operation that fully preserves GCE disk identity, including numeric disk ID (`name`, `selfLink`, `selfLinkWithId`). As a consequence, CSI `volumeHandle` is also preserved after disk conversion.

You will need to set the following environment variables to run the examples in this guide:

```bash
STATEFULSET_NAME=<statefulset_name> # StatefulSet name
POD_NAME=<pod_name> # Name of the Pod using the PersistentVolume to be converted
NODE_NAME=$(kubectl get pod $POD_NAME -o jsonpath='{.spec.nodeName}') # Name of the Node where the Pod is scheduled
PVC_NAME=<pvc_name> # Name of the PersistentVolumeClaim bound to the PersistentVolume to be converted
PV_NAME="$(kubectl get pvc "${PVC_NAME}" -o jsonpath='{.spec.volumeName}')" # Name of the PersistentVolume to be converted
VA_NAME="$(kubectl get volumeattachment -o jsonpath="{.items[?(@.spec.source.persistentVolumeName=='${PV_NAME}')].metadata.name}")" # Name of VolumeAttachment associated with PersistentVolume to be converted
```

### Step 1. Determine compatibility of the volume's node affinity

You may need to update node affinity of the converted `PersistentVolume` so that the replicas that use the volume can be scheduled to compatible nodes. This is necessary in the following situations:

- The `PersistentVolume`'s node affinity contains expression `disk-type.gke.io/pd-*` for the pre-conversion disk type, such as `disk-type.gke.io/pd-balanced`, which is incompatible with nodes that support Hyperdisk. Your persistent volume will have this selector if its storage class has [supported node scheduling](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/hyperdisk#supported_node_scheduling) enabled. This node affinity selector will need to be deleted after the disk is converted to Hyperdisk.
- The cluster has nodes incompatible with the target disk type and Kubernetes is likely to attempt to schedule the replica with the converted volume to these nodes. For example, `n2` or `e2` machine families don't support Hyperdisk, and a Pod with a Hyperdisk-based persistent volume will get stuck in `Pending` state if scheduled to such nodes.

> **Note:** If you convert all persistent volumes of all your workload's replicas simultaneously and your persistent volumes *don't* have node affinity expressions incompatible with nodes that support Hyperdisk, you won't need to set node affinity selectors for each persistent volume individually. You can configure the appropriate node selector, such as `disk-type.gke.io/hyperdisk-balanced`, in the controller's Pod template (e.g. `spec.template.spec.affinity.nodeAffinity` in a `StatefulSet`), which will apply to *all* replicas. Updating node affinity at a controller level does not apply if you convert persistent volumes of a subset of replicas while other replicas continue running because you'll need different node affinity rules for the original and updated replicas.

### Step 2. Mark the volume for conversion to the upgraded disk type

To mark the volume for conversion to the upgraded disk type, patch the `PersistentVolumeClaim` with the target `VolumeAttributesClass` (e.g., `vac-hyperdisk`):

```bash
kubectl patch pvc $PVC_NAME -p '{"spec": {"volumeAttributesClassName": "vac-hyperdisk"}}'
```

When a `VolumeAttributesClass` (VAC) is attached to a PVC and the VAC `type` does not match the actual GCE disk type, the disk is marked for conversion.

The conversion begins at the first of the following events:

- **If the disk is not attached**: Conversion begins immediately when `spec.volumeAttributesClassName` on the PVC is updated.
- **If the disk is attached**: The driver records `pdcsi.gke.io/disk-type-conversion-operation: "Pending"` and `pdcsi.gke.io/disk-type-conversion-target: "hyperdisk-balanced"` on the `PersistentVolume` and returns `FailedPrecondition` (`cannot convert volume ... while it is attached to [...], detach the volume to start the conversion`). Conversion begins automatically during `ControllerUnpublishVolume` as soon as the disk is detached, provided the VAC `type` still differs from the disk's current type.

No conversion occurs outside of the events above.

To track conversion progress and completion, see [Observe Disk Type Conversion](#observe-disk-type-conversion).

### Step 3. Detach the volume to begin conversion

The volume must be detached before the underlying GCE conversion operation can run. You can detach the volume either by forcing rescheduling of its Pod to a different node or by deleting the Pod.

If the `PersistentVolume`'s node affinity enables scheduling the replicas that use the volume to compatible nodes after disk conversion, detach the volume by evicting a Pod from its node. It is a simpler and less error-prone approach.

If the `PersistentVolume`'s node affinity does not allow scheduling the replicas that use the volume to compatible nodes after disk conversion, or if you'd also like to update your PVC and PV configuration to accurately reflect disk type after conversion, you'll need to recreate the `PersistentVolume` by orphaning and deleting its Pod before updating and recreating the PVC and PV.

#### Detach the volume by evicting a Pod from its node

You can detach the volume by forcing the workload controller, such as `StatefulSet`, to evict the Pod that uses the volume from its node.

```bash
kubectl cordon $NODE_NAME # Prevent workload controller from scheduling the Pod to the same node before volume is detached
kubectl delete pod $POD_NAME # Evict the Pod from the node to force volume detachment
kubectl wait --for=delete "volumeattachment/${VA_NAME}" --timeout=120s # Wait for detachment to trigger disk conversion before Pod rescheduling
kubectl uncordon $NODE_NAME # Make node available for scheduling after volume is detached and conversion begins
```

> **Important: Wait for `VolumeAttachment` deletion.** When deleting a Pod to detach the volume and trigger conversion, wait until the `VolumeAttachment` object for the `PersistentVolume` is deleted before uncordoning the node so that Kubernetes doesn't reschedule the Pod to the same node before the volume is detached. If a new Pod is scheduled onto the same node before `attachdetach-controller` deletes the `VolumeAttachment`, Kubernetes reuses the existing node attachment without calling `ControllerUnpublishVolume`, and the queued conversion will not start.

Conversion begins automatically as soon as the disk is detached, provided the VAC `type` still differs from the disk's current type.

While the conversion is in progress, the CSI driver will fail disk attach attempts before calling GCE attach, with a clear error message stating that the disk cannot be attached because conversion is in progress. The workload controller won't be able to schedule the Pod using the PVC during conversion, and the Pod will remain in `Pending` state with a `FailedAttachVolume` event showing that the volume attach operation failed due to an ongoing conversion.

The workload controller will schedule the Pod after conversion is completed.

#### Updating node affinity by orphaning and deleting a Pod, and recreating the Persistent Volume (PV)

This is only necessary if the node affinity on the PV prevents it from being scheduled on the desired new node. See the discussion in [Step 1](#step-1-determine-compatibility-of-the-volumes-node-affinity).

##### 1. Delete workload controller and Pod to detach the volume

You can detach the volume by deleting the workload controller and Pods that use it. Because a workload controller, such as `StatefulSet`, immediately recreates a deleted Pod, you need to delete the controller before you can delete a Pod.

**Delete the controller** while orphaning its running Pods so other replicas stay online and the target Pod is not immediately recreated after deletion:

```bash
kubectl delete statefulset $STATEFULSET_NAME --cascade=orphan # orphaned Pods keep running
```

This tells Kubernetes to remove the `StatefulSet` controller but leave the existing Pods, PVCs, and data completely untouched and running.

**Delete the target replica Pod** and wait for its `VolumeAttachment` to be removed:

```bash
kubectl delete pod $POD_NAME
kubectl wait --for=delete "volumeattachment/${VA_NAME}" --timeout=120s
```

> **Important: Wait for `VolumeAttachment` deletion.** When deleting a Pod to detach the volume and trigger conversion, wait until the `VolumeAttachment` object for the `PersistentVolume` is deleted before recreating the target Pod. If a new Pod is scheduled onto the same node before `attachdetach-controller` deletes the `VolumeAttachment`, Kubernetes reuses the existing node attachment without calling `ControllerUnpublishVolume`, and the queued conversion will not start.

Conversion begins automatically as soon as the disk is detached, provided the VAC `type` still differs from the disk's current type.

> **Note:** If the disk conversion fails and you need to restore the Pod replica, redeploy your workload controller with either the original or updated storage class: `kubectl apply -f updated-statefulset.yaml`. The `StatefulSet` will repair the replica Pod. The updated storage class in the `StatefulSet` configuration has no impact on the existing replicas. See [Remediate Conversion Failure](#remediate-conversion-failure) for troubleshooting guidance.

##### 2. (If necessary) Update persistent volume's node affinity

See the discussion in [Step 1](#step-1-determine-compatibility-of-the-volumes-node-affinity). This step is only necessary if your PV has node affinity that would prevent it from attaching to gen3/4 instance types.

Follow these steps to update node affinity of a specific persistent volume:

**1. Save PVC configuration** to yaml file, e.g. `pvc_config.yaml`, if you don't have a saved version:

```bash
kubectl get pvc $PVC_NAME -o yaml > pvc_config.yaml # Save PersistentVolumeClaim object to yaml file
```

**2. Update the saved PVC configuration**:

1. Update `storageClassName` to the target storage class, e.g. `hyperdisk-balanced-rwo`.
2. Preserve all other fields.

```yaml
# Example of PVC configuration saved in pvc_config.yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  annotations:
    volume.kubernetes.io/storage-provisioner: pd.csi.storage.gke.io
    ...
  name: storage-volume-demo-ss-0 # preserve PVC name
  namespace: default
  ...
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 4Gi
  storageClassName: hyperdisk-balanced-rwo # update to the new storage class
  volumeMode: Filesystem
  volumeName: pvc-556ab229-cde8-4675-91ac-c1139af42fee # preserve volume name
status:
  accessModes:
  - ReadWriteOnce
  capacity:
    storage: 4Gi
...
```

**3. Save persistent volume configuration to yaml file**, e.g. `pv_config.yaml`:

```bash
kubectl get pv $PV_NAME -o yaml > pv_config.yaml # Save PersistentVolume object to yaml file
```

**4. Update the saved persistent volume configuration**:

1. Update `spec.storageClassName` to the new storage class (e.g. `hyperdisk-balanced-rwo`).
2. Remove `spec.claimRef` field and its subfields to remove the reference to the PVC deleted in a preceding step.
3. (Conditional) If necessary, update the PV's node affinity selectors so that the replica can be scheduled on all compatible nodes. For example, you may need to remove the node affinity selectors that restrict replica scheduling only to nodes that support the original `pd-balanced` disk type, and you may need to add the node affinity selectors to ensure scheduling the PV only to compatible nodes after conversion.
4. Preserve all other fields.

```yaml
# Example of PV configuration saved in pv_config.yaml
apiVersion: v1
kind: PersistentVolume
metadata:
  name: pvc-556ab229-cde8-4675-91ac-c1139af42fee
  ...
spec:
  ...
  claimRef:                                  # remove claimRef and its subfields
   apiVersion: v1                            # to remove the reference
   kind: PersistentVolumeClaim               # to the PVC that will need to be updated
   name: storage-volume-demo-ss-0            # in the following steps
   namespace: default                        # remove
   resourceVersion: "1794422911955647008"    # remove
   uid: 7a9de879-70f1-40c6-8da5-ceced656322f # remove
  ...
  nodeAffinity:
    required:
      nodeSelectorTerms:
      - matchExpressions:
        - key: topology.gke.io/zone
          operator: In
          values:
          - us-east1-b
        - key: disk-type.gke.io/pd-balanced  # remove this selector
          operator: In                       # to allow the replica scheduling
          values:                            # on hyperdisk-compatible nodes
          - "true"                           # remove
        - key: disk-type.gke.io/hyperdisk-balanced  # (conditional) add this selector
          operator: In                       # to enforce the replica scheduling
          values:                            # on hyperdisk-compatible nodes only
          - "true"                           # add
  persistentVolumeReclaimPolicy: Retain
  storageClassName: hyperdisk-balanced-rwo # updated storage class name after the disk upgrade
...
```

**5. Delete the PVC** to release the persistent volume:

```bash
kubectl delete pvc $PVC_NAME # Delete PersistentVolumeClaim to release PersistentVolume
```

The `PersistentVolume` will stay because of its `Retain` reclaim policy.

**6. Recreate the persistent volume using the updated configuration**:

```bash
kubectl delete pv $PV_NAME # Delete PersistentVolume object
kubectl apply -f pv_config.yaml # Create PersistentVolume using the updated configuration
```

**7. Recreate the PVC using the updated configuration**:

```bash
kubectl apply -f pvc_config.yaml # Create PersistentVolumeClaim using the updated configuration
```

##### 3. Redeploy the controller to recover replicas after conversion

Deploy the controller so that it recreates the deleted replica's Pod and attaches the upgraded volume.

```bash
kubectl apply -f updated-statefulset.yaml # Deploy the controller, e.g. the updated StatefulSet configuration
```

1. The deleted replica Pod will be recreated and the updated PVC will be reattached.
2. Existing Pods will seamlessly reconnect to the newly recreated `StatefulSet`. Their underlying PVCs will keep their original storage class.
3. When you increase the replica count, any new Pods will use the updated template and automatically provision their PVCs using the new storage class.

### Step 4 (optional). Customize persistent volume performance

Hyperdisk Balanced provides a [baseline performance](https://docs.cloud.google.com/compute/docs/disks/hd-types/hyperdisk-balanced#baseline_performance) of 3000 IOPS and 140 MiB/s throughput, which is sufficient for a vast number of workloads. You may also use this [tool](https://github.com/dutchiechris/storage/tree/main/disk/disk-perf-sizing) to determine project level usage over a certain period of time.

Depending on your performance requirements, you may need to update your desired IOPS and throughput in a `VolumeAttributesClass` and [apply](https://docs.cloud.google.com/kubernetes-engine/docs/how-to/persistent-volumes/gce-pd-csi-driver#update_the_pvc_to_use_a_different_performance_tier) the updated VAC to the `PersistentVolumeClaim`.

---

## Observe Disk Type Conversion

Disk type conversion is a long-running operation whose state is persisted in annotations on the `PersistentVolume`, allowing it to survive Kubernetes control plane restarts, CSI controller restarts, or leader election.

If you initiate disk conversion by applying `VolumeAttributesClass` to the PVC when the disk is attached to a node, the conversion request will be queued and will begin automatically as soon as the disk is detached, provided the VAC `type` still differs from the disk's current type.

### Observe Disk Type Conversion Start

Once the volume is detached and the GCE `disks.convert` operation starts, the GCE PD CSI Driver emits a `DiskTypeConversionStart` Kubernetes Event on the `PersistentVolume`:

```json
{
  "apiVersion": "v1",
  "kind": "Event",
  "involvedObject": {
    "apiVersion": "v1",
    "kind": "PersistentVolume",
    "name": "pvc-556ab229-cde8-4675-91ac-c1139af42fee",
    "uid": "556ab229-cde8-4675-91ac-c1139af42fee"
  },
  "reason": "DiskTypeConversionStart",
  "message": "Disk type conversion started for volume \"pvc-556ab229-cde8-4675-91ac-c1139af42fee\" from pd-balanced to hyperdisk-balanced",
  "action": "ConvertDiskType",
  "reportingComponent": "pdcsi",
  "source": {
    "component": "pdcsi"
  },
  "type": "Normal"
}
```

### Observe Disk Type Conversion Progress

While the conversion is queued or in progress, the PVC reports `status.modifyVolumeStatus.status: InProgress`:

```bash
kubectl get pvc $PVC_NAME -o jsonpath='{.status.modifyVolumeStatus}'
# {"status":"InProgress","targetVolumeAttributesClassName":"vac-hyperdisk"}
```

You can identify and track the active disk type conversion [GCE operation](https://docs.cloud.google.com/compute/docs/instances/viewing-compute-operations) by inspecting the annotations on the `PersistentVolume` object:

```yaml
apiVersion: v1
kind: PersistentVolume
metadata:
  annotations:
    pdcsi.gke.io/disk-type-conversion-operation: "https://www.googleapis.com/compute/alpha/projects/example-project/zones/us-central1-a/operations/operation-1794249913601-65fdc5cbd0ec1-157e7d61-073d5443" # selfLink of the active GCE disks.convert operation
    pdcsi.gke.io/disk-type-conversion-source: "pd-balanced"
    pdcsi.gke.io/disk-type-conversion-target: "hyperdisk-balanced"
  name: pvc-556ab229-cde8-4675-91ac-c1139af42fee
spec:
  accessModes:
    - ReadWriteOnce
  capacity:
    storage: 200Gi
  claimRef:
    apiVersion: v1
    kind: PersistentVolumeClaim
    name: storage-volume-demo-ss-0
    namespace: default
  csi:
    driver: pd.csi.storage.gke.io
    fsType: ext4
    volumeHandle: projects/example-project/zones/us-central1-a/disks/pvc-556ab229-cde8-4675-91ac-c1139af42fee
  storageClassName: standard-rwo
  volumeMode: Filesystem
```

> **Note:** While the conversion is in progress, the CSI driver blocks disk attachment attempts in `ControllerPublishVolume` before calling GCE attach, returning `Unavailable` (`cannot attach disk ...: a disk type conversion is in progress (...)`). If you schedule a new Pod using the PVC during the conversion operation, the Pod remains in `Pending` / `ContainerCreating` with a `FailedAttachVolume` event until the conversion completes.

### Observe Disk Type Conversion Completion

When the disk type conversion completes, the driver emits a `DiskTypeConversionComplete` event on the `PersistentVolume`, removes the in-progress conversion annotations, records the completed migration in the annotations `pdcsi.gke.io/disk-type-converted-from` and `pdcsi.gke.io/disk-type-converted-to`, and updates `volumeAttributesClassName`:

```yaml
apiVersion: v1
kind: PersistentVolume
metadata:
  annotations:
    pdcsi.gke.io/disk-type-converted-from: "pd-balanced"        # original GCE disk type
    pdcsi.gke.io/disk-type-converted-to: "hyperdisk-balanced"   # current GCE disk type after conversion
  name: pvc-556ab229-cde8-4675-91ac-c1139af42fee
spec:
  accessModes:
    - ReadWriteOnce
  capacity:
    storage: 200Gi
  claimRef:
    apiVersion: v1
    kind: PersistentVolumeClaim
    name: storage-volume-demo-ss-0
    namespace: default
  storageClassName: standard-rwo
  volumeAttributesClassName: vac-hyperdisk
  volumeMode: Filesystem
```

The PVC status transitions to `currentVolumeAttributesClassName: vac-hyperdisk` (with `modifyVolumeStatus` cleared), and any Pod waiting to attach the volume proceeds to attach and start.

> **Note:** If the background watcher is interrupted by a controller restart when the GCE operation completes, the annotations and reconciliation are completed on the next `ControllerModifyVolume`, `ControllerPublishVolume`, or `ControllerExpandVolume` call.

### Observe Disk Type Conversion Cancellation

When you cancel conversion retries, PD CSI driver removes the `pdcsi.gke.io/disk-type-conversion-operation: "Pending"` annotation (and associated pending conversion annotations) from the `PersistentVolume`, emits a `DiskTypeConversionCancelled` event, and unblocks normal volume attachment. You can verify the current conversion state at any time by inspecting the annotations on the `PersistentVolume`.

### PersistentVolume Conversion Annotations Reference

The table below lists all `pdcsi.gke.io/disk-type-*` annotations managed on the `PersistentVolume` across the conversion lifecycle:

| Annotation Key | When Set / Values | Purpose |
| :--- | :--- | :--- |
| `pdcsi.gke.io/disk-type-conversion-operation` | `"Pending"`, `<GCE operation selfLink URL>`, or `"Reconciling"` (removed when conversion and reconciliation complete or are canceled) | Tracks the active conversion state: `"Pending"` when queued waiting for detach or retry, the GCE `compute/alpha` operation `selfLink` while `disks.convert` is running, and `"Reconciling"` while post-conversion VAC reconciliation is finishing. |
| `pdcsi.gke.io/disk-type-conversion-target` | Target disk type (e.g., `"hyperdisk-balanced"`); removed upon completion or cancellation | Records the target disk type requested by the `VolumeAttributesClass`. |
| `pdcsi.gke.io/disk-type-conversion-source` | Source disk type (e.g., `"pd-balanced"`); removed upon completion or cancellation | Records the disk's original type when `startDiskTypeConversion` prepares the conversion; moved to `pdcsi.gke.io/disk-type-converted-from` once the conversion completes. |
| `pdcsi.gke.io/disk-type-conversion-request-id` | `<uuid>`; written atomically with `"Pending"` before calling GCE `disks.convert`, and removed once the GCE operation URL is recorded | Deterministic GCE request UUID ensuring retries or controller restarts replay the same request rather than submitting duplicate GCE conversion operations. |
| `pdcsi.gke.io/disk-type-conversion-request-iops` | `<int64>` (only when `parameters.iops` is set in the VAC); removed upon completion or cancellation | Persists the requested target provisioned IOPS for idempotent request replay and infeasible-intent tracking. |
| `pdcsi.gke.io/disk-type-conversion-request-throughput` | `<int64>` in MiB/s (only when `parameters.throughput` is set in the VAC); removed upon completion or cancellation | Persists the requested target provisioned throughput for idempotent request replay and infeasible-intent tracking. |
| `pdcsi.gke.io/disk-type-conversion-reconciliation-id` | `<GCE operation selfLink URL>`; set alongside `operation: "Reconciling"` and removed when post-conversion reconciliation finishes | Identifies the completed GCE operation whose post-conversion VAC reconciliation is in progress so stale workers cannot clear a newer operation. |
| `pdcsi.gke.io/disk-type-conversion-last-error` | Error string from a failed retriable conversion; removed on the next `ControllerModifyVolume` call or completion | Records the failure of an operation that ran and failed with a retriable error so the next `ControllerModifyVolume` call can surface the error on the PVC status. |
| `pdcsi.gke.io/disk-type-conversion-infeasible` | Terminal error message; removed if a new VAC with a different intent is applied | Caches why a requested conversion is terminal (`Infeasible`) so `external-resizer` retries return `InvalidArgument` without calling GCE repeatedly. |
| `pdcsi.gke.io/disk-type-conversion-infeasible-intent` | `type=<type>[,iops=<iops>][,throughput=<throughput>]` | Scopes `disk-type-conversion-infeasible` to the exact target `type`, `iops`, and `throughput` parameters that produced the terminal error. |
| `pdcsi.gke.io/disk-type-converted-from` | Source disk type (e.g., `"pd-balanced"`); persisted after completion | Audit annotation recording the disk type prior to the last completed conversion. |
| `pdcsi.gke.io/disk-type-converted-to` | Target disk type (e.g., `"hyperdisk-balanced"`); persisted after completion | Audit annotation recording the disk type resulting from the last completed conversion. |

#### Inspect persistent volume annotations

You can list `PersistentVolume` annotations related to the disk conversion operation by running the following command:

```bash
kubectl get pv $PV_NAME -o json | jq -r '
  (.metadata.annotations // {}) |
  to_entries[] |
  select(.key | startswith("pdcsi.gke.io/disk-type-")) |
  "\(.key)=\(.value)"'
```

---

## Remediate Conversion Failure

If a conversion operation fails, the original disk stays intact and no temporary resources are created or leaked.

### Failure Mode 1: Original Disk Is Attached

When a `VolumeAttributesClass` requesting a disk type conversion is applied while the disk is still attached to a node:

- The conversion is queued and will begin automatically on the next detach (`ControllerUnpublishVolume`), provided the VAC `type` still differs from the disk's type.
- You can verify that the disk is queued for conversion by checking the `PersistentVolume` annotations:

```yaml
pdcsi.gke.io/disk-type-conversion-operation: "Pending" # The conversion operation is queued
pdcsi.gke.io/disk-type-conversion-target: "hyperdisk-balanced"
```

- On the PVC, `VolumeModifyFailed` reports `FailedPrecondition`:

```text
rpc error: code = FailedPrecondition desc = cannot convert volume projects/.../zones/.../disks/pvc-... from pd-balanced to hyperdisk-balanced while it is attached to [...], detach the volume to start the conversion
```

- **Remediation**: Delete the Pod(s) using the PVC and wait for the `VolumeAttachment` to be removed so `ControllerUnpublishVolume` can start the conversion.

### Failure Mode 2: Retriable Failures

Retriable failures are automatically retried by the CSI driver. While a conversion is waiting for retry, disk attachment remains blocked until the conversion completes (or is canceled), and the PVC status surfaces the pass-through error message from the `disks.convert` API.

Common retriable failure conditions include:

1. **Snapshotting operation in progress**: A snapshot operation for the original disk is currently in progress. The driver reports that conversion is still in progress/retrying via events and continues polling/retrying.
2. **Disk state prevents snapshotting**: The original disk is in a state that prevents snapshotting (for example, the disk has an instant snapshot). The driver reports the error and periodically retries the conversion.
   - **Remediation**: Manually [delete all instant snapshots](https://cloud.google.com/compute/docs/disks/create-instant-snapshots#delete-an-instant-snapshot) on the disk so the next retry can proceed.
3. **API and operation rate/concurrency limits**: The driver automatically retries when quota or concurrency limits are reached:
   - *Concurrent operation limit*: Exceeded the limit of 10 concurrent disk conversion operations per project per region.
   - *API rate limit*: Exceeded the limit of 375 disk conversion calls per minute per region.
4. **Zone stockout**: Insufficient capacity in the zone prevents the underlying conversion snapshot/restore from completing. The driver reports the error and retries.
5. **Miscellaneous transient failures**: The conversion operation starts but fails due to an unclassified/transient error during execution. The driver records the error in the annotation `pdcsi.gke.io/disk-type-conversion-last-error`, emits a `DiskTypeConversionRetry` warning event on the PV, resets the annotation `pdcsi.gke.io/disk-type-conversion-operation` to `"Pending"`, and retries the conversion.

You can verify that the conversion operation is queued for retry by inspecting the annotation on the `PersistentVolume`:

```yaml
pdcsi.gke.io/disk-type-conversion-operation: "Pending" # The conversion operation is queued
```

### Failure Mode 3: Unsupported Conversion Intent

Unsupported conversion requests are terminal (`Infeasible`). Conversion will not occur, disk attachment is **not** blocked, and the driver will not retry the conversion unless the `VolumeAttributesClass` on the PVC is changed. The driver records the annotations `pdcsi.gke.io/disk-type-conversion-infeasible` and `pdcsi.gke.io/disk-type-conversion-infeasible-intent` on the PV (emitting a `DiskTypeConversionFailed` event) and returns `InvalidArgument`, causing `csi-resizer` to set `status.modifyVolumeStatus.status: Infeasible` on the PVC with the error message in `status.conditions`.

Examples of unsupported conversion intents include:

1. **Same storage generation**: The target disk type is in the same generation as the original disk type (for example, converting from `hyperdisk-balanced` to `hyperdisk-throughput`).
   - *Note*: Applying a VAC whose `type` exactly matches the disk's current type is allowed, does not trigger conversion, and cancels any queued (`Pending`) conversion when replacing another VAC.
2. **Regional Persistent Disk**: The original disk is a regional Persistent Disk (disk type conversion is not supported for regional disks).
3. **Unsupported VAC parameters**: The VAC specifies `iops` or `throughput` for a target disk type that does not support dynamic IOPS/throughput provisioning (as configured via `--supports-dynamic-iops-provisioning` and `--supports-dynamic-throughput-provisioning`), or specifies values outside GCE limits (such as provisioned IOPS below the minimum for the target disk type).
4. **Multi-writer access mode**: The original disk uses [multi-writer](https://cloud.google.com/compute/docs/disks/sharing-disks-between-vms#pd_multi_writer_mode) (`READ_WRITE_MANY`) access mode (disk type conversion is not supported for multi-writer disks).

Example of a PVC status with a pass-through error message when an unsupported configuration is set in the VAC (`kubectl get pvc storage-volume-demo-ss-0 -o yaml`):

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: storage-volume-demo-ss-0
  namespace: default
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 200Gi
  storageClassName: hyperdisk-balanced-rwo
  volumeAttributesClassName: tiny-vac # custom VAC with unsupported small values of iops and throughput
status:
  conditions:
    - lastProbeTime: "2026-08-07T16:41:21Z"
      lastTransitionTime: "2026-08-07T16:40:49Z"
      message: 'Failed to modify volume projects/example-project/zones/us-central1-a/disks/pvc-556ab229-cde8-4675-91ac-c1139af42fee:
        error updating disk Key{"pvc-556ab229-cde8-4675-91ac-c1139af42fee", zone: "us-central1-a"}:
        googleapi: Error 400: Requested provisioned IOPS cannot be smaller than 3000.,
        badRequest'
      reason: Unknown
      status: "True"
      type: ModifyVolumeError
  modifyVolumeStatus:
    status: Infeasible # Operation is infeasible
    targetVolumeAttributesClassName: tiny-vac
```

If your VAC configuration is unsupported and you observe `status.modifyVolumeStatus.status: Infeasible` on your PVC, you can clear `volumeAttributesClassName` on the PVC (supported in Kubernetes 1.34+) and cancel future conversion retries:

```bash
kubectl patch pvc $PVC_NAME -p '{"spec": {"volumeAttributesClassName": null}}'
```

### Canceling Future Conversion Retries

You may need to cancel a queued conversion or future automatic conversion retries—for example, if a conversion operation fails and the maintenance window for the workload is no longer sufficient for another conversion attempt, or if you decide to abort a queued conversion before detaching the volume.

To cancel future retry attempts, set `spec.volumeAttributesClassName` on the PVC to a `VolumeAttributesClass` that specifies the disk's original disk type (for example, `pd-balanced`):

1. Specify the original volume configuration in a `VolumeAttributesClass` file, e.g. `vac-pd-balanced.yaml`:

   ```yaml
   # Save this configuration in vac-pd-balanced.yaml
   apiVersion: storage.k8s.io/v1
   kind: VolumeAttributesClass
   metadata:
     name: vac-pd-balanced
   driverName: pd.csi.storage.gke.io
   parameters:
     type: pd-balanced # The original disk type before conversion
   ```

2. Apply the `VolumeAttributesClass` to the cluster:

   ```bash
   kubectl apply -f vac-pd-balanced.yaml
   ```

3. Patch the PVC to reference the `VolumeAttributesClass` with the original disk type:

   ```bash
   kubectl patch pvc $PVC_NAME -p '{"spec": {"volumeAttributesClassName": "vac-pd-balanced"}}'
   ```

> **Note:** Updating `volumeAttributesClassName` during an already running GCE `disks.convert` operation does **not** cancel the in-flight GCE operation. However, if the conversion is currently in the `"Pending"` state (waiting for disk detachment or waiting for a retry after a failure), patching the PVC to a VAC matching the disk's current type (or a VAC without `type`) removes the `pdcsi.gke.io/disk-type-conversion-operation: "Pending"` annotation (and associated pending conversion annotations) from the `PersistentVolume`, emits a `DiskTypeConversionCancelled` event, and unblocks normal volume attachment. You can verify the current conversion state at any time by inspecting the annotations on the `PersistentVolume`.

### Converting Persistent Volume to the Original Disk Type

You can convert the volume back from Hyperdisk to Persistent Disk, e.g. from `hyperdisk-balanced` to `pd-balanced` disk type, by configuring `VolumeAttributesClass` with the target PD type (e.g. `pd-balanced`) and patching the PVC. Disk type conversion works in both directions.

### Converting Regional Persistent Disk

The `VolumeAttributesClass` API does not support conversion of Regional PDs. You can convert Regional PDs to Hyperdisk Balanced HA by creating a snapshot from the Regional PD and then creating Hyperdisk Balanced HA from the snapshot. The following disk conversion steps preserve disk name and URL so that CSI volume handle stays the same:

1. Detach the original disk `example-disk`.
2. Create a snapshot from the original disk `example-disk`.
3. Create a temporary Hyperdisk from the snapshot: `temp-example-disk`.
4. Delete the original disk `example-disk`.
5. Create a hyperdisk with the name of the original disk `example-disk` using the temporary disk `temp-example-disk` as a source disk.
6. Delete the temporary disk `temp-example-disk`.

Creation of a temporary Hyperdisk (step 3) ensures that you have a ready-to-use disk at any time during the conversion sequence.

---

## Updating IOPS and Throughput of Existing Hyperdisks

GKE allows [modifying Hyperdisk IOPS and throughput dynamically](https://cloud.google.com/kubernetes-engine/docs/how-to/persistent-volumes/gce-pd-csi-driver#hyperdisk-volume-attributes) using `VolumeAttributesClass` (VAC) for disk types listed in `--supports-dynamic-iops-provisioning` and `--supports-dynamic-throughput-provisioning`.

Before the disk type conversion capability, you could configure `iops` and `throughput` parameters in a VAC without specifying a disk `type`. Such configurations continue to work as before.

With the introduction of the disk `type` parameter to VAC, you have the option to specify the disk `type` when updating Hyperdisk IOPS and throughput parameters. This introduces two scenarios:

1. **VAC `type` matches the current disk type**: If the disk `type` specified in the VAC *exactly* matches the disk type of the `PersistentVolume` to which the VAC is applied, the CSI driver updates the IOPS and throughput of the Hyperdisk according to the VAC configuration:
   - The IOPS and throughput update works the same way as it does without the `type` parameter in the VAC configuration.
   - The CSI driver treats such a VAC as a target IOPS and throughput specification.
   - The CSI driver does *not* treat such a VAC as an intent to convert the disk type and does not trigger conversion logic.
   - For example, when a VAC with `type: hyperdisk-balanced` is applied to a `PersistentVolume` already backed by `hyperdisk-balanced`, it is treated as an IOPS and throughput modification for the `PersistentVolume`.
2. **VAC `type` differs from the current disk type**: If the disk `type` specified in the VAC does not match the disk type of the `PersistentVolume` to which the VAC is applied, the CSI driver treats it as a disk type conversion request and triggers conversion logic, passing all the VAC parameters (including `iops` and `throughput`) as the target disk configuration:
   - For example, when a VAC with `type: hyperdisk-throughput` is applied to a `PersistentVolume` backed by `hyperdisk-balanced`, the CSI driver treats it as a disk type conversion request. Because the underlying GCE `disks.convert` API does not support converting between two Hyperdisk types, applying the VAC results in an infeasible conversion error (see [Failure Mode 3: Unsupported Conversion Intent](#failure-mode-3-unsupported-conversion-intent)).
