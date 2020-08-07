# v0.7.0 - Changelog Since v0.6.0

## Changes with Action Required 

- Adding `PodSecurityPoliciy` to allow `csi-gce-pd-node` in clusters with policies enabled.
IF LOCAL PSP MANIFEST PATCH IS USED PLEASE BEWARE THAT YOU WILL NEED TO DELETE LOCAL CHANGES AND USE THE UPSTREAM ([#448](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/448), [@ffilippopoulos](https://github.com/ffilippopoulos))
- BREAKING CHANGE: All deployment objects in setup-cluster.yaml have been renamed. When deleting the deployment using ./delete-driver.sh, make sure to use specs from your previous deployment version to ensure the correct objects are cleaned up. ([#405](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/405), [@verult](https://github.com/verult))

## New Features

- Add GET_VOLUME_STATS Node Service Capability and implementation for getting stats for volume ([#406](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/406), [@davidz627](https://github.com/davidz627))
- ValidateVolumeCapabilities validates that the given volume conforms to all capabilities in the request. Validation of existing volumes during inserts also improved to check all parameters. ([#467](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/467), [@davidz627](https://github.com/davidz627))
- It is now possible to disable the controller service by setting `--run-controller-service=false`. Similarly, it is possible to disable the node service by setting `--run-node-service=false`. The latter enables running the controller server of the GCE PD driver separately/outside of the cluster it is serving. Also, if both `project-id` and `zone` are specified in the GCE cloud config then the controller server does no longer try to contact the GCE metadata service. ([#449](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/449), [@rfranzke](https://github.com/rfranzke))
- Add support for formatting and mounting an XFS filesystem ([#447](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/447), [@davidz627](https://github.com/davidz627))
- Add a blanket toleration to the Node Daemonset of the driver deployment so that it can be deployed on all nodes ([#417](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/417), [@davidz627](https://github.com/davidz627))
- Adds LIST_VOLUMES and LIST_VOLUMES_PUBLISHED_NODES capabilities with respective functionality ([#392](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/392), [@davidz627](https://github.com/davidz627))


## Bug Fixes

- Fixed bug where ControllerExpandVolume was returning incorrect size when disk was already the requested size or larger ([#462](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/462), [@davidz627](https://github.com/davidz627))
- Set volume limits to 15 only for machine-types: "f1-micro", "g1-small", "e2-micro", "e2-small", "e2-medium". Limit is 127 for all others ([#455](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/455), [@davidz627](https://github.com/davidz627))
- Changed deployment of Controller and Node components to use hostNetwork for compatibility with GKE Workload Identity ([#436](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/436), [@davidz627](https://github.com/davidz627))
- During NodeStageVolume run udevadm --trigger to fix device symlinks if device path is not found or device path points to the wrong device ([#459](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/459), [@davidz627](https://github.com/davidz627))
- Bump external-snapshotter version to v1.2.2 for fix of CVE-2019-11255 ([#434](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/434), [@davidz627](https://github.com/davidz627))


## Other Notable Changes

- Update driver base image distro to debian-amd64:v2.0.0 and build with go v1.13.4 ([#439](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/439), [@davidz627](https://github.com/davidz627))
- Mounting an unformatted volume with an fstype as read-only now throws a more descriptive error ([#458](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/458), [@davidz627](https://github.com/davidz627))
- Remove explicit stripping of secrets from RPC request/response logs since the driver doesn't accept secrets for operations ([#428](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/428), [@davidz627](https://github.com/davidz627))
- Improve driver logs to log success in all paths as well as logging additional useful information ([#409](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/409), [@davidz627](https://github.com/davidz627))