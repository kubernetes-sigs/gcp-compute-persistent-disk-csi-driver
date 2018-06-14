#!/bin/bash
source ./common.sh
kubectl create secret generic cloud-sa --from-file=$SA_FILE
kubectl create -f setup-cluster.yaml
kubectl create -f node.yaml
kubectl create -f controller.yaml