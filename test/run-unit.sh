#!/bin/bash

set -e
set -x

readonly PKGDIR=sigs.k8s.io/gcp-compute-persistent-disk-csi-driver

go test -timeout 30s "${PKGDIR}/pkg/gce-pd-csi-driver"
# The following have no unit tests yet
#go test -timeout 30s "${PKGDIR}/pkg/mount-manager"
#go test -timeout 30s "${PKGDIR}/pkg/gce-cloud-provider/compute"