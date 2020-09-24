#!/bin/bash

# The following is a workaround for issue https://github.com/moby/moby/issues/41417
# to manually inserver os.version information into docker manifest file
# TODO: once docker manifest annotation for os.versions is availabler for the installed docker here,
# replace the following with annotation approach. https://github.com/docker/cli/pull/2578

export DOCKER_CLI_EXPERIMENTAL=enabled
_WINDOWS_VERSIONS="1909 ltsc2019"
BASE="mcr.microsoft.com/windows/servercore"

IFS=', ' read -r -a osversions <<< "$_WINDOWS_VERSIONS"
MANIFEST_TAG=${STAGINGIMAGE}:${STAGINGVERSION}

# translate from image tag to docker manifest foler format
# e.g., gcr.io_k8s-staging-csi_gce-pd-windows-v2
manifest_folder=$(echo "${MANIFEST_TAG}" | sed "s|/|_|g" | sed "s/:/-/")
echo ${manifest_folder}
echo ${#osversions[@]}

for ((i=0;i<${#osversions[@]};++i)); do
  BASEIMAGE="${BASE}:${osversions[i]}"
  echo $BASEIIMAGE

  full_version=$(docker manifest inspect ${BASEIMAGE} | grep "os.version" | head -n 1 | awk '{print $2}') || true
  echo $full_version

  IMAGETAG=${STAGINGIMAGE}-${osversions[i]}:${STAGINGVERSION}
  image_folder=$(echo "${IMAGETAG}" | sed "s|/|_|g" | sed "s/:/-/")
  echo ${manifest_folder}

  sed -i -r "s/(\"os\"\:\"windows\")/\0,\"os.version\":$full_version/" \
  "${HOME}/.docker/manifests/${manifest_folder}/${image_folder}"

done
