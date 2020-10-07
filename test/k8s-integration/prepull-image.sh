#!/bin/bash

set -o nounset
set -o pipefail
set -o xtrace

# This is taken from prepull.yaml.
readonly prepull_daemonset=prepull-test-containers

wait_on_prepull()
{
    # Wait up to 15 minutes for the test images to be pulled onto the nodes.
    retries=90
    while [[ $retries -ge 0 ]];do
        ready=$(kubectl get daemonset "${prepull_daemonset}" -o jsonpath="{.status.numberReady}")
        required=$(kubectl get daemonset "${prepull_daemonset}" -o jsonpath="{.status.desiredNumberScheduled}")
        if [[ $ready -eq $required ]];then
            echo "Daemonset $prepull_daemonset ready"
            return 0
        fi
        ((retries--))
        sleep 10s
    done
    echo "Timeout waiting for daemonset $prepull_daemonset"
    return -1

}

if [[ -z "${PREPULL_YAML}" ]]; then
    # Pre-pull all the test images. The images are currently hard-coded.
    # Eventually, we should get the list directly from
    # https://github.com/kubernetes-sigs/windows-testing/blob/master/images/PullImages.ps1
    curl https://raw.githubusercontent.com/kubernetes-sigs/windows-testing/master/gce/prepull-1.18.yaml -o prepull.yaml
    PREPULL_YAML=prepull.yaml
    echo ${PREPULL_YAML}
fi

kubectl create -f ${PREPULL_YAML}
wait_on_prepull || exit -1  # Error already printed
# Check the status of the pods.
kubectl get pods -o wide
# Delete the pods anyway since pre-pulling is best-effort
kubectl delete -f ${PREPULL_YAML} --wait=true
