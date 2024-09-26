#!/bin/bash

# Common variables
readonly KUSTOMIZE_PATH="${PKGDIR}/bin/kustomize"
readonly VERBOSITY="${GCE_PD_VERBOSITY:-2}"
readonly KUBECTL="${GCE_PD_KUBECTL:-kubectl}"

# Common functions

function ensure_var(){
    if [[ -z "${!1:-}" ]];
    then
        echo "${1} is unset"
        exit 1
    else
        echo "${1} is ${!1}"
    fi
}

function get_needed_roles()
{
	echo "roles/editor roles/compute.storageAdmin roles/iam.serviceAccountUser projects/${PROJECT}/roles/gcp_compute_persistent_disk_csi_driver_custom_role"
}

# Installs kustomize in ${PKGDIR}/bin
function ensure_kustomize()
{
  ensure_var PKGDIR
  "${PKGDIR}/deploy/kubernetes/install-kustomize.sh"
}

# This is a heuristic list of roles that appear to be necessary to run the full developer workflow.
function check_dev_roles()
{
  local needed_roles="
    roles/artifactregistry.admin
    roles/compute.admin
    roles/container.admin
    roles/iam.roleAdmin
    roles/iam.serviceAccountAdmin
    roles/iam.serviceAccountKeyAdmin
    roles/iam.serviceAccountUser
    roles/resourcemanager.projectIamAdmin
    roles/serviceusage.serviceUsageAdmin
  "
  local have_roles=$(gcloud projects get-iam-policy "${PROJECT}")

  missing_roles=
  for r in $needed_roles; do
    echo $have_roles | grep -q $r || missing_roles="$missing_roles $r"
  done

  if [ -n "$missing_roles" ]; then
    echo "${PROJECT} is missing the following roles needed for development: $missing_roles"
    echo $have_roles | egrep -q '(roles/editor|roles/owner)' && echo "${PROJECT} does include roles/editor and/or roles/owner, so the missing roles may not be necessary"
  fi
}
