#!/usr/bin/env bash

# Copyright 2020 The Kubernetes Authors.
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

KUBE_ROOT=$(dirname "${BASH_SOURCE[0]}")/..
BIN_ROOT="${KUBE_ROOT}/hack/tools/bin"

MINIMUM_METAL_CLI_VERSION=0.6.0-alpha2

goarch=amd64
goos="unknown"
if [[ "${OSTYPE}" == "linux"* ]]; then
  goos="linux"
elif [[ "${OSTYPE}" == "darwin"* ]]; then
  goos="darwin"
fi

if [[ "$goos" == "unknown" ]]; then
  echo "OS '$OSTYPE' not supported. Aborting." >&2
  exit 1
fi

# Ensure the metal tool exists and is a viable version, or installs it
verify_metal_version() {

  # If metal is not available on the path, get it
  if ! [ -x "$(command -v metal)" ]; then
      echo 'metal not found, installing'
      if ! [ -d "${BIN_ROOT}" ]; then
        mkdir -p "${BIN_ROOT}"
      fi
      curl -sLo ${BIN_ROOT}/metal https://github.com/equinix/metal-cli/releases/download/${MINIMUM_METAL_CLI_VERSION}/metal-linux-amd64
      chmod +x ${BIN_ROOT}/metal
  fi

  local metal_version
  # Format is 'packet version 0.1.1'
  metal_version=$(metal -v | grep -E -o '[0-9]+\.[0-9]+\.[0-9]+')
  if [[ "${MINIMUM_METAL_CLI_VERSION}" != $(echo -e "${MINIMUM_METAL_CLI_VERSION}\n${metal_version}" | sort -s -t. -k 1,1 -k 2,2n -k 3,3n | head -n1) ]]; then
    cat <<EOF
Detected packet version: ${metal_version}.
Requires ${MINIMUM_METAL_CLI_VERSION} or greater.
Please install ${MINIMUM_METAL_CLI_VERSION} or later.
EOF
    return 2
  fi
}

verify_metal_version
