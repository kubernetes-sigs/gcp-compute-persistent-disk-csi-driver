#!/usr/bin/env bash
# Copyright 2021 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit
set -o nounset
set -o pipefail

PKG_ROOT=$(realpath $(dirname "${BASH_SOURCE[0]}")/..)

testdirs() {
  find -L "${PKG_ROOT}" -not \( \
      \( \
        -path "${PKG_ROOT}"/vendor/\* \
        -o -path "${PKG_ROOT}"/test/\* \) \
      -prune \) \
    -name \*_test.go -print0 | xargs -0n1 dirname | \
    sed "s|^${PKG_ROOT}/|./|" | LC_ALL=C sort -u
}

coverprofile=${1-"${PKG_ROOT}"/coverage_gcp-compute-persistent-disk-csi-driver.out}
if [[ -z $coverprofile ]]; then
  echo "usage: ./hack/verify-coverage.sh <coverprofile>"
  exit 1
fi

if [[ x"${coverprofile:(-4)}" != x.out ]] ; then
  echo "coverprofile ${coverprofile} must end in .out"
  exit 1
fi

# Remove any old cover profile so that the run is clean.
rm -f "${coverprofile}"

echo "Verifying coverage"
go test -coverprofile="${coverprofile}" $(testdirs)
