#!/bin/bash

set -o nounset
set -o errexit

readonly PKGDIR="${GOPATH}/src/github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver"
readonly KUBEDEPLOY="${PKGDIR}/deploy/kubernetes"

kubectl delete -f "${KUBEDEPLOY}/node.yaml" --ignore-not-found
kubectl delete -f "${KUBEDEPLOY}/controller.yaml" --ignore-not-found
kubectl delete -f "${KUBEDEPLOY}/setup-cluster.yaml" --ignore-not-found
kubectl delete secret cloud-sa --ignore-not-found