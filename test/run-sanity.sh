#!/bin/bash

set -e
set -x

readonly PKGDIR=sigs.k8s.io/gcp-compute-persistent-disk-csi-driver

go test -v -timeout 30s "${PKGDIR}/test/sanity/" -run ^TestSanity$