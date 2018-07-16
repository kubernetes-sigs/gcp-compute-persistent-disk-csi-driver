#!/bin/bash

set -o nounset
set -o errexit

readonly PKGDIR="${GOPATH}/src/sigs.k8s.io/gcp-compute-persistent-disk-csi-driver"
readonly KUBEDEPLOY="${PKGDIR}/deploy/kubernetes"

if ! kubectl get secret cloud-sa;
then
  kubectl create secret generic cloud-sa --from-file="${SA_FILE}"
fi

# GKE Required Setup
if ! kubectl get clusterrolebinding cluster-admin-binding;
then
  kubectl create clusterrolebinding cluster-admin-binding --clusterrole cluster-admin --user $(gcloud config get-value account)
fi

kubectl apply -f "${KUBEDEPLOY}/setup-cluster.yaml"
kubectl apply -f "${KUBEDEPLOY}/node.yaml"
kubectl apply -f "${KUBEDEPLOY}/controller.yaml"
