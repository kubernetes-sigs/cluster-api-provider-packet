#!/bin/sh
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

set -e

# might want to use a specific path to clusterctl
CLUSTERCTL=${CLUSTERCTL:-clusterctl}

# might want to use a specific config URL
CONFIG_URL=${CONFIG_URL:-""}
CONFIG_OPT=${CONFIG_OPT:-""}
if [ -n "$CONFIG_URL" ]; then
	CONFIG_OPT="--config ${CONFIG_URL}"
fi

TEMPLATE_OUT=./out/cluster.yaml

DEFAULT_KUBERNETES_VERSION="v1.18.2"
DEFAULT_POD_CIDR="172.25.0.0/16"
DEFAULT_SERVICE_CIDR="172.26.0.0/16"
DEFAULT_MASTER_NODE_TYPE="t1.small"
DEFAULT_WORKER_NODE_TYPE="t1.small"
DEFAULT_NODE_OS="ubuntu_18_04"
DEFAULT_WORKER_MACHINE_COUNT=3
DEFAULT_CONTROL_PLANE_MACHINE_COUNT=3

# check required environment variables
errstring=""

if [ -z "$PACKET_PROJECT_ID" ]; then
	errstring="${errstring} PACKET_PROJECT_ID"
fi
if [ -z "$PACKET_FACILITY" ]; then
	errstring="${errstring} PACKET_FACILITY"
fi

if [ -n "$errstring" ]; then
	echo "must set environment variables: ${errstring}" >&2
	exit 1
fi


# Generate a somewhat unique cluster name. This only needs to be unique per project.
RANDOM_STRING=$(LC_ALL=C tr -dc 'a-zA-Z0-9' < /dev/urandom | head -c5 | tr '[:upper:]' '[:lower:]')
# Human friendly cluster name, limited to 6 characters
HUMAN_FRIENDLY_CLUSTER_NAME=test1
DEFAULT_CLUSTER_NAME=${HUMAN_FRIENDLY_CLUSTER_NAME}-${RANDOM_STRING}

CLUSTER_NAME=${CLUSTER_NAME:-${DEFAULT_CLUSTER_NAME}}
POD_CIDR=${POD_CIDR:-${DEFAULT_POD_CIDR}}
SERVICE_CIDR=${SERVICE_CIDR:-${DEFAULT_SERVICE_CIDR}}
WORKER_NODE_TYPE=${WORKER_NODE_TYPE:-${DEFAULT_WORKER_NODE_TYPE}}
MASTER_NODE_TYPE=${MASTER_NODE_TYPE:-${DEFAULT_MASTER_NODE_TYPE}}
WORKER_MACHINE_COUNT=${WORKER_MACHINE_COUNT:-${DEFAULT_WORKER_MACHINE_COUNT}}
CONTROL_PLANE_MACHINE_COUNT=${CONTROL_PLANE_MACHINE_COUNT:-${DEFAULT_CONTROL_PLANE_MACHINE_COUNT}}
NODE_OS=${NODE_OS:-${DEFAULT_NODE_OS}}
KUBERNETES_VERSION=${KUBERNETES_VERSION:-${DEFAULT_KUBERNETES_VERSION}}
SSH_KEY=${SSH_KEY:-""}

PROJECT_ID=${PACKET_PROJECT_ID}
FACILITY=${PACKET_FACILITY}

# and now export them all so envsubst can use them
export PROJECT_ID FACILITY NODE_OS WORKER_NODE_TYPE MASTER_NODE_TYPE POD_CIDR SERVICE_CIDR SSH_KEY KUBERNETES_VERSION WORKER_MACHINE_COUNT CONTROL_PLANE_MACHINE_COUNT
${CLUSTERCTL} ${CONFIG_OPT} config cluster ${CLUSTER_NAME} --from file://$PWD/templates/cluster-template.yaml > $TEMPLATE_OUT

echo "Done! See output file at ${TEMPLATE_OUT}. Run:"
echo "   kubectl apply -f ${TEMPLATE_OUT}"
exit 0
