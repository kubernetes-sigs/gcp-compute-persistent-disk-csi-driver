#!/bin/bash

set -e
set -x

readonly PKGDIR=github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver

go test -timeout 30s "${PKGDIR}/test/sanity/" -run ^TestSanity$
go run "$GOPATH/src/${PKGDIR}/test/remote/run_remote/run_remote.go" --logtostderr --v 2 --project "${PROJECT}" --zone "${ZONE}" --ssh-env gce --delete-instances=false --cleanup=false --results-dir=my_test
