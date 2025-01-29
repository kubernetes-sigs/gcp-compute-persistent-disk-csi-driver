#!/bin/bash

set -e
set -x

readonly PKGDIR=sigs.k8s.io/gcp-compute-persistent-disk-csi-driver

TIMEOUT=40m
if [ "$RUN_CONTROLLER_MODIFY_VOLUME_TESTS" = true ]; then
    TIMEOUT=45m
fi

go test --timeout "${TIMEOUT}" --v "${PKGDIR}/test/e2e/tests" --run-in-prow=true --delete-instances=true --logtostderr $@
