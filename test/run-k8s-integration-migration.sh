#!/bin/bash

# Optional environment variables
# GCE_PD_OVERLAY_NAME: which Kustomize overlay to deploy with
# GCE_PD_DO_DRIVER_BUILD: if set, don't build the driver from source and just
#   use the driver version from the overlay
# GCE_PD_BOSKOS_RESOURCE_TYPE: name of the boskos resource type to reserve

set -o nounset
set -o errexit





readonly PKGDIR=${GOPATH}/src/sigs.k8s.io/gcp-compute-persistent-disk-csi-driver
export GCE_PD_VERBOSITY=9

source "${PKGDIR}/deploy/common.sh"

readonly overlay_name="${GCE_PD_OVERLAY_NAME:-stable}"
readonly boskos_resource_type="${GCE_PD_BOSKOS_RESOURCE_TYPE:-gce-project}"
readonly do_driver_build="${GCE_PD_DO_DRIVER_BUILD:-true}"
readonly deployment_strategy=${DEPLOYMENT_STRATEGY:-gce}
readonly kube_version=${GCE_PD_KUBE_VERSION:-master}
readonly test_version=${TEST_VERSION:-master}

ensure_var E2E_GOOGLE_APPLICATION_CREDENTIALS

readonly GCE_PD_TEST_FOCUS="\s[V|v]olume\sexpand|\[sig-storage\]\sIn-tree\sVolumes\s\[Driver:\sgcepd\]\s\[Testpattern:\sDynamic\sPV|allowedTopologies|Pod\sDisks|PersistentVolumes\sDefault"


# TODO(#167): Enable reconstructions tests
# TODO: Enabled inline volume tests
# TODO: Fix and enable the following tests. They all exhibit the same testing infra error:
# PersistentVolumes\sGCEPD|\[sig-storage\]\sIn-tree\sVolumes\s\[Driver:\sgcepd\]\s\[Testpattern:.*

# The Error: "PV Create API error: persistentvolumes "gce-" is forbidden: error
# querying GCE PD volume : can not fetch disk, zone is specified
# ("us-central1-b"), but disk name is empty"

make -C ${PKGDIR} test-k8s-integration
${PKGDIR}/bin/k8s-integration-test --kube-version=${kube_version} \
--kube-feature-gates="CSIMigration=true,CSIMigrationGCE=true,ExpandCSIVolumes=true" --run-in-prow=true \
--deploy-overlay-name=${overlay_name} --service-account-file=${E2E_GOOGLE_APPLICATION_CREDENTIALS} \
--do-driver-build=${do_driver_build} --boskos-resource-type=${boskos_resource_type} \
--migration-test=true --test-focus=${GCE_PD_TEST_FOCUS} \
--gce-zone="us-central1-b" --deployment-strategy=${deployment_strategy} --test-version=${test_version} \
--num-nodes=3
