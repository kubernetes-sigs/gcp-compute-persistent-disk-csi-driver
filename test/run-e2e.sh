#!/bin/bash

set -e
set -x

readonly PKGDIR=sigs.k8s.io/gcp-compute-persistent-disk-csi-driver

go test --timeout 30m --v "${PKGDIR}/test/e2e/tests" --run-in-prow=true --service-account=prow-build@k8s-infra-prow-build.iam.gserviceaccount.com --delete-instances=true --logtostderr "$@"
