#!/bin/bash

set -o nounset
set -o errexit

readonly PKGDIR="${GOPATH}/src/sigs.k8s.io/gcp-compute-persistent-disk-csi-driver"
readonly KUBEDEPLOY="${PKGDIR}/deploy/kubernetes"

BIND_ROLES="roles/compute.storageAdmin roles/iam.serviceAccountUser projects/${PROJECT}/roles/gcp_compute_persistent_disk_csi_driver_custom_role"
IAM_NAME="${GCEPD_SA_NAME}@${PROJECT}.iam.gserviceaccount.com"

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
if [ -f $SA_FILE ]; then
  rm "$SA_FILE"
fi
# Delete ALL EXISTING Bindings
gcloud projects get-iam-policy "${PROJECT}" --format json > "${PKGDIR}/deploy/iam.json"
sed -i "/serviceAccount:${IAM_NAME}/d" "${PKGDIR}/deploy/iam.json"
gcloud projects set-iam-policy "${PROJECT}" "${PKGDIR}/deploy/iam.json"
rm -f "${PKGDIR}/deploy/iam.json"
# Delete Service Account
gcloud iam service-accounts delete "$IAM_NAME" --quiet || true

# Create new Service Account and Keys
gcloud iam service-accounts create "${GCEPD_SA_NAME}"
for role in ${BIND_ROLES}
do
  gcloud projects add-iam-policy-binding "${PROJECT}" --member serviceAccount:"${IAM_NAME}" --role ${role}
done
gcloud iam service-accounts keys create "${SA_FILE}" --iam-account "${IAM_NAME}"