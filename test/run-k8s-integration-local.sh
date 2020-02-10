#!/bin/bash

set -o nounset
set -o errexit

readonly PKGDIR=${GOPATH}/src/sigs.k8s.io/gcp-compute-persistent-disk-csi-driver
readonly gke_cluster_version=${GKE_CLUSTER_VERSION:-latest}
readonly kube_version=${KUBE_VERSION:-master}
readonly test_version=${TEST_VERSION:-master}

source "${PKGDIR}/deploy/common.sh"

ensure_var GCE_PD_CSI_STAGING_IMAGE
ensure_var GCE_PD_SA_DIR

make -C ${PKGDIR} test-k8s-integration

# This version of the command creates a GKE cluster. It also downloads and builds a k8s release
# so that it can run the test specified

# ${PKGDIR}/bin/k8s-integration-test --run-in-prow=false \
# --staging-image=${GCE_PD_CSI_STAGING_IMAGE} --service-account-file=${GCE_PD_SA_DIR}/cloud-sa.json \
# --deploy-overlay-name=dev --storageclass-file=sc-standard.yaml \
# --test-focus="External.Storage" --gce-zone="us-central1-b" \
# --deployment-strategy=gke --gke-cluster-version=${gke_cluster_version} \
# --test-version=${test_version} --num-nodes=3

# This version of the command creates a GCE cluster. It downloads and builds two k8s releases,
# one for the cluster and one for the tests, unless the cluster and test versioning is the same.

# ${PKGDIR}/bin/k8s-integration-test --run-in-prow=false \
# --staging-image=${GCE_PD_CSI_STAGING_IMAGE} --service-account-file=${GCE_PD_SA_DIR}/cloud-sa.json \
# --deploy-overlay-name=dev --storageclass-file=sc-standard.yaml \
# --test-focus="External.Storage" --gce-zone="us-central1-b" \
# --deployment-strategy=gce --kube-version=${kube_version} \
# --test-version=${test_version} --num-nodes=3


# This version of the command creates a regional GKE cluster. It will test with
# the latest GKE version and the master test version

# ${PKGDIR}/bin/k8s-integration-test --run-in-prow=false \
# --staging-image=${GCE_PD_CSI_STAGING_IMAGE} --service-account-file=${GCE_PD_SA_DIR}/cloud-sa.json \
# --deploy-overlay-name=dev --bringup-cluster=false --teardown-cluster=false \
# --storageclass-file=sc-standard.yaml --do-driver-build=false  --test-focus="schedule.a.pod.with.AllowedTopologies" \
# --gce-region="us-central1" --num-nodes=${NUM_NODES:-3} --gke-cluster-version="latest" --deployment-strategy="gke" \
# --test-version="master"

# This version of the command does not build the driver or K8s, points to a
# local K8s repo to get the e2e.test binary, and does not bring up or down the cluster

${PKGDIR}/bin/k8s-integration-test --run-in-prow=false \
--staging-image=${GCE_PD_CSI_STAGING_IMAGE} --service-account-file=${GCE_PD_SA_DIR}/cloud-sa.json \
--deploy-overlay-name=dev --bringup-cluster=false --teardown-cluster=false --local-k8s-dir=$KTOP \
--storageclass-file=sc-standard.yaml --do-driver-build=true  --test-focus="External.Storage" \
--gce-zone="us-central1-b" --num-nodes=${NUM_NODES:-3}
