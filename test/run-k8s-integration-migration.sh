#!/bin/bash

# Optional environment variables
# GCE_PD_OVERLAY_NAME: which Kustomize overlay to deploy with
# GCE_PD_DO_DRIVER_BUILD: if set, don't build the driver from source and just
#   use the driver version from the overlay
# GCE_PD_BOSKOS_RESOURCE_TYPE: name of the boskos resource type to reserve

set -o nounset
set -o errexit

export GCE_PD_VERBOSITY=9

readonly PKGDIR=${GOPATH}/src/sigs.k8s.io/gcp-compute-persistent-disk-csi-driver

readonly overlay_name="${GCE_PD_OVERLAY_NAME:-stable-master}"
readonly boskos_resource_type="${GCE_PD_BOSKOS_RESOURCE_TYPE:-gce-project}"
readonly do_driver_build="${GCE_PD_DO_DRIVER_BUILD:-true}"
readonly deployment_strategy=${DEPLOYMENT_STRATEGY:-gce}
readonly kube_version=${GCE_PD_KUBE_VERSION:-master}
readonly test_version=${TEST_VERSION:-master}
readonly use_kubetest2=${USE_KUBETEST2:-false}

make -C "${PKGDIR}" test-k8s-integration

if [ "$use_kubetest2" = true ]; then
    kt2_version=22d5b1410bef09ae679fa5813a5f0d196b6079de
    go install sigs.k8s.io/kubetest2@${kt2_version}
    go install sigs.k8s.io/kubetest2/kubetest2-gce@${kt2_version}
    go install sigs.k8s.io/kubetest2/kubetest2-gke@${kt2_version}
    go install sigs.k8s.io/kubetest2/kubetest2-tester-ginkgo@${kt2_version}
fi

readonly GCE_PD_TEST_FOCUS="PersistentVolumes\sGCEPD|[V|v]olume\sexpand|\[sig-storage\]\sIn-tree\sVolumes\s\[Driver:\sgcepd\]|allowedTopologies|Pod\sDisks|PersistentVolumes\sDefault"

# TODO(#167): Enable reconstructions tests

"${PKGDIR}/bin/k8s-integration-test" --kube-version="${kube_version}" \
--kube-feature-gates="CSIMigration=true,CSIMigrationGCE=true,ExpandCSIVolumes=true" --run-in-prow=true \
--deploy-overlay-name="${overlay_name}" --service-account-file="${E2E_GOOGLE_APPLICATION_CREDENTIALS}" \
--do-driver-build="${do_driver_build}" --boskos-resource-type="${boskos_resource_type}" \
--migration-test=true --test-focus="${GCE_PD_TEST_FOCUS}" \
--gce-zone="us-central1-b" --deployment-strategy="${deployment_strategy}" --test-version="${test_version}" \
--num-nodes=3 --use-kubetest2=${use_kubetest2}
