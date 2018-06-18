#!/bin/bash

IAM_NAME="$GCEPD_SA_NAME@$PROJECT.iam.gserviceaccount.com"
gcloud iam service-accounts create $GCEPD_SA_NAME
gcloud iam service-accounts keys create $SA_FILE --iam-account $IAM_NAME
gcloud projects add-iam-policy-binding $PROJECT --member serviceAccount:$IAM_NAME --role roles/compute.storageAdmin roles/compute.admin
