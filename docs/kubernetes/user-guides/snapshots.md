# Kubernetes Snapshots User Guide (Alpha)

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

1. Create example Default Snapshot Class
```
$ kubectl create -f ./examples/kubernetes/demo-defaultsnapshotclass.yaml
```
2. Create a snapshot of the PVC created in above example
```
$ kubectl create -f ./examples/kubernetes/demo-snapshot.yaml
```
3. Verify Snapshot is created and is ready to use
```
$ k get volumesnapshots demo-snapshot-podpvc -o yaml
apiVersion: snapshot.storage.k8s.io/v1alpha1
kind: VolumeSnapshot
metadata:
  creationTimestamp: 2018-10-05T16:59:26Z
  generation: 1
  name: demo-snapshot-podpvc
  namespace: default

...

status:
  creationTime: 2018-10-05T16:59:27Z
  ready: true
  restoreSize: 6Gi

```
4. Create a new volume from the snapshot
```
$ kubectl create -f ./examples/kubernetes/demo-restore-snapshot.yaml
