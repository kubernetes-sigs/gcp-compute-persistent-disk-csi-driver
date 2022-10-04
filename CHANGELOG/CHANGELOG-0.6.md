# v0.6.0 - Changelog Since v0.5.0

## Breaking Changes

- Some of the API objects in the deployment specs have changed names/labels/namespaces, please tear down old driver before deploying this version to avoid orphaning old objects. You will also no longer see the driver in the `default` namespace.
- Some error codes have been changed, please see below for details if you rely on specific error codes of the driver

## New Features

- Add support for Raw Block devices. ([#283](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/283), [@davidz627](https://github.com/davidz627))
- Operations in the node driver are now parallelized, except those involving a volume already being operated on now return an error. ([#303](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/303), [@hantaowang](https://github.com/hantaowang))
- Adds support for ControllerExpandVolume and NodeExpandVolume ([#317](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/317), [@davidz627](https://github.com/davidz627))
- Operations in the controller driver on a volume already being operated on now return an error. ([#316](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/316), [@hantaowang](https://github.com/hantaowang))
- Picking up support for inline volume migration and some fixes for backward compatible access modes for migration ([#324](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/324), [@davidz627](https://github.com/davidz627))


## Bug Fixes

- Reduces node attach limits by 1 since the node boot disk is considered an attachable disk ([#361](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/361), [@davidz627](https://github.com/davidz627))
- Fixed a bug that causes disks in the same zone/region to be provisioned serially ([#344](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/344), [@hantaowang](https://github.com/hantaowang))
- Remove cross validation of access modes, multiple access modes can be specified that represent all the capabilities of the volume ([#289](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/289), [@davidz627](https://github.com/davidz627))
- Driver should check socket parent directory before trying to bind it ([#339](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/339), [@zhucan](https://github.com/zhucan))
- Updated CSI Attacher to stop ignoring errors from ControllerUnpublish ([#378](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/378), [@davidz627](https://github.com/davidz627))
- CreateVolume will now fail with NOT_FOUND error when VolumeContentSource SnapshotId does not refer to a snapshot that can be found ([#312](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/312), [@davidz627](https://github.com/davidz627))
- ControllerUnpublishVolume now returns success when the Node is GCE API NotFound.
Invalid format VolumeID is now GRPC InvalidArgument error instead of GRPC NotFound.
Underspecified disks not found in any zone now return GRPC NotFound. ([#368](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/368), [@davidz627](https://github.com/davidz627))


## Other Notable Changes

- Deployment spec updates:
The deployment is no longer in namespace `default`
Changed "app" label key to "k8s-app"
csi-snapshotter version has been changed to v1.2.0-gke.0
The resizer role binding has been renamed to "csi-controller-resizer-binding"
Removed driver-registrar role.  ([#364](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/364), [@verult](https://github.com/verult))
- Updating the following image versions in stable deployment specs:
gcp-compute-persistent-disk-csi-driver: v0.6.0-gke.0
csi-provisioner: v1.4.0-gke.0
csi-attacher: v2.0.0-gke.0
csi-node-driver-registrar: v1.2.0-gke.0 ([#400](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/400), [@verult](https://github.com/verult))