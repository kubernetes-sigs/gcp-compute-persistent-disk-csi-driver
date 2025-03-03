#!/bin/bash

set -o xtrace

# The following is a workaround for issue https://github.com/moby/moby/issues/41417
# to manually inserver os.version information into docker manifest file
# TODO: once docker manifest annotation for os.versions is availabler for the installed docker here,
# replace the following with annotation approach. https://github.com/docker/cli/pull/2578

export DOCKER_CLI_EXPERIMENTAL=enabled
BASE="mcr.microsoft.com/windows/servercore"

IFS=', ' read -r -a imagetags <<< "$WINDOWS_IMAGE_TAGS"
IFS=', ' read -r -a baseimages <<< "$WINDOWS_BASE_IMAGES"
MANIFEST_TAG=${STAGINGIMAGE}:${STAGINGVERSION}

# translate from image tag to docker manifest foler format
# e.g., gcr.io_k8s-staging-csi_gce-pd-windows-v2
manifest_folder=$(echo "${MANIFEST_TAG}" | sed "s|/|_|g" | sed "s/:/-/")
echo ${manifest_folder}
echo ${#imagetags[@]}
echo ${#baseimages[@]}

for ((i=0;i<${#imagetags[@]};++i)); do
  BASEIMAGE="${baseimages[i]}"
  echo $BASEIIMAGE

  full_version=$(docker manifest inspect ${BASEIMAGE} | grep "os.version" | head -n 1 | awk '{print $2}') || true
  echo $full_version

  IMAGETAG=${STAGINGIMAGE}:${STAGINGVERSION}_${imagetags[i]}
  image_folder=$(echo "${IMAGETAG}" | sed "s|/|_|g" | sed "s/:/-/")
  echo ${manifest_folder}

  # Needed for Windows nodes to correctly recognize the version of image to pull.
  # Populates the os.version field of the multi-arch image manifest.
  docker manifest annotate --os windows --arch amd64 --os-version ${full_version} ${STAGINGIMAGE}:${STAGINGVERSION} ${IMAGETAG}

  # manifest after transformations
  cat "${HOME}/.docker/manifests/${manifest_folder}/${image_folder}"
done

