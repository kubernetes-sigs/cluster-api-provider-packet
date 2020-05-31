#!/bin/sh

set -e

# default configure URL
DEFAULT_CONFIG_URL=https://api.github.com/repos/packethost/cluster-api-provider-packet/releases/latest
TMPYAML=/tmp/clusterctl-packet.yaml

# might want to use a specific path to clusterctl
CLUSTERCTL=${CLUSTERCTL:-clusterctl}

CONFIG_URL=${CONFIG_URL:-""}

# if the config url was not provided, download it
if [ -z "${CONFIG_URL}" ]; then
	# because github does not have a direct link to an asset
	# this would be easier with jq, but not everyone has jq installed
	YAML_URL=$(curl -s ${DEFAULT_CONFIG_URL} | grep clusterctl.yaml | grep browser_download_url | cut -d ":" -f 2,3  | tr -d "\"")
	curl -L -o ${TMPYAML} ${YAML_URL}
	CONFIG_URL=${TMPYAML}
fi

TEMPLATE_OUT=./out/cluster.yaml

DEFAULT_KUBERNETES_VERSION=1.18.2
DEFAULT_POD_CIDR="172.25.0.0/16"
DEFAULT_SERVICE_CIDR="172.26.0.0/16"
DEFAULT_MASTER_NODE_TYPE="t1.small"
DEFAULT_WORKER_NODE_TYPE="t1.small"
DEFAULT_NODE_OS="ubuntu_18_04"

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
NODE_OS=${NODE_OS:-${DEFAULT_NODE_OS}}
KUBERNETES_VERSION=${KUBERNETES_VERSION:-${DEFAULT_KUBERNETES_VERSION}}
SSH_KEY=${SSH_KEY:-""}

PROJECT_ID=${PACKET_PROJECT_ID}
FACILITY=${PACKET_FACILITY}

# and now export them all so envsubst can use them
export PROJECT_ID FACILITY NODE_OS WORKER_NODE_TYPE MASTER_NODE_TYPE POD_CIDR SERVICE_CIDR SSH_KEY KUBERNETES_VERSION
${CLUSTERCTL} --config=${CONFIG_URL} config cluster ${CLUSTER_NAME} > $TEMPLATE_OUT
# remove any lingering config file
rm -f ${TMPYAML}

echo "Done! See output file at ${TEMPLATE_OUT}. Run:"
echo "   kubectl apply -f ${TEMPLATE_OUT}"
exit 0
