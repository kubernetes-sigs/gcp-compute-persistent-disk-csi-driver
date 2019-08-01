#!/bin/bash

# This script will install kustomize, which is a tool that simplifies patching
# Kubernetes manifests for different environments.
# https://github.com/kubernetes-sigs/kustomize

set -o nounset
set -o errexit

readonly INSTALL_DIR="${GOPATH}/src/sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/bin"
readonly KUSTOMIZE_PATH="${INSTALL_DIR}/kustomize"
readonly KUSTOMIZE_VERSION="3.0.0"
readonly VERSION_REGEX="KustomizeVersion:([0-9]\.[0-9]\.[0-9])"

if [ -f "${KUSTOMIZE_PATH}" ]; then
  if [[ $(${KUSTOMIZE_PATH} version) =~ ${VERSION_REGEX} ]]; then
    if [ "${KUSTOMIZE_VERSION}" != "${BASH_REMATCH[1]}" ]; then
      echo "Existing Kustomize version in ${KUSTOMIZE_PATH} v${BASH_REMATCH[1]}, need v${KUSTOMIZE_VERSION}. Removing existing binary."
      rm "${KUSTOMIZE_PATH}"
    fi
  fi
fi

if [ ! -f "${KUSTOMIZE_PATH}" ]; then
  if [ ! -f "${INSTALL_DIR}" ]; then
    mkdir -p ${INSTALL_DIR}
  fi

  echo "Installing Kustomize v${KUSTOMIZE_VERSION} in ${KUSTOMIZE_PATH}"
  opsys=linux  # or darwin, or windows
  curl -s https://api.github.com/repos/kubernetes-sigs/kustomize/releases/tags/v${KUSTOMIZE_VERSION} |\
    grep browser_download |\
    grep $opsys |\
    cut -d '"' -f 4 |\
    xargs curl -O -L
  mv kustomize_*_${opsys}_amd64 ${KUSTOMIZE_PATH}
  chmod u+x ${KUSTOMIZE_PATH}
fi
