#!/bin/bash

set -o nounset
set -o errexit

readonly PKGDIR=${GOPATH}/src/sigs.k8s.io/gcp-compute-persistent-disk-csi-driver
readonly GCE_PD_TEST_FOCUS="\s[V|v]olume\sexpand|\[sig-storage\]\sIn-tree\sVolumes\s\[Driver:\sgcepd\]\s\[Testpattern:\sDynamic\sPV|allowedTopologies|Pod\sDisks|PersistentVolumes\sDefault"
source "${PKGDIR}/deploy/common.sh"

readonly kube_version=${GCE_PD_KUBE_VERSION:-master}
readonly test_version=${TEST_VERSION:-master}

ensure_var GCE_PD_CSI_STAGING_IMAGE
ensure_var GCE_PD_SA_DIR

make -C ${PKGDIR} test-k8s-integration

# ${PKGDIR}/bin/k8s-integration-test --kube-version=master --run-in-prow=false \
# --staging-image=${GCE_PD_CSI_STAGING_IMAGE} --service-account-file=${GCE_PD_SA_DIR}/cloud-sa.json \
# --deploy-overlay-name=dev --test-focus=${GCE_PD_TEST_FOCUS} \
# --kube-feature-gates="CSIMigration=true,CSIMigrationGCE=true" --migration-test=true --gce-zone="us-central1-b" \
# --deployment-strategy=gce --test-version=${test_version} --gce-zone=${GCE_PD_ZONE} \
# --num-nodes=${NUM_NODES:-3}

# This version of the command does not build the driver or K8s, points to a
# local K8s repo to get the e2e.test binary, and does not bring up or down the cluster
#
${PKGDIR}/bin/k8s-integration-test --run-in-prow=false \
--staging-image=${GCE_PD_CSI_STAGING_IMAGE} --service-account-file=${GCE_PD_SA_DIR}/cloud-sa.json \
--deploy-overlay-name=dev --test-focus=${GCE_PD_TEST_FOCUS} \
--bringup-cluster=false --teardown-cluster=false --local-k8s-dir=$KTOP --migration-test=true \
--do-driver-build=false --gce-zone=${GCE_PD_ZONE} --num-nodes=${NUM_NODES:-3}