#!/bin/sh

set -e

TEMPLATE_IN=./templates/cluster-template.yaml
TEMPLATE_OUT=./out/cluster.yaml

DEFAULT_POD_CIDR="172.25.0.0/16"
DEFAULT_SERVICE_CIDR="172.26.0.0/16"
DEFAULT_MASTER_NODE_TYPE="t1.small"
DEFAULT_WORKER_NODE_TYPE="t1.small"
DEFAULT_NODE_OS="ubuntu_18_04"

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
NODE_OS=${NODE_OS:=${DEFAULT_NODE_OS}}

if [ -z "$PACKET_PROJECT_ID" ]; then
	echo "must set environment variable PACKET_PROJECT_ID" >&2
	exit 1
fi
PROJECT_ID=${PACKET_PROJECT_ID}
if [ -z "$PACKET_FACILITY" ]; then
	echo "must set environment variable PACKET_FACILITY" >&2
	exit 1
fi
FACILITY=${PACKET_FACILITY}

# uses the template to generate an example
if [ ! -e "$TEMPLATE_IN" ]; then
	echo "failed to find template file $TEMPLATE_IN" >&2
	exit 1
fi

# and now export them all so envsubst can use them
export PROJECT_ID FACILITY NODE_OS WORKER_NODE_TYPE MASTER_NODE_TYPE POD_CIDR SERVICE_CIDR CLUSTER_NAME SSH_KEY
cat $TEMPLATE_IN | envsubst > $TEMPLATE_OUT

echo "Done! See output file at ${TEMPLATE_OUT}"
exit 0
