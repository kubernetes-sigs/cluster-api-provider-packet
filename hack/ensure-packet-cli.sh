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

GOPATH_BIN="$(go env GOPATH)/bin"
MINIMUM_PACKET_CLI_VERSION=0.1.1

# Ensure the doctl tool exists and is a viable version, or installs it
verify_packet_version() {

  # If doctl is not available on the path, get it
  if ! [ -x "$(command -v packet)" ]; then
    if [[ "${OSTYPE}" == "linux-gnu" ]]; then
      echo 'packet not found, installing'
      if ! [ -d "${GOPATH_BIN}" ]; then
        mkdir -p "${GOPATH_BIN}"
      fi
      curl -sLo packet https://github.com/packethost/packet-cli/releases/download/${MINIMUM_PACKET_CLI_VERSION}/packet-linux-amd64 -O packet
      chmod +x "${GOPATH_BIN}/packet"
    else
      echo "Missing required binary in path: packet"
      return 2
    fi
  fi

  local packet_version
  # Format is 'packet version 0.1.1'
  packet_version=$(packet -v | grep -E -o '[0-9]+\.[0-9]+\.[0-9]+')
  if [[ "${MINIMUM_PACKET_CLI_VERSION}" != $(echo -e "${MINIMUM_PACKET_CLI_VERSION}\n${packet_version}" | sort -s -t. -k 1,1 -k 2,2n -k 3,3n | head -n1) ]]; then
    cat <<EOF
Detected packet version: ${packet_version}.
Requires ${MINIMUM_PACKET_CLI_VERSION} or greater.
Please install ${MINIMUM_PACKET_CLI_VERSION} or later.
EOF
    return 2
  fi
}

verify_packet_version
