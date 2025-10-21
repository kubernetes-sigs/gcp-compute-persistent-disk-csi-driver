#!/bin/bash

# Common variables
readonly KUSTOMIZE_PATH="${PKGDIR}/bin/kustomize"
readonly VERBOSITY="${GCE_PD_VERBOSITY:-2}"
readonly KUBECTL="${GCE_PD_KUBECTL:-kubectl}"
readonly SA_USER_ROLE="roles/iam.serviceAccountUser"

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

# Return true if scoping serviceAccountUser role to node service accounts
function use_scoped_sa_role()
{
	[ -n "${NODE_SERVICE_ACCOUNTS:-}" ]
}

function get_needed_roles()
{
	ROLES="roles/editor roles/compute.storageAdmin projects/${PROJECT}/roles/gcp_compute_persistent_disk_csi_driver_custom_role"
	# Grant the role at the project level if no node service accounts are provided.
	if ! use_scoped_sa_role;
	then
		ROLES+=" ${SA_USER_ROLE}"
	fi
	echo "${ROLES}"
}

# Installs kustomize in ${PKGDIR}/bin
function ensure_kustomize()
{
  ensure_var PKGDIR
  "${PKGDIR}/deploy/kubernetes/install-kustomize.sh"
}
