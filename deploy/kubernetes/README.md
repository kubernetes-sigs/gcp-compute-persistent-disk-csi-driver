# PD CSI Driver Deployment in Kubernetes

## Kustomize Structure

The current structure for kustomization is as follows. Note that Windows support is currently an alpha feature.

* `base`: It contains the setup that is common to different driver versions.
  * `controller_setup`: Includes cluster setup and controller yaml files.
  * `node_setup`:
    * Linux: Includes node yaml file and related setting that is only applicable for Linux.
    * Windows: Includes node yaml file and related setting that is only applicable for Windows.
* `images`: It has a list of images for different versions.
  * `stable-master`: Image list of a stable driver for latest k8s master.
  * `alpha`: Image list containing features in development, in addition to images in `stable-master`. It also includes Windows images.
  * `prow-gke-release-xxx`: Image list used for Prow tests.
* `overlays`: It has the k8s minor version-specific driver manifest bundle.
  * `stable-master`: Contains deployment specs of a stable driver for k8s master.
  * `stable-{k8s-minor}`: Contains deployment specs of a stable driver for given k8s minor version release.
  * `alpha`: Contains deployment specs for features in development. Both Linux and Windows are supported.
  * `dev`: Based on alpha, and also contains the developer's specs for use in driver development.
  * `noauth-debug`: Based on alpha, used for debugging purposes only, see docs/kubernetes/development.md.
  * `prow-gke-release-staging-rc-master`: Used for prow tests. Contains deployment specs of a driver for latest k8s master.
  * `prow-gke-release-staging-rc-{k8s-minor}`: Used for prow tests. Contains deployment specs of a driver for given k8s    minor version release.
  * `prow-gke-release-staging-rc-head`: Used for prow tests. Contains deployment specs of a driver with latest sidecar images, for latest k8s master.
  * `stable`, `prow-gke-release-staging-rc`: Soon to be removed!
