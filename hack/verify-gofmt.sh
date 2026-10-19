#!/usr/bin/env bash
# Copyright 2018 The Kubernetes Authors.
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

echo "Verifying gofmt"

# 1. Find the files and save them to a list
go_files=$(find . -name "*.go" | grep -v "\/vendor\/")

# 2. Check if we actually found any Go files to check
if [[ -z "${go_files}" ]]; then
  echo "No Go files found to verify."
  exit 0
fi

# 3. Temporarily turn off 'errexit' (+e) so we can run xargs gofmt.
# If gofmt finds unformatted code, it will fail, but the script won't crash.
set +o errexit
diff=$(echo "${go_files}" | xargs gofmt -s -d 2>&1)
gofmt_exit_code=$?
set -o errexit

if [[ -n "${diff}" ]]; then
  echo "${diff}"
  echo
  echo "Please run hack/update-gofmt.sh"
  exit 1
fi

echo "Successfully verified gofmt"
