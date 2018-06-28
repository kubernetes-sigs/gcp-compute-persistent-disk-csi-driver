#!/bin/bash

set -e
set -x

readonly PKGDIR=sigs.k8s.io/gcp-compute-persistent-disk-csi-driver

go run "$GOPATH/src/${PKGDIR}/test/remote/run_remote/run_remote.go" --logtostderr --v 4 --zone "${ZONE}" --ssh-env gce --delete-instances=true --results-dir=my_test --run-in-prow=true