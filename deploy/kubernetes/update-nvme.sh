#!/bin/bash

# This scripts is to download source of a release from GoogleCloudPlatform/guest-configs repo
# and copy script and rules required for NVMe support to be under deploy/kubernetes 
#
# Args:
# GUEST_CONFIGS_VERSION: The version of the guest configs release

set -o nounset
set -o errexit

readonly PKGDIR="${GOPATH}/src/sigs.k8s.io/gcp-compute-persistent-disk-csi-driver"
readonly INSTALL_DIR="${PKGDIR}/tmp"
readonly DEST="${PKGDIR}/deploy/kubernetes/udev"
readonly SOURCE_FOLDER_NAME="guest-configs-${GUEST_CONFIGS_VERSION}"

function cleanup {
  rm -rf "$INSTALL_DIR"
}

trap cleanup EXIT

curl --create-dirs -O --output-dir ${INSTALL_DIR} -L https://github.com/GoogleCloudPlatform/guest-configs/archive/refs/tags/${GUEST_CONFIGS_VERSION}.tar.gz
tar -xzf ${INSTALL_DIR}/${GUEST_CONFIGS_VERSION}.tar.gz -C ${INSTALL_DIR}
cp -R ${INSTALL_DIR}/${SOURCE_FOLDER_NAME}/src/lib/udev/google_nvme_id ${DEST}
echo "GUEST_CONFIGS_VERSION: " ${GUEST_CONFIGS_VERSION} >  ${DEST}/README
