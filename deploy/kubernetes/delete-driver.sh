#!/bin/bash

# This script will remove the Google Compute Engine Persistent Disk CSI Driver
# from the currently available Kubernetes cluster
#
# Args:
# GCE_PD_DRIVER_VERSION: The kustomize overlay to deploy (located under
#   deploy/kubernetes/overlays). Can be one of {stable, dev}

set -o nounset
set -o errexit

readonly NAMESPACE="${GCE_PD_DRIVER_NAMESPACE:-gce-pd-csi-driver}"
readonly DEPLOY_VERSION="${GCE_PD_DRIVER_VERSION:-stable}"
readonly PKGDIR="${GOPATH}/src/sigs.k8s.io/gcp-compute-persistent-disk-csi-driver"
readonly DEPLOY_OS_VERSIONS=${DEPLOY_OS_VERSIONS:-"linux stable"}
source "${PKGDIR}/deploy/common.sh"

ensure_kustomize

echo ${DEPLOY_OS_VERSIONS} | tr ';' '\n' | while read -r NODE_OS VERSION; do \
  VERSION="${VERSION:-${DEPLOY_VERSION}}"
	${KUSTOMIZE_PATH} build ${PKGDIR}/deploy/kubernetes/overlays/${NODE_OS}/${VERSION} | ${KUBECTL} delete -v="${VERBOSITY}" --ignore-not-found -f -; \
done

${KUBECTL} delete secret cloud-sa -v="${VERBOSITY}" --ignore-not-found

if [[ ${NAMESPACE} != "" && ${NAMESPACE} != "default" ]] && \
  ${KUBECTL} get namespace ${NAMESPACE} -v="${VERBOSITY}";
then
    ${KUBECTL} delete namespace ${NAMESPACE} -v="${VERBOSITY}"
fi
