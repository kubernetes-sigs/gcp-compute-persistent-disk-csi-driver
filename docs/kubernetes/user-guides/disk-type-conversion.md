# Persistent Disk to Hyperdisk Conversion User Guide

>**Attention:** This feature is available at v1.29.0 and later.

>**Attention:** Disk type conversion uses the Kubernetes `VolumeAttributesClass` (`v1`) API (`ModifyVolume`). See [PR #2445](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/2445) for the driver implementation.

---

## Configuring the PD CSI Driver

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

> **Note:** Ensure that workloads using the converted volume are scheduled onto node pools whose machine types support both the source Persistent Disk type and the target Hyperdisk type (for example, `c3-standard-4` or `c3d-standard-4`, which carry both `disk-type.gke.io/pd-balanced=true` and `disk-type.gke.io/hyperdisk-balanced=true`). Machine families such as `e2` only support Persistent Disk types and cannot attach Hyperdisk volumes after conversion.

---

## CUJ-01: Specify Target Persistent Volume Configuration

You may specify allowed target persistent volume configurations in `VolumeAttributesClass` resources.

Specify the target volume configuration and save it in `vac-hyperdisk.yaml`:

```yaml
# Save this configuration in vac-hyperdisk.yaml
apiVersion: storage.k8s.io/v1
kind: VolumeAttributesClass
metadata:
  name: vac-hyperdisk
driverName: pd.csi.storage.gke.io
parameters:
  iops: "3000"               # optional: requires target type in --supports-dynamic-iops-provisioning
  throughput: "188Mi"        # optional: requires target type in --supports-dynamic-throughput-provisioning
  type: hyperdisk-balanced   # target disk type
```

Make the target volume configuration available for use in the cluster:

```bash
kubectl apply -f vac-hyperdisk.yaml
```

### Updating IOPS and Throughput of Existing Hyperdisks

GKE allows [modifying Hyperdisk IOPS and throughput dynamically](https://cloud.google.com/kubernetes-engine/docs/how-to/persistent-volumes/gce-pd-csi-driver#hyperdisk-volume-attributes) using `VolumeAttributesClass` (VAC) for disk types listed in `--supports-dynamic-iops-provisioning` and `--supports-dynamic-throughput-provisioning`.

Before the disk type conversion capability, you could configure `iops` and `throughput` parameters in a VAC without specifying a disk `type`. Such configurations continue to work as before.

With the introduction of the disk `type` parameter to VAC, you have the option to specify the disk `type` when updating Hyperdisk IOPS and throughput parameters. This introduces two scenarios:

1. **VAC `type` matches the current disk type**: If the disk `type` specified in the VAC *exactly* matches the disk type of the PersistentVolume to which the VAC is applied, the CSI driver updates the IOPS and throughput of the Hyperdisk according to the VAC configuration:
   - The IOPS and throughput update works the same way as it does without the `type` parameter in the VAC configuration.
   - The CSI driver treats such a VAC as a target IOPS and throughput specification.
   - The CSI driver does *not* treat such a VAC as an intent to convert the disk type and does not trigger conversion logic.
   - For example, when a VAC with `type: hyperdisk-balanced` is applied to a PersistentVolume already backed by `hyperdisk-balanced`, it is treated as an IOPS and throughput modification for the PersistentVolume.
2. **VAC `type` differs from the current disk type**: If the disk `type` specified in the VAC does not match the disk type of the PersistentVolume to which the VAC is applied, the CSI driver treats it as a disk type conversion request and triggers conversion logic, passing all the VAC parameters (including `iops` and `throughput`) as the target disk configuration:
   - For example, when a VAC with `type: hyperdisk-throughput` is applied to a PersistentVolume backed by `hyperdisk-balanced`, the CSI driver treats it as a disk type conversion request. Because the GCE `disks.convert` API does not support converting between two Hyperdisk types, applying the VAC results in an infeasible conversion error (see [Failure mode 3: Unsupported conversion intent](#failure-mode-3-unsupported-conversion-intent)).

---

## CUJ-02: Convert Persistent Volume

The volume must be detached before the underlying GCE conversion operation can run. You can detach the volume by deleting or scaling down the Pods that use it.

For example, suppose you have a PersistentVolumeClaim provisioned with `pd-balanced`:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: database-pvc
spec:
  accessModes:
    - ReadWriteOnce
  storageClassName: standard-rwo
  resources:
    requests:
      storage: 200Gi
```

### Initiate Disk Type Conversion

When you are ready to convert the volume, patch the PVC (e.g., `database-pvc`) with the target `VolumeAttributesClass` (e.g., `vac-hyperdisk`):

```bash
kubectl patch pvc database-pvc -p '{"spec": {"volumeAttributesClassName": "vac-hyperdisk"}}'
```

When a VAC is attached to a PVC and the VAC `type` does not match the actual GCE disk type, the disk is marked for conversion.

The conversion begins at the first of the following events:

- **If the disk is not attached**: Conversion begins immediately when `spec.volumeAttributesClassName` on the PVC is updated.
- **If the disk is attached**: The driver records `pdcsi.gke.io/disk-type-conversion-operation: "Pending"` and `pdcsi.gke.io/disk-type-conversion-target: "hyperdisk-balanced"` on the PersistentVolume and returns `FailedPrecondition` (`cannot convert volume ... while it is attached to [...], detach the volume to start the conversion`). Conversion begins automatically during `ControllerUnpublishVolume` as soon as the disk is detached, provided the VAC `type` still differs from the disk's current type.

> **Important (Waiting for `VolumeAttachment` Deletion When Replacing Pods on the Same Node):**
> When deleting a Pod (or scaling down a `Deployment` / `StatefulSet`) to detach the volume and trigger conversion, wait until the `VolumeAttachment` object for the PersistentVolume is deleted before creating or scaling up the new Pod:
> ```bash
> PV_NAME="$(kubectl get pvc database-pvc -o jsonpath='{.spec.volumeName}')"
> VA_NAME="$(kubectl get volumeattachment -o jsonpath="{.items[?(@.spec.source.persistentVolumeName=='${PV_NAME}')].metadata.name}")"
> kubectl delete pod <pod-name>
> kubectl wait --for=delete "volumeattachment/${VA_NAME}" --timeout=120s
> ```
> If a new Pod is scheduled onto the **same node** before `attachdetach-controller` deletes the `VolumeAttachment`, Kubernetes reuses the existing node attachment without calling `ControllerUnpublishVolume`, and the queued conversion will not start.

No conversion occurs outside of the events above.

### Converting Volumes Managed by a Workload Controller (`StatefulSet` / `Deployment`)

When a volume is used by a Pod managed by a workload controller such as a `StatefulSet` or `Deployment`, additional considerations apply:

1. **Deleting a Pod alone is not sufficient to detach the disk**: The controller (`StatefulSet` / `Deployment`) immediately recreates the deleted Pod. If the replacement Pod lands on the same node before `attachdetach-controller` removes the `VolumeAttachment`, Kubernetes reuses the existing attachment and `ControllerUnpublishVolume` is never called; if it lands on a different node, `attachdetach-controller` immediately attempts to re-attach the volume.
2. **Scaling down a `StatefulSet` impacts High Availability (HA)**: Scaling down a `StatefulSet` (`kubectl scale statefulset --replicas=...`) terminates Pods in strict reverse ordinal order (`N-1` down to `0`). For example, in a 3-replica `StatefulSet` (`pod-0`, `pod-1`, `pod-2`), converting `pod-0` or `pod-1` via `kubectl scale` requires scaling down all higher-ordinal replicas first—taking down the entire `StatefulSet` to convert `pod-0`. To convert a single replica while keeping the remaining replicas online, delete the `StatefulSet` with `--cascade=orphan` so its Pods remain running, then delete and convert one replica at a time.
3. **Node affinity and scheduling after conversion**: After converting a volume to Hyperdisk, the Pod must be scheduled onto a node whose machine family supports the target Hyperdisk type (e.g., `disk-type.gke.io/hyperdisk-balanced: "true"`).
   - **Note on `PersistentVolume.spec.nodeAffinity` immutability**: Kubernetes treats `pv.spec.nodeAffinity` as **immutable** after PV creation (`field is immutable, except for updating from beta label to GA`).
   - If the original PV was provisioned *without* `--disk-topology=true` / `use-allowed-disk-topology: "true"`, `pv.spec.nodeAffinity` only constrains the zone (`topology.gke.io/zone`), so you can constrain scheduling by adding `nodeSelector` or `nodeAffinity` (`disk-type.gke.io/hyperdisk-balanced: "true"`) to the **Pod / controller template (`spec.template.spec`)**.
   - If `pv.spec.nodeAffinity` itself must be changed (for example, if the original PV has a `disk-type.gke.io/pd-*` requirement that target Hyperdisk-only nodes such as `c4` do not carry), you must set `persistentVolumeReclaimPolicy: Retain` on the PV, delete the PV object, and recreate the PV with the updated `spec.nodeAffinity`.
4. **Updating controller `volumeClaimTemplates` for future replicas**: Because `spec.volumeClaimTemplates` on a `StatefulSet` is also immutable in Kubernetes, deleting the `StatefulSet` with `--cascade=orphan` allows you to update `spec.volumeClaimTemplates` (to use a Hyperdisk `StorageClass` for any future scaled-up replicas) and `spec.template.spec` (with Hyperdisk node affinity) before redeploying the `StatefulSet`. Existing replicas continue to bind to their converted PVCs by name (`<volumeClaimTemplate>-<statefulset>-<ordinal>`).

#### End-to-End Steps for Converting a `StatefulSet` Replica Without Full Downtime

1. **Delete the controller while orphaning its running Pods** so other replicas stay online and the target Pod is not immediately recreated:
   ```bash
   kubectl delete statefulset <statefulset-name> --cascade=orphan
   ```
2. **Delete the target replica Pod and wait for its `VolumeAttachment` to be removed**:
   ```bash
   PVC_NAME="<pvc-name-for-target-replica>"
   PV_NAME="$(kubectl get pvc "${PVC_NAME}" -o jsonpath='{.spec.volumeName}')"
   VA_NAME="$(kubectl get volumeattachment -o jsonpath="{.items[?(@.spec.source.persistentVolumeName=='${PV_NAME}')].metadata.name}")"
   kubectl delete pod <target-replica-pod-name>
   kubectl wait --for=delete "volumeattachment/${VA_NAME}" --timeout=120s
   ```
3. **Patch the target PVC with the `VolumeAttributesClass` to execute the conversion** (note: Step 3 can also be performed before Step 2 to queue the conversion as `"Pending"`, which then starts automatically when Step 2 detaches the disk):
   ```bash
   kubectl patch pvc "${PVC_NAME}" -p '{"spec": {"volumeAttributesClassName": "vac-hyperdisk"}}'
   ```
4. **Update node affinity so the Pod schedules onto Hyperdisk-compatible nodes**:
   - Add `disk-type.gke.io/hyperdisk-balanced: "true"` to the controller's Pod template (`spec.template.spec.nodeSelector` or `spec.template.spec.affinity.nodeAffinity`), or—if `pv.spec.nodeAffinity` contains an incompatible `disk-type.gke.io/pd-*` expression—set `persistentVolumeReclaimPolicy: Retain` on the PV and recreate the PV object with the updated `spec.nodeAffinity` after conversion completes.
5. **[Optional] Update the controller configuration (`spec.volumeClaimTemplates`)** so that any new replicas created in the future are provisioned directly with the target Hyperdisk `StorageClass`.
6. **Redeploy the controller** to recreate the Pod deleted in Step 2 (once conversion completes, `ControllerPublishVolume` attaches the converted Hyperdisk and the replica starts):
   ```bash
   kubectl apply -f <statefulset-manifest.yaml>
   ```

### Observe Disk Type Conversion

While the conversion is queued or in progress, the PVC reports `status.modifyVolumeStatus.status: InProgress`:

```bash
kubectl get pvc database-pvc -o jsonpath='{.status.modifyVolumeStatus}'
# {"status":"InProgress","targetVolumeAttributesClassName":"vac-hyperdisk"}
```

Once the volume is detached and the GCE `disks.convert` operation starts, the GCE PD CSI Driver emits a `DiskTypeConversionStart` Kubernetes Event on the `PersistentVolume`:

```json
{
  "apiVersion": "v1",
  "kind": "Event",
  "involvedObject": {
    "apiVersion": "v1",
    "kind": "PersistentVolume",
    "name": "pvc-a9a34765-258e-404d-8cf3-ee7e349cbd1c",
    "uid": "7458a6e8-a7c4-413a-aea7-911045750dd6"
  },
  "reason": "DiskTypeConversionStart",
  "message": "Disk type conversion started for volume \"pvc-a9a34765-258e-404d-8cf3-ee7e349cbd1c\" from pd-balanced to hyperdisk-balanced",
  "action": "ConvertDiskType",
  "reportingComponent": "pdcsi",
  "source": {
    "component": "pdcsi"
  },
  "type": "Normal"
}
```

You can identify and track the active disk type conversion operation by inspecting the annotations on the `PersistentVolume` object:

```yaml
apiVersion: v1
kind: PersistentVolume
metadata:
  annotations:
    pdcsi.gke.io/disk-type-conversion-operation: "https://www.googleapis.com/compute/alpha/projects/example-project/zones/us-central1-a/operations/operation-1794249913601-65fdc5cbd0ec1-157e7d61-073d5443" # selfLink of the active GCE disks.convert operation
    pdcsi.gke.io/disk-type-conversion-source: "pd-balanced"
    pdcsi.gke.io/disk-type-conversion-target: "hyperdisk-balanced"
  name: pvc-a9a34765-258e-404d-8cf3-ee7e349cbd1c
spec:
  accessModes:
    - ReadWriteOnce
  capacity:
    storage: 200Gi
  claimRef:
    apiVersion: v1
    kind: PersistentVolumeClaim
    name: database-pvc
    namespace: default
  csi:
    driver: pd.csi.storage.gke.io
    fsType: ext4
    volumeHandle: projects/example-project/zones/us-central1-a/disks/pvc-a9a34765-258e-404d-8cf3-ee7e349cbd1c
  storageClassName: standard-rwo
  volumeMode: Filesystem
```

While the conversion is in progress, the CSI driver blocks disk attachment attempts in `ControllerPublishVolume` before calling GCE attach, returning `Unavailable` (`cannot attach disk ...: a disk type conversion is in progress (...)`). If you schedule a new Pod using the PVC during the conversion operation, the Pod remains in `Pending` / `ContainerCreating` with a `FailedAttachVolume` event until the conversion completes.

Disk type conversion is a long-running operation whose state is persisted in annotations on the `PersistentVolume`, allowing it to survive Kubernetes control plane restarts, CSI controller restarts, or leader election.

When the disk type conversion completes, the driver emits a `DiskTypeConversionComplete` event on the `PersistentVolume`, removes the in-progress conversion annotations, records the completed migration in `pdcsi.gke.io/disk-type-converted-from` and `pdcsi.gke.io/disk-type-converted-to`, and updates `volumeAttributesClassName`:

```yaml
apiVersion: v1
kind: PersistentVolume
metadata:
  annotations:
    pdcsi.gke.io/disk-type-converted-from: "pd-balanced"        # original GCE disk type
    pdcsi.gke.io/disk-type-converted-to: "hyperdisk-balanced"   # current GCE disk type after conversion
  name: pvc-a9a34765-258e-404d-8cf3-ee7e349cbd1c
spec:
  accessModes:
    - ReadWriteOnce
  capacity:
    storage: 200Gi
  claimRef:
    apiVersion: v1
    kind: PersistentVolumeClaim
    name: database-pvc
    namespace: default
  storageClassName: standard-rwo
  volumeAttributesClassName: vac-hyperdisk
  volumeMode: Filesystem
```

The PVC status transitions to `currentVolumeAttributesClassName: vac-hyperdisk` (with `modifyVolumeStatus` cleared), and any Pod waiting to attach the volume proceeds to attach and start. If the background watcher is interrupted by a controller restart when the GCE operation completes, the annotations and reconciliation are completed on the next `ControllerModifyVolume`, `ControllerPublishVolume`, or `ControllerExpandVolume` call.

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

---

## CUJ-03: Remediate Conversion Failure

### Failure Mode 1: Original Disk Is Attached

When a `VolumeAttributesClass` requesting a disk type conversion is applied while the disk is still attached to a node:
- The conversion is queued and will begin automatically on the next detach (`ControllerUnpublishVolume`), provided the VAC `type` still differs from the disk's type.
- You can verify that the disk is queued for conversion by checking the `PersistentVolume` annotations:
  ```yaml
  pdcsi.gke.io/disk-type-conversion-operation: "Pending"
  pdcsi.gke.io/disk-type-conversion-target: "hyperdisk-balanced"
  ```
- On the PVC, `VolumeModifyFailed` reports `FailedPrecondition`:
  ```text
  rpc error: code = FailedPrecondition desc = cannot convert volume projects/.../zones/.../disks/pvc-... from pd-balanced to hyperdisk-balanced while it is attached to [...], detach the volume to start the conversion
  ```
- **Remediation**: Delete or scale down the Pod(s) using the PVC and wait for the `VolumeAttachment` to be removed so `ControllerUnpublishVolume` can start the conversion.

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
5. **Miscellaneous transient failures**: The conversion operation starts but fails due to an unclassified/transient error during execution. The driver records the error in `pdcsi.gke.io/disk-type-conversion-last-error`, emits a `DiskTypeConversionRetry` warning event on the PV, resets `pdcsi.gke.io/disk-type-conversion-operation` to `"Pending"`, and retries the conversion.

You can verify that the conversion operation is queued for retry by inspecting the annotation on the `PersistentVolume`:

```yaml
pdcsi.gke.io/disk-type-conversion-operation: "Pending"
```

### Failure Mode 3: Unsupported Conversion Intent

Unsupported conversion requests are terminal (`Infeasible`). Conversion will not occur, disk attachment is **not** blocked, and the driver will not retry the conversion unless the `VolumeAttributesClass` on the PVC is changed. The driver records `pdcsi.gke.io/disk-type-conversion-infeasible` and `pdcsi.gke.io/disk-type-conversion-infeasible-intent` on the PV (emitting a `DiskTypeConversionFailed` event) and returns `InvalidArgument`, causing `csi-resizer` to set `status.modifyVolumeStatus.status: Infeasible` on the PVC with the error message in `status.conditions`.

Examples of unsupported conversion intents include:

1. **Same storage generation**: The target disk type is in the same generation as the original disk type (for example, converting from `hyperdisk-balanced` to `hyperdisk-throughput`).
   - *Note*: Applying a VAC whose `type` exactly matches the disk's current type is allowed, does not trigger conversion, and cancels any queued (`Pending`) conversion when replacing another VAC.
2. **Regional Persistent Disk**: The original disk is a regional Persistent Disk (`disk type conversion is not supported for regional disks`).
3. **Unsupported VAC parameters**: The VAC specifies `iops` or `throughput` for a target disk type that does not support dynamic IOPS/throughput provisioning (as configured via `--supports-dynamic-iops-provisioning` and `--supports-dynamic-throughput-provisioning`), or specifies values outside GCE limits (such as provisioned IOPS below the minimum for the target disk type).
4. **Multi-writer access mode**: The original disk uses [multi-writer](https://cloud.google.com/compute/docs/disks/sharing-disks-between-vms#pd_multi_writer_mode) (`READ_WRITE_MANY`) access mode (`disk type conversion is not supported for multi-writer disks`).

Example of a PVC status with a pass-through error message when an unsupported configuration is set in the VAC (`kubectl get pvc database-pvc -o yaml`):

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: database-pvc
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
      message: 'Failed to modify volume projects/example-project/zones/us-east1-c/disks/pvc-337361e7-5b67-49ce-b271-7b97a6441f34:
        error updating disk Key{"pvc-337361e7-5b67-49ce-b271-7b97a6441f34", zone: "us-east1-c"}:
        googleapi: Error 400: Requested provisioned IOPS cannot be smaller than 3000.,
        badRequest'
      reason: Unknown
      status: "True"
      type: ModifyVolumeError
  modifyVolumeStatus:
    status: Infeasible
    targetVolumeAttributesClassName: tiny-vac
```

If your VAC configuration is unsupported and you observe `status.modifyVolumeStatus.status: Infeasible` on your PVC, you can clear `volumeAttributesClassName` on the PVC (supported in Kubernetes 1.34+):

```bash
kubectl patch pvc database-pvc -p '{"spec": {"volumeAttributesClassName": null}}'
```

### Canceling Future Conversion Retries

You may need to cancel a queued conversion or future automatic conversion retries—for example, if a conversion operation fails and the maintenance window for the workload is no longer sufficient for another conversion attempt, or if you decide to abort a queued conversion before detaching the volume.

To cancel future retry attempts, set `spec.volumeAttributesClassName` on the PVC to a `VolumeAttributesClass` that specifies the disk's original disk type (for example, `pd-balanced`):

1. Specify the original volume configuration in `vac-pd-balanced.yaml`:

   ```yaml
   # Save this configuration in vac-pd-balanced.yaml
   apiVersion: storage.k8s.io/v1
   kind: VolumeAttributesClass
   metadata:
     name: vac-pd-balanced # Original disk type before conversion
   driverName: pd.csi.storage.gke.io
   parameters:
     type: pd-balanced
   ```

2. Apply the `VolumeAttributesClass` to the cluster:

   ```bash
   kubectl apply -f vac-pd-balanced.yaml
   ```

3. Patch the PVC to reference the `VolumeAttributesClass` with the original disk type:

   ```bash
   kubectl patch pvc database-pvc -p '{"spec": {"volumeAttributesClassName": "vac-pd-balanced"}}'
   ```

> **Note:** Updating `volumeAttributesClassName` during an already running GCE `disks.convert` operation does **not** cancel the in-flight GCE operation. However, if the conversion is currently in the `"Pending"` state (waiting for disk detachment or waiting for a retry after a failure), patching the PVC to a VAC matching the disk's current type (or a VAC without `type`) removes the `pdcsi.gke.io/disk-type-conversion-operation: "Pending"` annotation (and associated pending conversion annotations) from the `PersistentVolume`, emits a `DiskTypeConversionCancelled` event, and unblocks normal volume attachment. You can verify the current conversion state at any time by inspecting the annotations on the `PersistentVolume`.
