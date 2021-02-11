# Overview of per kubernetes minor version overlays

The Persistent Disk CSI Driver manifest [bundle](../../deploy/kubernetes/base) is a set of many kubernetes resources (driver pod which includes the containers csi-provisioner, csi-resizer, csi-snapshotter, gce-pd-driver, csi-driver-registrar; csi driver object, rbacs, pod security policies etc). Not all driver capabilties can be used with all kubernetes versions. For example [volume snapshots](https://kubernetes.io/docs/concepts/storage/volume-snapshots/) are supported on 1.17+ kubernetes versions. Thus structuring the overlays on a per kubernetes minor version is beneficial to quickly identify driver capabilities supported for a given minor version of kubernetes (example [stable-1-16](../../deploy/kubernetes/overlays/stable-1-16) driver manifests does not contain the snapshotter sidecar), and facilitates easy maintenance/updates of the CSI driver. The master branch has the up-to-date [overlays](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/tree/master/deploy/kubernetes/overlays) for each kubernetes version, which specify which driver release version to use with each kubernetes version.

Example:
`stable-1-19` [overlays](../../deploy/kubernetes/overlays/stable-1-19) bundle can be used to deploy all the components of the driver on kubernetes 1.19. This overlay uses persistent disk driver release version [1.2.0](../../deploy/kubernetes/images/stable-1-19/image.yaml).
`prow-gke-staging-release-rc-1-19` [overlays](../../deploy/kubernetes/overlays/prow-gke-staging-release-rc-1-19) bundle can be used to test all the components of the driver on kubernetes 1.19.

## General guidelines for creation/modification of new/existing overlays

When it comes to creation of overlays there are no strict rules on how overlays must be created/modified but the following captures some of the common scenarios of adding new capabilities to the CSI driver bundle and how it would affect the overlays creation/modification.

1. Creating a new k8s minor version overlay with an existing released driver image
   * Consider a scenario where we have existing overlays `stable-1-18`, `stable-master`. We plan to now create a new overlay for k8s 1.19
   * Create a new `deploy/kubernetes/images/prow-gke-staging-release-rc-1-19` directory with appropriate image versions. If the images used in `stable-master` or `stable-1-18` are compatible with 1.19, we can clone from either `deploy/kubernetes/images/stable-master/image.yaml` or `deploy/kubernetes/images/stable-1-18/image.yaml`
   * Create a new `deploy/kubernetes/overlays/prow-gke-staging-release-rc-1-19` directory and clone contents from either `deploy/kubernetes/overlays/stable-master/*` or `deploy/kubernetes/overlays/stable-1-18/*`, depending on which manifests would be compatible with the 1.19 driver bundle and add the necessary kustomize diff patches to `deploy/kubernetes/overlays/stable-1-19/*`.
   * At this stage, validate the changes made to the `prow-gke-release-staging-rc-1-19` overlay. Ensure the upstream testgrids like [this](https://k8s-testgrid.appspot.com/provider-gcp-compute-persistent-disk-csi-driver) are green, and verify with repository maintainers that downstream test grids (e.g GKE internal prow test grids) are green. Care must be taken to avoid directly making changes to `deploy/kubernetes/base` manifests at this stage, as they may impact existing overlays.

   A sample kustomize patch file would look like this:

   ```
   deploy/kubernetes/overlays/prow-gke-release-staging-rc-1-19/your-1-19-kustomize-diff-patch.yaml
   ```

   In deploy/kubernetes/overlays/prow-gke-release-staging-rc-1-19/kustomize.yaml,

   ```
    apiVersion: kustomize.config.k8s.io/v1beta1
    kind: Kustomization
    resources:
    - ../stable-master (If this 1.19 overlay is created based out of stable-master)
    patchesStrategicMerge: # or any other patch strategies (such as JsonPatches6902)
    - <your-1-19-kustomize-diff-patch.yaml>
    transformers:
    - ../../images/prow-gke-release-staging-rc-1-19
   ```

   * When `prow-gke-release-staging-rc-1-19` is validated, we have couple of options to update the stable-1-19. If the kustomize diffs are a one-off change specific to 1.19, we can simply move the kustomize patches to stable-1-19. The kustomize patch file would now look like this.

   ```
   $ mv deploy/kubernetes/overlays/prow-gke-release-staging-rc-1-19/your-1-19-kustomize-diff-patch.yaml deploy/kubernetes/overlays/stable-1-19/your-1-19-kustomize-diff-patch.yaml
   ```

   For `stable-1-19`, 
   ```
    apiVersion: kustomize.config.k8s.io/v1beta1
    kind: Kustomization
    namespace:
    gce-pd-csi-driver
    resources:
    - ../../base/controller
    - ../../base/node_linux
    patchesStrategicMerge: # or any other patch strategies (such as JsonPatches6902)
    - <your-1-19-kustomize-diff-patch.yaml>
    transformers:
    - ../../images/stable-1-19
   ```

   For `prow-gke-release-staging-rc-1-19`,
   ```
    apiVersion: kustomize.config.k8s.io/v1beta1
    kind: Kustomization
    resources:
    - ../stable-1-19
    transformers:
    - ../../images/prow-gke-release-staging-rc-1-19
   ```

   * If the changes are long term, make the changes to `deploy/kubernetes/base`, add the appropriate diff to remove the changes from the overlays less than `stable-1-19`. The overlay kustomize would look something like this

   For `stable-1-19`,
   ```
    apiVersion: kustomize.config.k8s.io/v1beta1
    kind: Kustomization
    namespace:
    gce-pd-csi-driver
    resources:
    - ../../base
    # no patches needed since changes merged to deploy/kubernetes/base
    transformers:
    - ../../images/stable-1-19
   ```

   For `prow-gke-release-staging-rc-1-19`,
   ```
    apiVersion: kustomize.config.k8s.io/v1beta1
    kind: Kustomization
    resources:
    - ../stable-1-19
    transformers:
    - ../../images/prow-gke-release-staging-rc-1-19
   ```

   For `stable-1-18` and older overlays,
   ```
    apiVersion: kustomize.config.k8s.io/v1beta1
    kind: Kustomization
    namespace:
    gce-pd-csi-driver
    resources:
    - ../../base
    patchesStrategicMerge: # or any other patch strategies (such as JsonPatches6902)
    - <your-1-18-kustomize-diff-remove-1-19-changes-patch.yaml>
    transformers:
    - ../../images/stable-1-18
   ```

2. Sidecar image version update for existing overlays
   * Consider a scenario where we have three existing stable overlays `stable-1-17`, `stable-1-18`, `stable-master`. Sidecar S1 is available 1.17+, S2 is available 1.18+. If we need to upgrade the image versions for these sidecars, we would do the following:
   * Update the `deploy/kubernetes/images/prow-gke-staging-release-rc-1-17` for sidecar S1, `deploy/kubernetes/images/prow-gke-staging-release-rc-1-18` and `deploy/kubernetes/images/prow-gke-staging-release-rc-master` for S1 and S2.
   * Validate the changes made to the staging overlays. Ensure the upstream testgrids like [this](https://k8s-testgrid.appspot.com/provider-gcp-compute-persistent-disk-csi-driver) are green, and verify with repository maintainers that downstream test grids (e.g GKE internal prow test grids) are green.
   * Now update `deploy/kubernetes/images/stable-1-17` for sidecar S1, and `deploy/kubernetes/images/stable-1-18`, `deploy/kubernetes/images/stable-master` for sidecar S1 and S2. Repository maintainers should ensure that any internal downstream testgrids are green before approving the pull request to merge changes to a stable overlay.

3. Sidecar spec changes
   * Consider a scenario where a sidecar S1 has version V1 in 1.17 and V2 in 1.18. V2 introcudes a new capability via a new arg 'new-flag' (and say this flag deprecates 'old-flag'). To introduce this change, first we make changes to the `deploy/kubernetes/images/prow-gke-staging-release-rc-1-18` sidecar spec.

   For `prow-gke-staging-release-rc-1-18` overlay, 
   ```
   apiVersion: kustomize.config.k8s.io/v1beta1
   kind: Kustomization
   resources:
   - ../stable-1-18
   patchesStrategicMerge: # or any other patch strategies (such as JsonPatches6902)
   - <your-1-18-kustomize-diff-patch-for-new-sidecar-spec.yaml>
   transformers:
   - ../../images/prow-gke-release-staging-rc-1-18
   ```

   * Validate the changes made to the staging overlays. Ensure the upstream testgrids like [this](https://k8s-testgrid.appspot.com/provider-gcp-compute-persistent-disk-csi-driver) are green, and verify with repository maintainers that downstream test grids (e.g GKE internal prow test grids) are green.
   * Now to merge the change to stable overlay, we incorporate the change in deploy/kubernetes/base, 

   ```
   containers:
        - name: <your-sidecar>
          ...
          args:
            - "--new-flag" # --old-flag deleted.
   ```

   * 
   For `stable-1-18`,
   ```
    apiVersion: kustomize.config.k8s.io/v1beta1
    kind: Kustomization
    namespace:
    gce-pd-csi-driver
    resources:
    - ../../base
    # no patches needed since changes merged to deploy/kubernetes/base
    transformers:
    - ../../images/stable-1-18
   ```

   For `prow-gke-release-staging-rc-1-18`,
   ```
    apiVersion: kustomize.config.k8s.io/v1beta1
    kind: Kustomization
    resources:
    - ../stable-1-18
    transformers:
    - ../../images/prow-gke-release-staging-rc-1-18
   ```

   For `stable-1-17` and older overlays,
   ```
    apiVersion: kustomize.config.k8s.io/v1beta1
    kind: Kustomization
    namespace:
    gce-pd-csi-driver
    resources:
    - ../../base
    patchesStrategicMerge: # or any other patch strategies (such as JsonPatches6902)
    - <your-1-17-kustomize-diff-patch-to-replace-new-flag-with-old-flag.yaml>
    transformers:
    - ../../images/stable-1-17
   ```

4. Driver image update
   * If we have released a new driver image version, update the `deploy/kubernetes/images/prow-gke-release-staging-rc-{k8s-minor}/image.yaml` and `deploy/kubernetes/images/prow-gke-release-staging-rc-master/image.yaml`, for whichever k8s minor version are compatible with the new driver image version.
   * Validate the staging drivers and then make changes to the `deploy/kubernetes/images/stable-{k8s-minor}/image.yaml`and `deploy/kubernetes/images/stable-master/image.yaml` (as needed).

5. Cutting new releases
   * With the master branch maintaining the stable-x.y overlays, the master branch stable overlays can always be the referenced to pick the latest and greatest stable driver manifest bundle.
   * At the time of cutting a release branch (e.g release-1.0), we should ensure that the release branch has valid overlays bundle for all the k8s minor versions the driver supports, and tests running against the release branch will work. As the master branch moves forward, the release branch is still valid (but not necessarily up-to-date). For the most up-to-date manifests, refer to the master stable-x.y overlays.
   * GKE  managed driver changes will be pulled from the master branch stable-x.y overlays.
   * As the master branch moves forward (changes to sidecars etc), we should cherry-pick changes to an older release branch only on a need basis. Possible cases of cherrypick:

    1. Critical bug fixes on the core driver.

    2. If a critical bug fix involves multiple components (driver and sidecar), cherry-pick all the changes to the older release branch.

   After a change has been cherry-picked to a release branch, a new patch release will have to be cut on that branch, and the overlays would be updated to reflect the new driver image version. Check release process [here](../../release-tools/README.md)