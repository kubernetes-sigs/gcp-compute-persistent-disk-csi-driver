# Kubernetes Snapshots User Guide (Beta)

>**Attention:** VolumeSnapshot is a Beta feature enabled by default in
Kubernetes 1.17+. 

>**Attention:** VolumeSnapshot is only available in the driver version "master".

### Install Driver with beta snapshot feature as described [here](driver-install.md)

### Snapshot Example

1. Create `StorageClass`

    If you haven't created a `StorageClass` yet, create one first:

    ```console
    kubectl apply -f ./examples/kubernetes/demo-zonal-sc.yaml
    ```

1. Create default `VolumeSnapshotClass`

    ```console
    kubectl create -f ./examples/kubernetes/snapshot/default-volumesnapshotclass.yaml
    ```

  To place the snapshot in a [custom storage location](https://cloud.google.com/compute/docs/disks/snapshots#custom_location),
  edit `volumesnapshotclass-storage-locations.yaml` to change the `storage-locations` parameter to a location of your
  choice, and then create the `VolumeSnapshotClass`.

    ```console
    kubectl create -f ./examples/kubernetes/snapshot/volumesnapshotclass-storage-locations.yaml
    ```

1. Create source PVC

    ```console
    kubectl create -f ./examples/kubernetes/snapshot/source-pvc.yaml
    ```

1. Generate sample data

    Create a sample pod with the source PVC. The source PVC is mounted into `/demo/data` directory of this pod. This pod will create a file `sample-file.txt` in `/demo/data` directory.

    ```console
    kubectl create -f ./examples/kubernetes/snapshot/source-pod.yaml
    ```

    Check if the file has been created successfully:

    ```console
    kubectl exec source-pod -- ls /demo/data/
    ```

    The output should be:

    ```
    lost+found
    sample-file.txt
    ```

1. Create a `VolumeSnapshot` of the source PVC

    ```console
    kubectl create -f ./examples/kubernetes/snapshot/snapshot.yaml
    ```

1. Verify that `VolumeSnapshot` has been created and it is ready to use:

    ```console
    kubectl get volumesnapshot snapshot-source-pvc -o yaml
    ```

    The output is similar to this:

    ```yaml
    kind: VolumeSnapshot
    metadata:
        creationTimestamp: "2020-05-13T21:48:08Z"
        finalizers:
        - snapshot.storage.kubernetes.io/volumesnapshot-as-source-protection
        - snapshot.storage.kubernetes.io/volumesnapshot-bound-protection
        generation: 1
        managedFields:
        - apiVersion: snapshot.storage.k8s.io/v1beta1
        fieldsType: FieldsV1
        fieldsV1:
            f:status:
            f:readyToUse: {}
        manager: snapshot-controller
        operation: Update
        time: "2020-05-13T21:49:42Z"
        name: snapshot-source-pvc
        namespace: default
        resourceVersion: "531499"
        selfLink: /apis/snapshot.storage.k8s.io/v1beta1/namespaces/default/volumesnapshots/snapshot-source-pvc
        uid: a10fd0ff-b868-4527-abe7-74d5b420731e
    spec:
      source:
        persistentVolumeClaimName: source-pvc
      volumeSnapshotClassName: csi-gce-pd-snapshot-class
    status:
      boundVolumeSnapshotContentName: snapcontent-a10fd0ff-b868-4527-abe7-74d5b420731e
      creationTime: "2020-05-13T21:48:43Z"
      readyToUse: true
      restoreSize: 6Gi
    ```

1. Restore the `VolumeSnapshot` into a new PVC:

    Create a new PVC. Specify `spec.dataSource` section to restore from VolumeSnapshot `snapshot-source-pvc`.

    ```console
    kubectl create -f ./examples/kubernetes/snapshot/restored-pvc.yaml
    ```

1. Verify sample data has been restored:

    Create a sample pod with the restored PVC:

    ```console
    kubectl create -f ./examples/kubernetes/snapshot/restored-pod.yaml
    ```

    Check data has been restored in `/demo/data` directory:

    ```console
    kubectl exec restored-pod -- ls /demo/data/
    ```

    Verify that the output is:

    ```
    lost+found
    sample-file.txt
    ```

### Import a Pre-Existing Snapshot

An existing PD snapshot can be used to provision a `VolumeSnapshotContents`
manually. A possible use case is to populate a PD in GKE from a snapshot created
elsewhere in GCP.

  1. Go to
     [console.cloud.google.com/compute/snapshots](https://console.cloud.google.com/compute/snapshots),
     locate your snapshot, and set an env variable from the snapshot name;
     `export SNAPSHOT_NAME=snapshot-XXXXXXXX-XXXX-XXXX-XXXX-XXXX-XXXXXXXX` (copy
     in your exact name).
  1. Export your project id: `export PROJECT_ID=<your project id>`.
  1. Create a `VolumeSnapshot` resource which will be bound to a pre-provisioned
     `VolumeSnapshotContents`. Note this is called `restored-snapshot`; this
     name can be changed, but do it consistently across the other resources.
     `VolumeSnapshotContentName` field must reference to the
     VolumeSnapshotContent's name for the bidirectional binding to be valid.
     ```console
     kubectl apply -f - <<EOF
       apiVersion: snapshot.storage.k8s.io/v1beta1
       kind: VolumeSnapshot
       metadata:
         name: restored-snapshot
       spec:
         volumeSnapshotClassName: csi-gce-pd-snapshot-class
         source:
           volumeSnapshotContentName: restored-snapshot-content
     EOF
     ```
  1. Create a `VolumeSnapshotContents` pointing to your existing PD
     snapshot from the first step. Both `volumeSnapshotRef.name` and
     `volumeSnapshotRef.namespace` must point to the previously created
     VolumeSnapshot for the bidirectional binding to be valid.
     ```console
     kubectl apply -f - <<EOF
       apiVersion: snapshot.storage.k8s.io/v1beta1
       kind: VolumeSnapshotContent
       metadata:
         name: restored-snapshot-content
       spec:
         deletionPolicy: Retain
         driver: pd.csi.storage.gke.io
         source:
           snapshotHandle: projects/$PROJECT_ID/global/snapshots/$SNAPSHOT_NAME
         volumeSnapshotRef:
           kind: VolumeSnapshot
           name: restored-snapshot
           namespace: default
     EOF
     ```
  1. Create a `PersistentVolumeClaim` which will pull from the
     `VolumeSnapshot`. The `StorageClass` must match what you use to provision
     PDs; what is below matches the first example in this document.
     ```console
     kubectl apply -f - <<EOF
       kind: PersistentVolumeClaim
       apiVersion: v1
       metadata:
         name: restored-pvc
       spec:
         accessModes:
           - ReadWriteOnce
         storageClassName: csi-gce-pd
         resources:
           requests:
             storage: 6Gi
         dataSource:
           kind: VolumeSnapshot
           name: restored-snapshot
           apiGroup: snapshot.storage.k8s.io
     EOF
     ```
  1. Finally, create a pod referring to the `PersistentVolumeClaim`. The PD CSI
     driver will provision a `PersistentVolume` and populate it from the
     snapshot. The pod from `examples/kubernetes/snapshot/restored-pod.yaml` is
     set to use the PVC created above. After `kubectl apply`'ing the pod, run
     `kubectl exec restored-pod -- ls -l /demo/data/` to confirm that the
     snapshot has been restored correctly.

#### Troubleshooting

If the `VolumeSnapshot`, `VolumeSnapshotContents` and `PersistentVolumeClaim`
are not all mutually synchronized, the pod will not start. Try `kubectl describe
volumesnapshotcontent restored-snapshot-content` to see any error messages
relating to the binding together of the `VolumeSnapshot` and the
`VolumeSnapshotContents`. Further errors may be found in the snapshot controller
logs:

```console
kubectl logs snapshot-controller-0 | tail
```

Any errors in `kubectl describe pvc restored-pvc` may also shed light on any
troubles. Sometimes there is quite a wait to dynamically provision the
`PersistentVolume` for the PVC; the following command will give more
information.

```console
kubectl logs -n gce-pd-csi-driver csi-gce-pd-controller-0 -c csi-provisioner | tail
```

### Tips on Migrating from Alpha Snapshots

The api version has changed between the alpha and beta releases of the CSI
snapshotter. In many cases, snapshots created with the alpha will not be able to
be used with a beta driver. In some cases, they can be migrated.

>**Attention:** You may lose all data in your snapshot!

>**Attention:** These instructions not work in your case and could delete all data in your
snapshot!

These instructions happened to work with a particular GKE configuration. Because
of the variety of alpha deployments, it is not possible to verify these steps
will work all of the time. The following has been tested with a GKE cluster of
master version 1.17.5-gke.0 using the following image versions for the
snapshot-controller and PD CSI driver:

  * quay.io/k8scsi/snapshot-controller:v2.0.1
  * gke.gcr.io/csi-provisioner:v1.5.0-gke.0
  * gke.gcr.io/csi-attacher:v2.1.1-gke.0
  * gke.gcr.io/csi-resizer:v0.4.0-gke.0
  * gke.gcr.io/csi-snapshotter:v2.1.1-gke.0
  * registry.k8s.io/cloud-provider-gcp/gcp-compute-persistent-disk-csi-driver:v1.2.1

#### Migrating by Restoring from a Manually Provisioned Snapshot

The idea is to provision a beta `VolumeSnapshotContents` in your new or upgraded
beta PD CSI driver cluster with the PD snapshot handle from the alpha snapshot,
then bind that to a new beta `VolumeSnapshot`.

Note that if using the same upgraded cluster where the alpha driver was
installed, the alpha driver must be completely removed, including an CRs and
CRDs. In particular the alpha `VolumeSnapshot` and `VolumeSnapshotContents`
**must** be deleted before installing any of the beta CRDs. This means that any
information about which snapshot goes with which workload must be manually saved
somewhere. We recommend spinnng up a new cluster and installing the beta driver
there rather than trying to upgrade the alpha driver in-place.

If you are able to set (or have already set) the `deletionPolicy` of any
existing alpha `VolumeSnapshotContents` and alpha `VolumeSnapshotClass` to
`Retain`, then deleting the snapshot CRs will not delete the underlying PD
snapshot.

If you are creating a new cluster rather than upgrading an existing one, the
method below could also be used before deleting the alpha cluster and/or alpha
PD CSI driver deployment. This may be a safer option if you are not sure about
the deletion policy of your alpha snapshot.

After confirming that the PD snapshot is available, restore it using the beta PD
CSI driver as described above in [Import a Pre-Existing
Snapshot](#import-a-pre-existing-snapshot). The restoration should in a cluster
with the beta PD CSI driver installed. At no point is the alpha cluster or
driver referenced. After this is done, using the alpha snapshot may conflict
with the resources created below in the beta cluster.

The maintainers will welcome any additional information and will be happy to
review PRs clarifying or extending these tips.
