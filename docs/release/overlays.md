# Overview of per kubernetes overlays

The Persistent Disk CSI Driver manifest [bundle](../../deploy/kubernetes/base) is a set of many kubernetes resources (driver pod which includes the containers csi-provisioner, csi-resizer, csi-snapshotter, gce-pd-driver, csi-driver-registrar; csi driver object, rbacs, pod security policies etc). The master branch has the up-to-date [overlays](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/tree/master/deploy/kubernetes/overlays) for the lastest stable release, in addition to several other overlays used for release and other testing.

## General guidelines for creation/modification of new/existing overlays

When it comes to creation of overlays there are no strict rules on how overlays must be created/modified but the following captures some of the common scenarios of adding new capabilities to the CSI driver bundle and how it would affect the overlays creation/modification.

1. Creating a new k8s minor version overlay with an existing released driver image
   * Consider a scenario where the existing `stable-master` overlay needs to be changed for a new kubernetes version.
   * Update the `deploy/kubernetes/images/prow-stable-sidecar-rc-master` directory with appropriate image versions.
   * Update the `deploy/kubernetes/images/prow-stable-sidecar-rc-master` directory. If there are no additional kustomize diff patches to add, no updates are necessary.
   * At this stage, validate the changes made to the `prow-stable-sidecar-rc-master` overlay. Ensure the upstream testgrids like [this](https://k8s-testgrid.appspot.com/provider-gcp-compute-persistent-disk-csi-driver) are green, and verify with repository maintainers that downstream test grids (e.g GKE internal prow test grids) are green. Care must be taken to avoid directly making changes to `deploy/kubernetes/base` manifests at this stage, as they may impact existing overlays.

   A sample kustomize patch file would look like this:

   ```
   deploy/kubernetes/overlays/prow-stable-sidecar-rc-master/your-new-version-kustomize-diff-patch.yaml
   ```

   In deploy/kubernetes/overlays/prow-stable-sidecar-rc-master/kustomize.yaml,

   ```
    apiVersion: kustomize.config.k8s.io/v1beta1
    kind: Kustomization
    resources:
    - ../stable-master
    patchesStrategicMerge: # or any other patch strategies (such as JsonPatches6902)
    - <your-1-23-kustomize-diff-patch.yaml>
    transformers:
    - ../../images/prow-stable-sidecar-rc-master
   ```

   * When `prow-stable-sidecar-rc-master` is validated, we can then update the yaml in the `base/` directory.
   ```

2. Cutting new releases
   * At the time of cutting a release branch (e.g release-1.0), we should ensure that the release branch has valid overlays bundle for all the most recent k8s minor version. As the master branch moves forward, the release branch is still valid (but not necessarily up-to-date). For the most up-to-date manifests, refer to the master overlay.
   * As the master branch moves forward (changes to sidecars etc), we should cherry-pick changes to an older release branch only on a need basis. Possible cases of cherrypick:

    1. Critical bug fixes on the core driver.

    2. If a critical bug fix involves multiple components (driver and sidecar), cherry-pick all the changes to the older release branch.

   After a change has been cherry-picked to a release branch, a new patch release will have to be cut on that branch, and the overlays would be updated to reflect the new driver image version. Check release process [here](../../release-tools/README.md)
