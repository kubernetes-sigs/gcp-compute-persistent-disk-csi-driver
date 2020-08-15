# Kubernetes Basic User Guide
This guide gives a simple example on how to provision zonal and regional PDs in single-zone and regional clusters.

**Note:** Regional cluster support only available in beta starting with
Kubernetes 1.14.

## Install Driver

See [instructions](driver-install.md)

## Zonal PD example
This example provisions a zonal PD in both single-zone and regional clusters.

1. Create example Zonal Storage Class
```
$ kubectl apply -f ./examples/kubernetes/demo-zonal-sc.yaml
```

2. Create example PVC and Pod
```
$ kubectl apply -f ./examples/kubernetes/demo-pod.yaml
```

3. Verify PV is created and bound to PVC
```
$ kubectl get pvc
NAME      STATUS    VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
podpvc     Bound     pvc-e36abf50-84f3-11e8-8538-42010a800002   10Gi       RWO            csi-gce-pd     9s
```

4. Verify pod is created and in `RUNNING` state (it may take a few minutes to get to running state)
```
$ kubectl get pods
NAME                      READY     STATUS    RESTARTS   AGE
web-server                1/1       Running   0          1m
```

## Regional PD example

**Note:** Regional cluster support only available in beta starting with
Kubernetes 1.14.

This example provisions a regional PD in regional clusters.

1. Create example Regional Storage Class. Choose between:

    * Unrestricted zones
      ```
      $ kubectl apply -f ./examples/kubernetes/demo-regional-sc.yaml
      ```

    * Restricted zones
      ```
      $ kubectl apply -f ./examples/kubernetes/demo-regional-restricted-sc.yaml
      ```

2. Create example PVC and Pod
```
$ kubectl apply -f ./examples/kubernetes/demo-pod.yaml
```

3. Verify PV is created and bound to PVC
```
$ kubectl get pvc
NAME      STATUS    VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
podpvc     Bound     pvc-e36abf50-84f3-11e8-8538-42010a800002   10Gi       RWO            csi-gce-pd     9s
```

4. Verify pod is created and in `RUNNING` state (it may take a few minutes to get to running state)
```
$ kubectl get pods
NAME                      READY     STATUS    RESTARTS   AGE
web-server                1/1       Running   0          1m
```

## StorageClass Fields

The list of recognized StorageClass [`parameters`](https://kubernetes.io/docs/concepts/storage/storage-classes/#parameters) is the same as the list of [CSI CreateVolume parameters](../../../README.md#createvolume-parameters).

Additional provisioning parameters are described
[here](https://kubernetes-csi.github.io/docs/external-provisioner.html), for
example the following `StorageClass` will format provisioned volumes as XFS.

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: csi-gce-pd-xfs
provisioner: pd.csi.storage.gke.io
parameters:
  type: pd-standard
  csi.storage.k8s.io/fstype: xfs
volumeBindingMode: WaitForFirstConsumer
```

The list of recognized topology keys in [`allowedTopologies`](https://kubernetes.io/docs/concepts/storage/storage-classes/#allowed-topologies) is listed [here](../../../README.md#topology)
