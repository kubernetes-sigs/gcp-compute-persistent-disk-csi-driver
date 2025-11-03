#!/bin/bash

set -e
set -x

readonly PKGDIR=sigs.k8s.io/gcp-compute-persistent-disk-csi-driver

TIMEOUT=60m

go test --timeout "${TIMEOUT}" --v "${PKGDIR}/test/e2e/tests" --run-in-prow=true --delete-instances=true --logtostderr $@
