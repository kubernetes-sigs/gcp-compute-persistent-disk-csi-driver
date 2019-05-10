# Kubernetes Snapshots User Guide (Alpha)

>**Attention:** VolumeSnapshot is an alpha feature. Make sure you have enabled it in Kubernetes API server using `--feature-gates=VolumeSnapshotDataSource=true` flag.

### Install Driver with alpha snapshot feature

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
$ GCE_PD_DRIVER_VERSION=alpha                     # Driver version to deploy
$ ./deploy/kubernetes/deploy-driver.sh
```

### Snapshot Example

**Create Storage Class:**

If you haven't created a `StorageClass` yet, create one first:

```console
kubectl apply -f ./examples/kubernetes/demo-zonal-sc.yaml
```

**Create  Default VolumeSnapshotClass:**

```console
kubectl create -f ./examples/kubernetes/snapshot/default_volumesnapshotclass.yaml
```

**Create source PVC:**

```console
kubectl create -f ./examples/kubernetes/snapshot/source_pvc.yaml
```

**Generate sample data:**

Create a sample pod with the source PVC. The source PVC is mounted into `/demo/data` directory of this pod. This pod will create a file `sample-file.txt` in `/demo/data` directory.

```console
kubectl create -f ./examples/kubernetes/snapshot/source_pod.yaml
```

Check if the file has been created successfully:

```console
$ kubectl exec source-pod -- ls /demo/data/
lost+found
sample-file.txt
```

**Create a snapshot of the source PVC:**

```console
kubectl create -f ./examples/kubernetes/snapshot/snapshot.yaml
```

**Verify Snapshot has been created and it is ready to use:**

```console
$ kubectl get volumesnapshot snapshot-source-pvc -o yaml
apiVersion: snapshot.storage.k8s.io/v1alpha1
kind: VolumeSnapshot
metadata:
  ...
  name: snapshot-source-pvc
  namespace: default
  ...
spec:
  snapshotClassName: default-snapshot-class
  snapshotContentName: snapcontent-b408076b-720b-11e9-b9e3-42010a800014
  ...
status:
  creationTime: "2019-05-09T03:37:01Z"
  readyToUse: true
  restoreSize: 6Gi
```

**Restore the Snapshot into a new PVC:**

Create a new PVC. Specify `spec.dataSource` section to restore from VolumeSnapshot `snapshot-source-pvc`.

```console
 kubectl create -f ./examples/kubernetes/snapshot/restored_pvc.yaml
```

**Verify sample data has been restored:**

Create a sample pod with the restored PVC:

```console
kubectl create -f ./examples/kubernetes/snapshot/restored_pod.yaml
```

Check data has been restored in `/demo/data` directory:

```console
$ kubectl exec restored-pod -- ls /demo/data/
lost+found
sample-file.txt
```
