#!/bin/bash

# Optional environment variables
# GCE_PD_OVERLAY_NAME: which Kustomize overlay to deploy with
# GCE_PD_DO_DRIVER_BUILD: if set, don't build the driver from source and just
#   use the driver version from the overlay
# GCE_PD_BOSKOS_RESOURCE_TYPE: name of the boskos resource type to reserve

set -o nounset
set -o errexit

readonly PKGDIR=${GOPATH}/src/sigs.k8s.io/gcp-compute-persistent-disk-csi-driver
readonly overlay_name="${GCE_PD_OVERLAY_NAME:-stable}"
readonly boskos_resource_type="${GCE_PD_BOSKOS_RESOURCE_TYPE:-gce-project}"
readonly do_driver_build="${GCE_PD_DO_DRIVER_BUILD:-true}"
readonly deployment_strategy=${DEPLOYMENT_STRATEGY:-gce}
readonly gke_cluster_version=${GKE_CLUSTER_VERSION:-latest}
readonly kube_version=${GCE_PD_KUBE_VERSION:-master}
readonly test_version=${TEST_VERSION:-master}
readonly gce_zone=${GCE_CLUSTER_ZONE:-us-central1-b}
readonly gce_region=${GCE_CLUSTER_REGION:-}
readonly image_type=${IMAGE_TYPE:-cos}

export GCE_PD_VERBOSITY=9

make -C ${PKGDIR} test-k8s-integration

base_cmd="${PKGDIR}/bin/k8s-integration-test \
            --run-in-prow=true --deploy-overlay-name=${overlay_name} --service-account-file=${E2E_GOOGLE_APPLICATION_CREDENTIALS} \
            --do-driver-build=${do_driver_build} --boskos-resource-type=${boskos_resource_type} \
            --storageclass-file=sc-standard.yaml --test-focus="External.Storage" \
            --deployment-strategy=${deployment_strategy} --test-version=${test_version} --num-nodes=3 \
            --image-type=${image_type}"

if [ "$deployment_strategy" = "gke" ]; then
  base_cmd="${base_cmd} --gke-cluster-version=${gke_cluster_version}"
else
  base_cmd="${base_cmd} --kube-version=${kube_version}"
fi

if [ -z "$gce_region" ]; then
  base_cmd="${base_cmd} --gce-zone=${gce_zone}"
else
  base_cmd="${base_cmd} --gce-region=${gce_region}"
fi

# TODO: When beta csi snapshotter sidecar is promoted to stable, add stable
# overlay to the check.
if [[ "$overlay_name" =~ .*"gke-release-staging".* ]]; then
  base_cmd="${base_cmd} --snapshotclass-file=pd-volumesnapshotclass.yaml"
fi

eval $base_cmd
