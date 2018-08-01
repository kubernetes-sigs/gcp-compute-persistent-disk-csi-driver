#!/bin/bash

# This script will setup the given project with a Service Account that has the correct
# restricted permissions to run the gcp_compute_persistent_disk_csi_driver and download
# the keys to a specified directory

# WARNING: This script will delete and recreate the service accounts, bindings, and keys
# associated with ${GCE_PD_SA_NAME}. Great care must be taken to not run the script
# with a service account that is currently in use.

# Args:
# PROJECT: GCP project
# GCE_PD_SA_NAME: Name of the service account to create
# GCE_PD_SA_DIR: Directory to save the service account key


set -o nounset
set -o errexit

readonly PKGDIR="${GOPATH}/src/sigs.k8s.io/gcp-compute-persistent-disk-csi-driver"
readonly KUBEDEPLOY="${PKGDIR}/deploy/kubernetes"
readonly BIND_ROLES="roles/compute.storageAdmin roles/iam.serviceAccountUser projects/${PROJECT}/roles/gcp_compute_persistent_disk_csi_driver_custom_role"
readonly IAM_NAME="${GCE_PD_SA_NAME}@${PROJECT}.iam.gserviceaccount.com"

# Create or Update Custom Role
if gcloud iam roles describe gcp_compute_persistent_disk_csi_driver_custom_role --project "${PROJECT}";
then
  gcloud iam roles update gcp_compute_persistent_disk_csi_driver_custom_role --quiet \
          --project "${PROJECT}"                                                     \
          --file "${PKGDIR}/deploy/gcp-compute-persistent-disk-csi-driver-custom-role.yaml"
else
  gcloud iam roles create gcp_compute_persistent_disk_csi_driver_custom_role --quiet \
    --project "${PROJECT}"                                                           \
    --file "${PKGDIR}/deploy/gcp-compute-persistent-disk-csi-driver-custom-role.yaml"
fi

# Delete Service Account Key
if [ -f "${GCE_PD_SA_DIR}/cloud-sa.json" ]; then
  rm "${GCE_PD_SA_DIR}/cloud-sa.json"
fi
# Delete ALL EXISTING Bindings
gcloud projects get-iam-policy "${PROJECT}" --format json > "${PKGDIR}/deploy/iam.json"
sed -i "/serviceAccount:${IAM_NAME}/d" "${PKGDIR}/deploy/iam.json"
gcloud projects set-iam-policy "${PROJECT}" "${PKGDIR}/deploy/iam.json"
rm -f "${PKGDIR}/deploy/iam.json"
# Delete Service Account
gcloud iam service-accounts delete "${IAM_NAME}" --project "${PROJECT}" --quiet || true

# Create new Service Account and Keys
gcloud iam service-accounts create "${GCE_PD_SA_NAME}" --project "${PROJECT}"
for role in ${BIND_ROLES}
do
  gcloud projects add-iam-policy-binding "${PROJECT}" --member serviceAccount:"${IAM_NAME}" --role ${role}
done
gcloud iam service-accounts keys create "${GCE_PD_SA_DIR}/cloud-sa.json" --iam-account "${IAM_NAME}" --project "${PROJECT}"
