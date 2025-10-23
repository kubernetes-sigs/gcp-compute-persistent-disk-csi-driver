#!/bin/bash

# This script will deploy the Google Compute Engine Persistent Disk CSI Driver
# to the currently available Kubernetes cluster

# Note: setup-cluster.yaml depends on the existence of cluster-roles
# system:csi-external-attacher and system:csi-external-provisioner
# which are in Kubernetes version 1.10.5+

# Args:
# GCE_PD_SA_DIR: Directory the service account key has been saved in (generated
#   by setup-project.sh). Ignored if GCE_PD_DRIVER_VERSION == noauth.
# GCE_PD_DRIVER_VERSION: The kustomize overlay to deploy. See
# `deploy/kubernetes/overlays` for your choices.
# NODE_SERVICE_ACCOUNTS: (Optional) Comma-separated list of service accounts
#   that the CSI driver should be allowed to impersonate. If not specified,
#   defaults to project-level serviceAccountUser role.

set -o nounset
set -o errexit
set -x

readonly NAMESPACE="${GCE_PD_DRIVER_NAMESPACE:-gce-pd-csi-driver}"
readonly DEPLOY_VERSION="${GCE_PD_DRIVER_VERSION:-stable-master}"
readonly PKGDIR="${GOPATH}/src/sigs.k8s.io/gcp-compute-persistent-disk-csi-driver"
source "${PKGDIR}/deploy/common.sh"

print_usage()
{
    printf "deploy-driver.sh [--skip-sa-check]\n"
    printf "\t--skip-sa-check: don't check the service account for required roles"
    echo
}

skip_sa_check=
while [ -n "${1-}" ]; do
  case $1 in
    --skip-sa-check ) shift
                      skip_sa_check=true
                      ;;
    -h | --help )     print_usage
                      exit 1
                      ;;
    * )               print_usage
                      exit 1
                      ;;
  esac
done

if [[ ! "${DEPLOY_VERSION}" == *noauth* ]]; then
  ensure_var GCE_PD_SA_DIR
fi

function check_service_account()
{
	# Using bash magic to parse JSON for IAM
	# Grepping for a line with client email returning anything quoted after the colon
	readonly IAM_NAME=$(grep -Po '"client_email": *\K"[^"]*"' "${GCE_PD_SA_DIR}/cloud-sa.json" | tr -d '"')
	readonly PROJECT=$(grep -Po '"project_id": *\K"[^"]*"' "${GCE_PD_SA_DIR}/cloud-sa.json" | tr -d '"')
	readonly GOTTEN_BIND_ROLES=$(gcloud projects get-iam-policy "${PROJECT}" --flatten="bindings[].members" --format='table(bindings.role)' --filter="bindings.members:${IAM_NAME}")
	readonly BIND_ROLES=$(get_needed_roles)
	MISSING_ROLES=false
	for role in ${BIND_ROLES}
	do
		if ! grep -q "$role" <<<"${GOTTEN_BIND_ROLES}" ;
		then
			echo "Missing role: $role"
			MISSING_ROLES=true
		fi
	done

	# Check each node service account for serviceAccountUser role
	if use_scoped_sa_role;
	then
		IFS=',' read -ra NODE_SA_ARRAY <<< "${NODE_SERVICE_ACCOUNTS}"
		for node_sa in "${NODE_SA_ARRAY[@]}";
		do
			node_sa=$(echo "${node_sa}" | xargs) # trim whitespace
			SA_POLICY=$(gcloud iam service-accounts get-iam-policy "${node_sa}" --project="${PROJECT}" --flatten="bindings[].members" --format='table(bindings.role)' --filter="bindings.members:${IAM_NAME}")
			if ! grep -q "${SA_USER_ROLE}" <<< "${SA_POLICY}";
			then
				echo "Missing ${SA_USER_ROLE} for: ${node_sa}"
				MISSING_ROLES=true
			fi
		done
	fi

	if [ "${MISSING_ROLES}" = true ];
	then
		echo "Cannot deploy with missing roles in service account, please run setup-project.sh to setup Service Account"
		exit 1
	fi
}

ensure_kustomize

if [[ "$skip_sa_check" != true ]] && [[ ! "${DEPLOY_VERSION}" == *noauth* ]]; then
  check_service_account
fi

if ! ${KUBECTL} get namespace "${NAMESPACE}" -v="${VERBOSITY}";
then
  ${KUBECTL} create namespace "${NAMESPACE}" -v="${VERBOSITY}"
fi

if [[ ! "${DEPLOY_VERSION}" == *noauth* ]]; then
  if ! ${KUBECTL} get secret cloud-sa -v="${VERBOSITY}" -n "${NAMESPACE}";
  then
    ${KUBECTL} create secret generic cloud-sa -v="${VERBOSITY}" --from-file="${GCE_PD_SA_DIR}/cloud-sa.json" -n "${NAMESPACE}"
  fi
fi

# GKE Required Setup
if ! ${KUBECTL} get clusterrolebinding -v="${VERBOSITY}" cluster-admin-binding;
then
  ${KUBECTL} create clusterrolebinding cluster-admin-binding -v="${VERBOSITY}" --clusterrole cluster-admin --user "$(gcloud config get-value account)"
fi

# Debug log: print ${KUBECTL} version
${KUBECTL} version

readonly tmp_spec=/tmp/gcp-compute-persistent-disk-csi-driver-specs-generated.yaml
${KUSTOMIZE_PATH} build "${PKGDIR}/deploy/kubernetes/overlays/${DEPLOY_VERSION}" | tee $tmp_spec
${KUBECTL} apply -v="${VERBOSITY}" -f $tmp_spec
