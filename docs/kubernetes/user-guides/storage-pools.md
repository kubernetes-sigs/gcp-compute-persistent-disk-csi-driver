# Hyperdisk Storage Pools User Guide

Provisioning attached disks in Storage Pools is supported in the managed version of our driver which is automatically enabled on new GKE clusters, and in a manually deployed GCE PD CSI Driver. We recommend using this feature with the managed GCE PD CSI Driver. See the public documentation [here](https://cloud.google.com/kubernetes-engine/docs/how-to/persistent-volumes/hyperdisk-storage-pools). 

If you would like to use Storage Pools with a manual deployment of the GCE PD CSI driver, you will need to do a couple additional things to enable the feature as described below.

### Enabling Storage Pools for a Manual Deployment of GCE PD CSI driver.

>**Attention:** Note that Storage Pools is only available in the driver version [v1.13.0+](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/releases/tag/v1.13.0).

In addition to the install instructions [here](driver-install.md), you need to specify the CSI driver command line flags, `--enable-storage-pools=true`. 

### Provision Volumes in a Storage Pool

To provision volumes in a Storage Pool, follow the instructions [here](https://cloud.google.com/kubernetes-engine/docs/how-to/persistent-volumes/hyperdisk-storage-pools#provision-attached-disk). When using the feature in a manual deployment of the GCE PD CSI Driver, in the [Create a StorageClass](https://cloud.google.com/kubernetes-engine/docs/how-to/persistent-volumes/hyperdisk-storage-pools#create-storageclass) section, you must additionally set [`allowedTopologies`](https://kubernetes.io/docs/concepts/storage/storage-classes/#allowed-topologies) to restrict the topology of provisioned volumes to specific zones where Storage Pools exist as specified in the `storage-pools` StorageClass parameter. 

For example, looking at the example in [Create a StorageClass](https://cloud.google.com/kubernetes-engine/docs/how-to/persistent-volumes/hyperdisk-storage-pools#create-storageclass), assuming you already created a Storage Pool in `us-east4-c` according to [Create a Hyperdisk Storage Pool](https://cloud.google.com/kubernetes-engine/docs/how-to/persistent-volumes/hyperdisk-storage-pools#create-sp), your `StorageClass` would need to specify `allowedTopologies` to restrict the topology of provisioned volumes to us-east4-c, where the Storage Pool exists.

    ```yaml
    apiVersion: storage.k8s.io/v1
    kind: StorageClass
    metadata:
        name: storage-pools-sc
    provisioner: pd.csi.storage.gke.io
    volumeBindingMode: WaitForFirstConsumer
    allowVolumeExpansion: true
    parameters:
        type: hyperdisk-balanced
        provisioned-throughput-on-create: "140Mi"
        provisioned-iops-on-create: "3000"
        storage-pools: projects/my-project/zones/us-east4-c/storagePools/pool-us-east4-c
    allowedTopologies:
    - matchLabelExpressions:
        - key: topology.gke.io/zone
            values:
            - us-east4-c
    ```
