#!/bin/bash

set -o nounset
set -o errexit

IAM_NAME="${GCEPD_SA_NAME}@${PROJECT}.iam.gserviceaccount.com"

# Cleanup old Service Account and Key
if [ -f $SA_FILE ]; then
  rm "$SA_FILE"
fi
gcloud iam service-accounts delete "$IAM_NAME" --quiet || true
# TODO: Delete ALL policy bindings

# Create new Service Account and Keys
gcloud iam service-accounts create "${GCEPD_SA_NAME}"
gcloud iam service-accounts keys create "${SA_FILE}" --iam-account "${IAM_NAME}"
gcloud projects add-iam-policy-binding "${PROJECT}" --member serviceAccount:"${IAM_NAME}" --role roles/compute.admin
gcloud projects add-iam-policy-binding "${PROJECT}" --member serviceAccount:"${IAM_NAME}" --role roles/iam.serviceAccountUser
