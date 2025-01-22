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
readonly do_k8s_build="${GCE_PD_DO_K8S_BUILD:-false}"
readonly deployment_strategy=${DEPLOYMENT_STRATEGY:-gce}
readonly gke_cluster_version=${GKE_CLUSTER_VERSION:-latest}
readonly kube_version=${GCE_PD_KUBE_VERSION:-latest}
readonly test_version=${TEST_VERSION:-latest}
readonly gce_zone=${GCE_CLUSTER_ZONE:-us-central1-b}
readonly gce_region=${GCE_CLUSTER_REGION:-}
readonly image_type=${IMAGE_TYPE:-cos_containerd}
readonly use_gke_managed_driver=${USE_GKE_MANAGED_DRIVER:-false}
readonly gke_release_channel=${GKE_RELEASE_CHANNEL:-""}
readonly teardown_driver=${GCE_PD_TEARDOWN_DRIVER:-true}
readonly gke_node_version=${GKE_NODE_VERSION:-}
readonly use_kubetest2=${USE_KUBETEST2:-true}
readonly migration_test=${MIGRATION_TEST:-false}

export GCE_PD_VERBOSITY=9

readonly GCE_PD_TEST_FOCUS="PersistentVolumes\sGCEPD|[V|v]olume\sexpand|\[sig-storage\]\sIn-tree\sVolumes\s\[Driver:\sgcepd\]|allowedTopologies|Pod\sDisks|PersistentVolumes\sDefault"

make -C "${PKGDIR}" test-k8s-integration

if [ "$use_kubetest2" = true ]; then
    kt2_version=22d5b1410bef09ae679fa5813a5f0d196b6079de
    go install sigs.k8s.io/kubetest2@${kt2_version}
    go install sigs.k8s.io/kubetest2/kubetest2-gce@${kt2_version}
    go install sigs.k8s.io/kubetest2/kubetest2-gke@${kt2_version}
    go install sigs.k8s.io/kubetest2/kubetest2-tester-ginkgo@${kt2_version}
fi

base_cmd="${PKGDIR}/bin/k8s-integration-test \
            --run-in-prow=true --service-account-file=${E2E_GOOGLE_APPLICATION_CREDENTIALS} \
            --do-driver-build=${do_driver_build} --teardown-driver=${teardown_driver} \
            --do-k8s-build=${do_k8s_build} --boskos-resource-type=${boskos_resource_type} \
            --storageclass-files=sc-balanced.yaml --snapshotclass-files=pd-volumesnapshotclass.yaml \
            --volumeattributesclass-files=hdb-volumeattributesclass.yaml \
            --storageclass-for-vac-file=sc-hdb.yaml \
            --kube-runtime-config=api/all=true \
            --deployment-strategy=${deployment_strategy} --test-version=${test_version} \
            --num-nodes=3 --image-type=${image_type} --use-kubetest2=${use_kubetest2}"

if [ "$use_gke_managed_driver" = false ]; then
  base_cmd="${base_cmd} --deploy-overlay-name=${overlay_name}"
else
  base_cmd="${base_cmd} --use-gke-managed-driver=${use_gke_managed_driver}"
fi

if [ "$deployment_strategy" = "gke" ]; then
  if [ "$gke_release_channel" ]; then
    base_cmd="${base_cmd} --gke-release-channel=${gke_release_channel}"
  else
    base_cmd="${base_cmd} --gke-cluster-version=${gke_cluster_version}"
  fi
else
  base_cmd="${base_cmd} --kube-version=${kube_version} --kube-feature-gates=VolumeAttributesClass=true"
fi

if [ -z "$gce_region" ]; then
  base_cmd="${base_cmd} --gce-zone=${gce_zone}"
else
  base_cmd="${base_cmd} --gce-region=${gce_region}"
fi

if [ -z "$gke_node_version" ]; then
  base_cmd="${base_cmd} --gke-node-version=${gke_node_version}"
fi

if [ "$migration_test" = "true" ]; then
  base_cmd="${base_cmd} --migration-test=true --test-focus='${GCE_PD_TEST_FOCUS}'"
else
  base_cmd="${base_cmd} --test-focus='External.Storage'"
fi

eval "$base_cmd"
