#!/bin/bash

set -o nounset
set -o errexit

kubectl delete -f node.yaml --ignore-not-found
kubectl delete -f controller.yaml --ignore-not-found
kubectl delete -f setup-cluster.yaml --ignore-not-found