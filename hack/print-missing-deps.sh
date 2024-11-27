#!/bin/bash

# Copyright 2022 The Kubernetes Authors.
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

echo "Verifying Docker Executables have appropriate dependencies"

TEMP_DIR="$(mktemp -d)"
trap 'rm -rf -- "$TEMP_DIR"' EXIT

export CONTAINER_IMAGE="$1"
export CONTAINER_EXPORT_DIR="$TEMP_DIR/image_dir"

extractContainerImage() {
  CONTAINER_ID="$(docker create "$CONTAINER_IMAGE")"
  CONTAINER_EXPORT_TAR="$TEMP_DIR/image.tar"
  docker export "$CONTAINER_ID" -o "$CONTAINER_EXPORT_TAR"
  mkdir -p "$CONTAINER_EXPORT_DIR"
  tar xf "$CONTAINER_EXPORT_TAR" -C "$CONTAINER_EXPORT_DIR"
}

printNeededDeps() {
  readelf -d "$@" 2>&1 | grep NEEDED | awk '{print $5}' | sed -e 's@\[@@g' -e 's@\]@@g'
}

printMissingDep() {
  if ! find "$CONTAINER_EXPORT_DIR" -name "$@" > /dev/null; then
    echo "!!! Missing deps for $@ !!!"
    exit 1
  fi
}

export -f printNeededDeps
export -f printMissingDep

extractContainerImage
/usr/bin/find "$CONTAINER_EXPORT_DIR" -type f -executable -print | /usr/bin/xargs -I {} /bin/bash -c 'printNeededDeps "{}"' | sort | uniq | /usr/bin/xargs -I {} /bin/bash -c 'printMissingDep "{}"'
