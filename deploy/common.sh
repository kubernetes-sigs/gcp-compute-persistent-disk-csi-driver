#!/bin/bash

# Common functions

function ensure_var(){
    if [[ -v "${1}" ]];
    then
        echo "${1} is ${!1}"
    else
        echo "${1} is unset"
        exit 1
    fi
}

function get_needed_roles()
{
	echo "roles/compute.storageAdmin roles/iam.serviceAccountUser projects/${PROJECT}/roles/gcp_compute_persistent_disk_csi_driver_custom_role"
}