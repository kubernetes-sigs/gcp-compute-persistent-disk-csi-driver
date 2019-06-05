#!/bin/bash

set -o nounset
set -o errexit

readonly PKGDIR=${GOPATH}/src/sigs.k8s.io/gcp-compute-persistent-disk-csi-driver
readonly deployment_strategy=${DEPLOYMENT_STRATEGY:-gce}

source "${PKGDIR}/deploy/common.sh"

ensure_var GCE_PD_CSI_STAGING_IMAGE
ensure_var GCE_PD_SA_DIR

make -C ${PKGDIR} test-k8s-integration

# ${PKGDIR}/bin/k8s-integration-test --kube-version=master --run-in-prow=false \
# --staging-image=${GCE_PD_CSI_STAGING_IMAGE} --service-account-file=${GCE_PD_SA_DIR}/cloud-sa.json \
# --deploy-overlay-name=dev --storageclass-file=sc-standard.yaml \
# --test-focus="External.Storage" --gce-zone="us-central1-b" \
# --deployment-strategy=${deployment_strategy}

# This version of the command does not build the driver or K8s, points to a
# local K8s repo to get the e2e.test binary, and does not bring up or down the cluster
#
${PKGDIR}/bin/k8s-integration-test --kube-version=master --run-in-prow=false \
--staging-image=${GCE_PD_CSI_STAGING_IMAGE} --service-account-file=${GCE_PD_SA_DIR}/cloud-sa.json \
--deploy-overlay-name=dev --bringup-cluster=false --teardown-cluster=false --local-k8s-dir=$KTOP \
--storageclass-file=sc-standard.yaml --do-driver-build=true  --test-focus="External.Storage" \
--gce-zone="us-central1-b"
