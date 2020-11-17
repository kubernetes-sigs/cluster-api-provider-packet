#!/bin/bash

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

###############################################################################

# This script is executed by presubmit `pull-cluster-api-provider-azure-e2e`
# To run locally, set PACKET_API_KEY, PROJECT_ID, ``

set -o errexit
set -o nounset
set -o pipefail

REPO_ROOT=$(dirname "${BASH_SOURCE[0]}")/..
cd "${REPO_ROOT}" || exit 1

# shellcheck source=../hack/ensure-go.sh
source "${REPO_ROOT}/hack/ensure-go.sh"
# shellcheck source=../hack/ensure-kind.sh
source "${REPO_ROOT}/hack/ensure-kind.sh"
# shellcheck source=../hack/ensure-kubectl.sh
source "${REPO_ROOT}/hack/ensure-kubectl.sh"
# shellcheck source=../hack/ensure-kustomize.sh
source "${REPO_ROOT}/hack/ensure-kustomize.sh"
# shellcheck source=../hack/ensure-packet-cli.sh
source "${REPO_ROOT}/hack/ensure-packet-cli.sh"

# Verify the required Environment Variables are present.
: "${PACKET_API_KEY:?Environment variable empty or not defined.}"
: "${PROJECT_ID:?Environment variable empty or not defined.}"

get_random_facility() {
    # local FACILITIES=("sjc1" "lax1" "ams1" "dfw2" "ewr1" "ny5")
    local FACILITIES=("ewr1")
    echo "${FACILITIES[${RANDOM} % ${#FACILITIES[@]}]}"
}

export GINKGO_NODES=3
export FACILITY="${FACILITY:-$(get_random_facility)}"
export PACKET_TOKEN=${PACKET_API_KEY}

export SSH_KEY_NAME=capp-e2e-$(head /dev/urandom | tr -dc A-Za-z0-9 | head -c 12 ; echo '')
export SSH_KEY_PATH=/tmp/${SSH_KEY_NAME}
export SSH_KEY_UUID=""
create_ssh_key() {
    echo "generating new ssh key"
    ssh-keygen -t rsa -f ${SSH_KEY_PATH} -N '' 2>/dev/null <<< y >/dev/null
    echo "importing ssh key "
    SSH_KEY_STRING=$(cat ${SSH_KEY_PATH}.pub)
    SSH_KEY_UUID=$(packet ssh-key create --key "${SSH_KEY_STRING}" --label "${SSH_KEY_NAME}" --json  | jq -r '.id')
}

cleanup() {
    echo "removing ssh key"
    packet ssh-key delete --id ${SSH_KEY_UUID} --force || true
    rm -f ${SSH_KEY_PATH} || true

    ${REPO_ROOT}/hack/log/redact.sh || true
}

create_ssh_key
trap cleanup EXIT

export SSH_KEY=${SSH_KEY_NAME}
make conformance
test_status="${?}"
