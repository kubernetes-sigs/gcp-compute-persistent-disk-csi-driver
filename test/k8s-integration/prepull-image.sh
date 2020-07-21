#!/bin/bash

set -o nounset
set -o pipefail
set -o xtrace

if [[ -z "${PREPULL_IMAGE}" ]]; then
    # Pre-pull all the test images. The images are currently hard-coded.
    # Eventually, we should get the list directly from
    # https://github.com/kubernetes-sigs/windows-testing/blob/master/images/PullImages.ps1
    curl https://raw.githubusercontent.com/kubernetes-sigs/windows-testing/master/gce/prepull-1.18.yaml -o prepull.yaml
    PREPULL_IMAGE=prepull.yaml
    echo ${PREPULL_IMAGE}
fi

kubectl create -f ${PREPULL_IMAGE}
# Wait 10 minutes for the test images to be pulled onto the nodes.
sleep 15m
echo "sleep 15m"
# Check the status of the pods.
kubectl get pods -o wide
# Delete the pods anyway since pre-pulling is best-effort
kubectl delete -f ${PREPULL_IMAGE}
# Wait a few more minutes for the pod to be cleaned up.
sleep 5m