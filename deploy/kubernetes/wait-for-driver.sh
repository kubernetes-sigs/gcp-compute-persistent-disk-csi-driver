#!/bin/bash

# This script waits for the deployments from ./deploy-driver.sh to be ready.

print_usage()
{
    printf "wait-for-driver.sh [--windows]\n"
    printf "\t--windows: wait on the windows deployment rather than linux"
    echo
}

node_daemonset=csi-gce-pd-node
while [[ -n "${1-}" ]]; do
  case $1 in
    --windows )    shift
                   node_daemonset=csi-gce-pd-node-win
                   ;;
    -h | --help )  print_usage
                   exit 1
                   ;;
    * )            print_usage
                   exit 1
                   ;;
  esac
done

kubectl wait -n gce-pd-csi-driver deployment csi-gce-pd-controller --for condition=available

retries=90
while [[ $retries -ge 0 ]];do
    ready=$(kubectl -n gce-pd-csi-driver get daemonset "${node_daemonset}" -o jsonpath="{.status.numberReady}")
    required=$(kubectl -n gce-pd-csi-driver get daemonset "${node_daemonset}" -o jsonpath="{.status.desiredNumberScheduled}")
    if [[ $ready -eq $required ]];then
        echo "Daemonset $node_daemonset found"
        exit 0
    fi
    ((retries--))
    sleep 10s
done
echo "Timeout waiting for node daemonset $node_daemonset"
kubectl describe pods -n gce-pd-csi-driver
exit -1

