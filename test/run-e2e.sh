#!/bin/bash

set -e
set -x

readonly PKGDIR=sigs.k8s.io/gcp-compute-persistent-disk-csi-driver

go test --v=true "${PKGDIR}/test/e2e" --logtostderr --run-in-prow=true