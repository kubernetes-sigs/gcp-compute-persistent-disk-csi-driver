#!/bin/bash

set -e
set -x

readonly PKGDIR=sigs.k8s.io/gcp-compute-persistent-disk-csi-driver

# TODO(#452): Temporarily disabled for development velocity and migration to go
# mod k8s.io/kubernetes/pkg/mount needs to be updated to the one in k8s.io/utils
# to fix an issue with the sanity test's behavior with NodeGetVolumeStats

# go test -timeout 30s "${PKGDIR}/test/sanity/" -run ^TestSanity$
