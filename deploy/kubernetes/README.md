# PD CSI Driver Deployment in Kubernetes

## Kustomize Structure

The current structure for kustomization is as follows. Note that Windows support is currently an alpha feature.

* `base`: it contains the setup that is common to different driver versions.
  * `controller_setup`: includes cluster setup and controller yaml files.
  * `node_setup`:
    * Linux: includes node yaml file and related setting that is only applicable for Linux.
    * Windows: includes node yaml file and related setting that is only applicable for Windows.
* `images`: it has a list of images for different versions.
  * `stable`: image list of a stable driver release. Currently only has image list for Linux stable version.
  * `alpha`: image list containing features in development, in addition to images in stable. It also includes Windows images.
  * `dev`: based on alpha, and also contains the developer's image for use in driver development.
  * `prow-gke-release-xxx`: image list used for Prow tests. Currently only Linux is supported.
* `overlays`: it has the version-specific setup. Each overlay corresponds to image lists with the matching name.
  * `stable`: contains deployment specs of a stable driver release. Currently only Linux is supported.
  * `alpha`: contains deployment specs for features in development. Both Linux and Windows are supported. 
  * `dev`: based on alpha, and also contains the developer's specs for use in driver development.
  * `prow-gke-release-xxx`: based on stable, and contains specs for Prow tests. Currently only Linux is supported.
