# Kubernetes Snapshots User Guide (Beta)

>**Attention:** Attention: VolumeSnapshot is a Beta feature enabled by default in Kubernetes 1.17+. Attention: VolumeSnapshot is only available in the driver version "master".

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
