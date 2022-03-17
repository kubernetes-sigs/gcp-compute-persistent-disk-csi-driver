# PD CSI Driver Deployment in Kubernetes

## Kustomize Structure

The current structure for kustomization is as follows. Note that Windows support is currently an alpha feature.

* `base`: It contains the setup that is common to different driver versions.
  * `controller`: Includes cluster setup and controller yaml files.
  * `node_linux`: Includes node yaml file and related setting that is only applicable for Linux.
  * `node_windows`: Includes node yaml file and related setting that is only applicable for Windows.
* `images`: Contains driver and sidecar image versions for the various [overlays](deploy/kubernetes/overlays/). 
  * `stable-master`: Image versions for the stable-master overlay. 
  * `stable-{k8s-minor}`: Image versions for the stable-{k8s-minor} overlays. 
  * `prow-stable-sidecar-rc-master`: Image versions for the prow-stable-sidecar-rc-master overlay. 
  * `prow-canary-sidecar`: Image versions for the prow-canary-sidecar overlay.
* `overlays`: Contains various k8s resources driver manifest bundles used to deploy all the components of the driver. 
  * `stable-master`: Contains deployment specs of a stable driver for k8s master.
  * `stable-{k8s-minor}`: Contains deployment specs of a stable driver for given k8s minor version release. 
  * `dev`: Based on stable-master, and also contains the developer's specs for use in driver development.
  * `noauth` Based on stable-master, patches the [base controller configuration](deploy/kubernetes/base/controller.yaml) to remove any dependencies on service account keys.
  * `noauth-debug`: Based on stable-master, used for debugging purposes only, see docs/kubernetes/development.md.
  * `prow-stable-sidecar-rc-master`: Used for prow tests on OSS testgrid to test the latest sidecars. Contains deployment specs of a driver with k8s master branch, driver latest release candidate, and stable sidecars.
  * `prow-canary-sidecar`: Used for prow tests on OSS testgrid to test release candidates when a new release is being cut. Contains deployment specs of a driver with k8s master branch, Kubernetes driver latest build, and canary sidecars. 

  ## NVME support
  udev folder under deploy/kubernetes contains google_nvme_id script required for NVME support. Source is downloaded from [GoogleCloudPlatform/guest-configs](https://github.com/GoogleCloudPlatform/guest-configs). README file in the folder contains the downloaded version.
  Execute deploy/kubernetes/update-nvme.sh to update. See releases available in [GoogleCloudPlatform/guest-configs/releases](https://github.com/GoogleCloudPlatform/guest-configs/releases).
