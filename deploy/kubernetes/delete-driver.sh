#!/bin/bash

# This script will remove the GCP Compute Persistent Disk CSI Driver from the
# currently available Kubernetes cluster
#
# Args:
# GCE_PD_DRIVER_VERSION: The version of the GCE PD CSI Driver to deploy. Can be one of {v0.1.0, latest}

set -o nounset
set -o errexit

readonly PKGDIR="${GOPATH}/src/sigs.k8s.io/gcp-compute-persistent-disk-csi-driver"
source "${PKGDIR}/deploy/common.sh"

ensure_var GCE_PD_DRIVER_VERSION

readonly KUBEDEPLOY="${PKGDIR}/deploy/kubernetes/${GCE_PD_DRIVER_VERSION}"

kubectl delete -f "${KUBEDEPLOY}/node.yaml" --ignore-not-found
kubectl delete -f "${KUBEDEPLOY}/controller.yaml" --ignore-not-found
kubectl delete -f "${KUBEDEPLOY}/setup-cluster.yaml" --ignore-not-found
kubectl delete secret cloud-sa --ignore-not-found