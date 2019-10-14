# Kubernetes Pre-Provisioned Disks User Guide

This guide gives a simple example on how to use this driver with disks that have
been pre-provisioned by an administrator.

## Install Driver

See [instructions](driver-install.md)

## Pre-Provision a Disk

If you have not already pre-provisioned a disk on GCP you can do that now.

1. Create example PD

```bash
gcloud compute disks create test-disk --zone=us-central1-c --size=200Gi
```

## Create Persistent Volume for Disk

1. Create example Storage Class

```bash
kubectl apply -f ./examples/kubernetes/demo-zonal-sc.yaml
```

2. Create example Persistent Volume

**Note:** The `volumeHandle` and `nodeSelectorTerms` values should be generated
based on the project, zone\[s\], and PD name of the disk created. `storage` value
should be generated based on the size of the underlying disk

**Regional PD Note:** The volume handle format is different and the
`nodeSelectorTerms` must contain both zones the PD is supported in.

```yaml
apiVersion: v1
kind: PersistentVolume
metadata:
  name: my-pv
  annotations:
    pv.kubernetes.io/provisioned-by: pd.csi.storage.gke.io
spec:
  storageClassName: "csi-gce-pd"
  capacity:
    storage: 200Gi # MODIFY THIS LINE
  accessModes:
    - ReadWriteOnce
  nodeAffinity:
    required:
      nodeSelectorTerms:
      - matchExpressions:
        - key: topology.gke.io/zone
          operator: In
          values:
          - us-central1-c # MODIFY THIS LINE
        # - us-central1-b <For Regional PD>
  csi:
    driver: "pd.csi.storage.gke.io"
  # volumeHandle: "projects/${PROJECT}/regions/us-central1/disks/test-disk" <For Regional PD>
    volumeHandle: "projects/${PROJECT}/zones/us-central1-c/disks/test-disk" # MODIFY THIS LINE
```

Then `kubectl apply` this YAML.

## Use Persistent Volume In Pod

1. Create example PVC and Pod

```bash
kubectl apply -f ./examples/kubernetes/demo-pod.yaml
```

2. Verify PV is created and bound to PVC

```bash
$ kubectl get pvc
NAME      STATUS    VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
podpvc     Bound     podpvc                                    200Gi       RWO            csi-gce-pd     9s
```

3. Verify pod is created and in `RUNNING` state (it may take a few minutes to
   get to running state)

```bash
$ kubectl get pods
NAME                      READY     STATUS    RESTARTS   AGE
web-server                1/1       Running   0          1m
```
