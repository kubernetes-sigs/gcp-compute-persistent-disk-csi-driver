#!/bin/bash

set -e
set -x

readonly PKGDIR=sigs.k8s.io/gcp-compute-persistent-disk-csi-driver

go run "$GOPATH/src/${PKGDIR}/test/remote/run_remote/run_remote.go" --logtostderr --v 2 --project "${PROJECT}" --zone "${ZONE}" --ssh-env gce --delete-instances=true --cleanup=true --results-dir=my_test  --service-account="${IAM_NAME}"