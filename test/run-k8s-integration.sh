#!/bin/bash

set -o nounset
set -o errexit

readonly PKGDIR=${GOPATH}/src/sigs.k8s.io/gcp-compute-persistent-disk-csi-driver

make -C ${PKGDIR} test-k8s-integration
${PKGDIR}/bin/k8s-integration-test --kube-version=master --run-in-prow=true --deploy-overlay-name=prow-head-template --service-account-file=${E2E_GOOGLE_APPLICATION_CREDENTIALS}
