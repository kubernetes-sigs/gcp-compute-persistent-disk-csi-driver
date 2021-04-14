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

echo "installing kustomize"

where=$PWD
if [ -f $where/kustomize ]; then
  echo "A file named kustomize already exists (remove it first)."
  exit 1
fi

tmpDir=`mktemp -d`
if [[ ! "$tmpDir" || ! -d "$tmpDir" ]]; then
  echo "Could not create temp dir."
  exit 1
fi

function cleanup {
  rm -rf "$tmpDir"
}

trap cleanup EXIT

pushd $tmpDir >& /dev/null

opsys=windows
arch=amd64
if [[ "$OSTYPE" == linux* ]]; then
  opsys=linux
elif [[ "$OSTYPE" == darwin* ]]; then
  opsys=darwin
fi

# As github has a limit on what stored in releases/, and kustomize has many different package
# versions, we just point directly at the version we want. See 
# github.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh.

version=v3.9.4
url_base=https://api.github.com/repos/kubernetes-sigs/kustomize/releases/tags/kustomize%2F
curl -s ${url_base}${version} |\
  grep browser_download.*${opsys}_${arch} |\
  cut -d '"' -f 4 |\
  sort -V | tail -n 1 |\
  xargs curl -s -O -L

tar xzf ./kustomize_v*_${opsys}_${arch}.tar.gz

cp ./kustomize $where

popd >& /dev/null

./kustomize version

mv kustomize "${INSTALL_DIR}"
