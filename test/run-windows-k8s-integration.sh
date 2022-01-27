#!/bin/bash

# Optional environment variables
# GCE_PD_OVERLAY_NAME: which Kustomize overlay to deploy with
# GCE_PD_DO_DRIVER_BUILD: if set, don't build the driver from source and just
#   use the driver version from the overlay
# GCE_PD_BOSKOS_RESOURCE_TYPE: name of the boskos resource type to reserve

set -o xtrace
set -o nounset
set -o errexit

readonly PKGDIR=${GOPATH}/src/sigs.k8s.io/gcp-compute-persistent-disk-csi-driver
readonly overlay_name="${GCE_PD_OVERLAY_NAME:-stable-master}"
readonly boskos_resource_type="${GCE_PD_BOSKOS_RESOURCE_TYPE:-gce-project}"
readonly do_driver_build="${GCE_PD_DO_DRIVER_BUILD:-true}"
readonly teardown_driver=${GCE_PD_TEARDOWN_DRIVER:-true}
readonly deployment_strategy=${DEPLOYMENT_STRATEGY:-gce}
readonly kube_version=${GCE_PD_KUBE_VERSION:-master}
readonly test_version=${TEST_VERSION:-master}
readonly gce_zone=${GCE_CLUSTER_ZONE:-us-central1-b}
readonly use_kubetest2=${USE_KUBETEST2:-true}
readonly num_windows_nodes=${NUM_WINDOWS_NODES:-3}

# build platforms for `make quick-release`
export KUBE_BUILD_PLATFORMS=${KUBE_BUILD_PLATFORMS:-"linux/amd64 windows/amd64"}

make -C "${PKGDIR}" test-k8s-integration

if [ "$use_kubetest2" = true ]; then
    go install sigs.k8s.io/kubetest2@latest;
    go install sigs.k8s.io/kubetest2/kubetest2-gce@latest;
    go install sigs.k8s.io/kubetest2/kubetest2-gke@latest;
    go install sigs.k8s.io/kubetest2/kubetest2-tester-ginkgo@latest;
fi

${PKGDIR}/bin/k8s-integration-test \
    --run-in-prow=true \
    --service-account-file="${E2E_GOOGLE_APPLICATION_CREDENTIALS}" \
    --boskos-resource-type="${boskos_resource_type}" \
    --deployment-strategy="${deployment_strategy}" \
    --gce-zone="${gce_zone}" \
    --platform=windows \
    --bringup-cluster=true \
    --teardown-cluster=true \
    --num-nodes=1 \
    --num-windows-nodes="${num_windows_nodes}" \
    --teardown-driver="${teardown_driver}" \
    --do-driver-build="${do_driver_build}" \
    --deploy-overlay-name="${overlay_name}" \
    --test-version="${test_version}" \
    --kube-version="${kube_version}" \
    --storageclass-files=sc-windows.yaml \
    --snapshotclass-file=pd-volumesnapshotclass.yaml \
    --test-focus='External.Storage' \
    --use-kubetest2="${use_kubetest2}"
