#!/bin/bash

# This script will setup the given project with a Service Account that has the correct
# restricted permissions to run the gcp_compute_persistent_disk_csi_driver and download
# the keys to a specified directory. This script also authorizes GCE to encrypt/decrypt
# using Cloud KMS keys for the CMEK feature.

# WARNING: This script will delete and recreate the service accounts, bindings, and keys
# associated with ${GCE_PD_SA_NAME}. Great care must be taken to not run the script
# with a service account that is currently in use.

# Args:
# PROJECT: GCP project
# GCE_PD_SA_NAME: Name of the service account to create
# GCE_PD_SA_DIR: Directory to save the service account key
# ENABLE_KMS: If true, it will enable Cloud KMS and configure IAM ACLs.
# CREATE_SA_KEY: (Optional) If true, creates a new service account key and
#   exports it if creating a new service account

set -o nounset
set -o errexit

readonly PKGDIR="${GOPATH}/src/sigs.k8s.io/gcp-compute-persistent-disk-csi-driver"

source "${PKGDIR}/deploy/common.sh"

ensure_var PROJECT
ensure_var GCE_PD_SA_NAME
ensure_var ENABLE_KMS

# Allow the user to pass CREATE_SA_KEY=false to skip the SA key creation
# Ensure the SA directory set, if we're creating the SA_KEY
CREATE_SA_KEY="${CREATE_SA_KEY:-true}"
if [ "${CREATE_SA_KEY}" = true ]; then
  ensure_var GCE_PD_SA_DIR
fi

# If the project id includes the org name in the format "org-name:project", the
# gCloud api will format the project part of the iam email domain as
# "project.org-name"
if [[ $PROJECT == *":"* ]]; then
  IFS=':' read -ra SPLIT <<< "$PROJECT"
  readonly IAM_PROJECT="${SPLIT[1]}.${SPLIT[0]}"
else
  readonly IAM_PROJECT="${PROJECT}"
fi

readonly KUBEDEPLOY="${PKGDIR}/deploy/kubernetes"
readonly BIND_ROLES=$(get_needed_roles)
readonly IAM_NAME="${GCE_PD_SA_NAME}@${IAM_PROJECT}.iam.gserviceaccount.com"
readonly PROJECT_NUMBER=`gcloud projects describe ${PROJECT} --format="value(projectNumber)"`

# Check if SA exists
CREATE_SA=true
SA_JSON=$(gcloud iam service-accounts list --filter="name:${IAM_NAME}" --format="json")
if [ "[]" != "${SA_JSON}" ];
then
	CREATE_SA=false
	echo "Service account ${IAM_NAME} exists. Would you like to create a new one (y) or reuse the existing one (n)"
	read -p "(y/n)" -n 1 -r REPLY
	echo
    if [[ ${REPLY} =~ ^[Yy]$ ]];
	then
		CREATE_SA=true
    fi
fi

if [ "${CREATE_SA}" = true ];
then
	# Delete Service Account Key, if applicable
	if [ "${CREATE_SA_KEY}" = true ]; then
		if [ -f "${GCE_PD_SA_DIR}/cloud-sa.json" ];
		then
		  rm "${GCE_PD_SA_DIR}/cloud-sa.json"
		fi
	fi

	# Delete ALL EXISTING Bindings
	gcloud projects get-iam-policy "${PROJECT}" --format json > "${PKGDIR}/deploy/iam.json"
	sed -i "/serviceAccount:${IAM_NAME}/d" "${PKGDIR}/deploy/iam.json"
	gcloud projects set-iam-policy "${PROJECT}" "${PKGDIR}/deploy/iam.json"
	rm -f "${PKGDIR}/deploy/iam.json"
	# Delete Service Account
	gcloud iam service-accounts delete "${IAM_NAME}" --project "${PROJECT}" --quiet || true

	# Create new Service Account
	gcloud iam service-accounts create "${GCE_PD_SA_NAME}" --project "${PROJECT}"
fi

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

# Bind service account to roles
for role in ${BIND_ROLES}
do
  gcloud projects add-iam-policy-binding "${PROJECT}" --member serviceAccount:"${IAM_NAME}" --role "${role}"
done

# Authorize GCE to encrypt/decrypt using Cloud KMS encryption keys.
# https://cloud.google.com/compute/docs/disks/customer-managed-encryption#before_you_begin
if [ "${ENABLE_KMS}" = true ];
then
  gcloud services enable cloudkms.googleapis.com --project="${PROJECT}"
  gcloud projects add-iam-policy-binding "${PROJECT}" --member serviceAccount:"service-${PROJECT_NUMBER}@compute-system.iam.gserviceaccount.com" --role "roles/cloudkms.cryptoKeyEncrypterDecrypter"
fi

# Export key if needed
if [ "${CREATE_SA}" = true ] && [ "${CREATE_SA_KEY}" = true ];
then
  gcloud iam service-accounts keys create "${GCE_PD_SA_DIR}/cloud-sa.json" --iam-account "${IAM_NAME}" --project "${PROJECT}"
fi
