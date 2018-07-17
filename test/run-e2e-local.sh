#!/bin/bash

set -o nounset
set -o errexit

readonly PKGDIR=sigs.k8s.io/gcp-compute-persistent-disk-csi-driver

go test --v=true "${PKGDIR}/test/e2e" --logtostderr --project ${PROJECT} --service-account ${IAM_NAME}