#!/bin/bash

# This script will remove the GCP Compute Persistent Disk CSI Driver from the
# currently available Kubernetes cluster
#
# Args:
# GCE_PD_DRIVER_VERSION: The kustomize overlay to deploy (located under
#   deploy/kubernetes/overlays). Can be one of {stable, dev}

set -o nounset
set -o errexit

readonly DEPLOY_VERSION="${GCE_PD_DRIVER_VERSION:-stable}"
readonly PKGDIR="${GOPATH}/src/sigs.k8s.io/gcp-compute-persistent-disk-csi-driver"
source "${PKGDIR}/deploy/common.sh"

ensure_kustomize

${KUSTOMIZE_PATH} build ${PKGDIR}/deploy/kubernetes/overlays/${DEPLOY_VERSION} | kubectl delete --ignore-not-found -f -
kubectl delete secret cloud-sa --ignore-not-found
