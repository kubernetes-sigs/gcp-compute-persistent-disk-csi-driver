# Kubernetes Resize User Guide (Alpha)

>**Attention:** Volume Resize is an alpha feature. Make sure you have enabled it in Kubernetes API server using `--feature-gates=ExpandCSIVolumes=true` flag.
>**Attention:** Volume Resize is only available in the driver version v0.6.0+

### Install Driver with alpha resize feature

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

### Resize Example

This example provisions a zonal PD in both single-zone and regional clusters.

1. Add resize field to example Zonal Storage Class
```
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: csi-gce-pd
provisioner: pd.csi.storage.gke.io
parameters:
  type: pd-standard
volumeBindingMode: WaitForFirstConsumer
allowVolumeExpansion: true
```

2. Create example Zonal Storage Class with resize enabled
```
$ kubectl apply -f ./examples/kubernetes/demo-zonal-sc.yaml
```

3. Create example PVC and Pod
```
$ kubectl apply -f ./examples/kubernetes/demo-pod.yaml
```

4. Verify PV is created and bound to PVC
```
$ kubectl get pvc
NAME      STATUS    VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
podpvc     Bound     pvc-e36abf50-84f3-11e8-8538-42010a800002   10Gi       RWO            csi-gce-pd     9s
```

5. Verify pod is created and in `RUNNING` state (it may take a few minutes to get to running state)
```
$ kubectl get pods
NAME                      READY     STATUS    RESTARTS   AGE
web-server                1/1       Running   0          1m
```

8. Check current filesystem size on the running pod
```
$ kubectl exec web-server -- df -h /var/lib/www/html
Filesystem      Size  Used Avail Use% Mounted on
/dev/sdb        5.9G   24M  5.9G   1% /var/lib/www/html
```

6. Resize volume by modifying the field `spec -> resources -> requests -> storage`
```
$ kubectl edit pvc podpvc
apiVersion: v1
kind: PersistentVolumeClaim
...
spec:
  resources:
    requests:
      storage: 9Gi
  ...
...
```

7. Verify actual disk resized on cloud
```
$ gcloud compute disks describe ${disk_name} --zone=${zone}
description: Disk created by GCE-PD CSI Driver
name: pvc-10ea155f-e5a4-4a82-a171-21481742c80c
...
sizeGb: '9'
```

8. Verify filesystem resized on the running pod
```
$ kubectl exec web-server -- df -h /var/lib/www/html
Filesystem      Size  Used Avail Use% Mounted on
/dev/sdb        8.8G   27M  8.8G   1% /var/lib/www/html
```