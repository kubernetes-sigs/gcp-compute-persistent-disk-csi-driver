#!/bin/bash

set -o nounset
set -o errexit
set -x

echo Using GOPATH $GOPATH

readonly PKGDIR=sigs.k8s.io/gcp-compute-persistent-disk-csi-driver

# This requires application default credentials to be set up, eg by
# `gcloud auth application-default login`

ginkgo --v "test/e2e/tests" -- --project "${PROJECT}" --service-account "${IAM_NAME}" --v=6 --logtostderr
