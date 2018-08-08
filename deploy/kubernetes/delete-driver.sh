#!/bin/bash

# This script will remove the GCP Compute Persistent Disk CSI Driver from the
# currently available Kubernetes cluster

set -o nounset
set -o errexit

readonly PKGDIR="${GOPATH}/src/sigs.k8s.io/gcp-compute-persistent-disk-csi-driver"
readonly KUBEDEPLOY="${PKGDIR}/deploy/kubernetes"

kubectl delete -f "${KUBEDEPLOY}/node.yaml" --ignore-not-found
kubectl delete -f "${KUBEDEPLOY}/controller.yaml" --ignore-not-found
kubectl delete -f "${KUBEDEPLOY}/zonal-sc.yaml" --ignore-not-found
kubectl delete -f "${KUBEDEPLOY}/regional-sc.yaml" --ignore-not-found
kubectl delete -f "${KUBEDEPLOY}/setup-cluster.yaml" --ignore-not-found
kubectl delete secret cloud-sa --ignore-not-found