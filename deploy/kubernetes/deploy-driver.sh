#!/bin/bash

set -o nounset
set -o errexit

if ! kubectl get secret cloud-sa;
then
    kubectl create secret generic cloud-sa --from-file="${SA_FILE}"
fi
kubectl apply -f setup-cluster.yaml
kubectl apply -f node.yaml
kubectl apply -f controller.yaml