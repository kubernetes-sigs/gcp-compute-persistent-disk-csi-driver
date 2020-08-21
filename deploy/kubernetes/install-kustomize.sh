#!/bin/bash

# This script will install kustomize, which is a tool that simplifies patching
# Kubernetes manifests for different environments.
# https://github.com/kubernetes-sigs/kustomize

set -o nounset
set -o errexit

readonly INSTALL_DIR="${GOPATH}/src/sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/bin"
readonly KUSTOMIZE_PATH="${INSTALL_DIR}/kustomize"

if [ ! -f "${INSTALL_DIR}" ]; then
  mkdir -p "${INSTALL_DIR}"
fi
if [ -f "kustomize" ]; then
  rm kustomize
fi
echo "installing latest version of kustomize"
curl -s "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"  | bash
mv kustomize "${INSTALL_DIR}"
