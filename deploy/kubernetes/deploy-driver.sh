#!/bin/bash

set -o nounset
set -o errexit

readonly PKGDIR="${GOPATH}/src/sigs.k8s.io/gcp-compute-persistent-disk-csi-driver"
readonly KUBEDEPLOY="${PKGDIR}/deploy/kubernetes"

if ! kubectl get secret cloud-sa;
then
    kubectl create secret generic cloud-sa --from-file="${SA_FILE}"
fi
kubectl apply -f "${KUBEDEPLOY}/setup-cluster.yaml"
kubectl apply -f "${KUBEDEPLOY}/node.yaml"
kubectl apply -f "${KUBEDEPLOY}/controller.yaml"