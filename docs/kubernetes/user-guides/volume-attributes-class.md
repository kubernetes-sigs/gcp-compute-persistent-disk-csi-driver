# VolumeAttributesClass User Guide

>**Attention:** VolumeAttributesClass is a Kubernetes Beta feature since v1.31, but was initially introduced as an alpha feature in v1.29. See [this blog post](https://kubernetes.io/docs/concepts/storage/volume-attributes-classes/) for more information on VolumeAttributesClasses and how to enable the feature gate.

### VolumeAttributesClass Example

This example provisions a hyperdisk-balanced and then updates its IOPS and throughput.

1. Create the VolumeAttributesClasses (VACs), PVC, Storage Classes, and Pod
``` 
$ kubectl apply -f ./examples/kubernetes/demo-vol-create.yaml
```

This creates the VACs silver and gold, with the VAC for silver being the initial metadata for the PV and gold representing the update. This file also create a storage class test-sc. Note that the IOPS/throughput takes priority from the VACs if they are different. Then, a PVC is created, specifying the storage class for the PV being test-sc and the VAC being silver. Finally, a pod is created with the volume being created from the PVC we defined.

2. Verify the PV is properly created
```
$ kubectl get pvc
NAME        STATUS    VOLUME      CAPACITY   ACCESS MODES   STORAGECLASS   VOLUMEATTRIBUTESCLASS    AGE
test-pvc     Bound     {pv-name}   64Gi       RWO            test-sc        silver                   {some-time}
```

3. Update the IOPS/throughput for the created PV
```
$ kubectl apply -f ./examples/kubernetes/demo-vol-update.yaml
```

4. Verify the PV VAC is properly updated
```
$ kubectl get pvc
NAME        STATUS    VOLUME      CAPACITY   ACCESS MODES   STORAGECLASS   VOLUMEATTRIBUTESCLASS    AGE
test-pvc     Bound     {pv-name}   64Gi       RWO            test-sc        gold                   {some-time}
```

After verifying the VAC is updated from the PV, we can check that the IOPS and throughput are properly updated.

```
$ gcloud compute disks describe {pv-name} --zone={pv-zone}
```

Ensure that the provisionedIops and provisionedThroughput fields match those from the gold VAC. Note that it will take a few minutes for the value updates to be reflected 