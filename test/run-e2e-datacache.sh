#!/bin/bash

set -o nounset
set -o errexit
set -x

echo "Using ${PROJECT}"
PROJECT_NUM="$(gcloud projects describe "${PROJECT}" --format='value(projectNumber)')"
SA="${PROJECT_NUM}-compute@developer.gserviceaccount.com"

CLOUDTOP_HOST=
if [ -z "${CLOUDTOP_HOST}" ] && hostname | grep -q c.googlers.com ; then
  CLOUDTOP_HOST=--cloudtop-host
fi

GOPATH="" ginkgo --v ./test/e2e/tests -- \
  -project="${PROJECT}" \
  -service-account="${SA}" \
  -subnetwork="default" \
  -zones="us-east1-b" \
  -instances-per-zone=2 \
  -machine-type="n2d-standard-4" \
  -local-ssd-count=2 \
  -machine-type-mw="none" \
  -cloudtop-host=true \
  -delete-instances=true \
  "${CLOUDTOP_HOST}" --v=6 --logtostderr \
  -ginkgo.focus="\[DataCache\]|setup caching->write" \
  "$@"

# 
# Flag Notes
#
# • -machine-type-mw="none": Skips provisioning the c3-standard-4 multi-writer/Hyperdisk VMs that are stocking out upstream.
# • -zones="us-east1-b" -instances-per-zone=2: Provisions just 2 n2d-standard-4 VMs (with 2 Local SSDs each) instead of 8 across 2 zones; all 3 Data
# Cache tests run in a single zone (2 of them need 1 VM, and the cross-node test needs 2 VMs in the same zone).
# • -ginkgo.focus="\[DataCache\]|setup caching->write": Runs all 3 Data Cache specs in single_zone_e2e_test.go:1536-1777 (the 3rd spec, "Should create-
# >attach->setup caching->write->detach->attach to different node->mount->read", doesn't have [DataCache] in its title).
# • GOPATH="": In GCEClientAndDriverSetup, if GOPATH is non-empty, the test runner looks for the repo at $GOPATH/src/sigs.k8s.io/gcp-compute-persistent-
# disk-csi-driver instead of the current directory.
# • -cloudtop-host=true & Auth: Bootstraps SSH keys via gcloud compute ssh ... ProxyCommand=corp-ssh-helper (instance.go:332-339). Make sure gcloud auth
# application-default login is active (used by the Go Compute/KMS clients) and -delete-instances=false if you want to keep the 2 VMs up across repeated
# runs.
