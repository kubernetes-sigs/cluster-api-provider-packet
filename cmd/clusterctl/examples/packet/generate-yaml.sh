#!/bin/bash

# Copyright 2019 Packet Inc.
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

doexitdir() {
	echo "Cannot determine working directory. generate-yaml.sh must be run from the Packet cluster-api provider repository root, or the examples/packet directory."
	exit 1
}

# we might be executed from the same dir as the templates, or from the root dir of the repository
# we have a symlink in the root dir for convenience
case $PWD in
*cluster-api-provider-packet)
	if [ -d "cmd/clusterctl/examples/packet" ]; then
		BASEDIR="./cmd/clusterctl/examples/packet"
	else
		doexitdir
	fi
	;;
*cmd/clusterctl/examples/packet)
	BASEDIR="."
	;;
*)
	doexitdir
	;;
esac


# Generate a somewhat unique cluster name. This only needs to be unique per project.
RANDOM_STRING=$(head -c5 < <(LC_ALL=C tr -dc 'a-zA-Z0-9' < /dev/urandom) | tr '[:upper:]' '[:lower:]')
# Human friendly cluster name, limited to 6 characters
HUMAN_FRIENDLY_CLUSTER_NAME=test1
GENERATED_CLUSTER_NAME=${HUMAN_FRIENDLY_CLUSTER_NAME}-${RANDOM_STRING}
CLUSTER_NAME=${CLUSTER_NAME:-${GENERATED_CLUSTER_NAME}}

FACILITY="${FACILITY:-ewr1}"

OUTPUT_DIR=out/packet

MACHINE_TEMPLATE_FILE=${BASEDIR}/machines.yaml.template
MACHINE_GENERATED_FILE=${OUTPUT_DIR}/machines.yaml
CLUSTER_TEMPLATE_FILE=${BASEDIR}/cluster.yaml.template
CLUSTER_GENERATED_FILE=${OUTPUT_DIR}/cluster.yaml
ADDON_TEMPLATE_FILE=${BASEDIR}/addons.yaml.template
ADDON_GENERATED_FILE=${OUTPUT_DIR}/addons.yaml

SSH_PRIVATE_FILE=${OUTPUT_DIR}/id_rsa
SSH_PUBLIC_FILE=${SSH_PRIVATE_FILE}.pub
SSH_USER_PLAIN=clusterapi
# By default, linux wraps base64 output every 76 cols, so we use 'tr -d' to remove whitespaces.
# Note 'base64 -w0' doesn't work on Mac OS X, which has different flags.
SSH_USER=$(echo -n "$SSH_USER_PLAIN" | base64 | tr -d '\r\n')


OVERWRITE=0

SCRIPT=$(basename $0)
while test $# -gt 0; do
        case "$1" in
          -h|--help)
            echo "$SCRIPT - generates input yaml files for Cluster API on Packet"
            echo " "
            echo "$SCRIPT [options]"
            echo " "
            echo "options:"
            echo "-h, --help                show brief help"
            echo "-f, --force-overwrite     if file to be generated already exists, force script to overwrite it"
            exit 0
            ;;
          -f)
            OVERWRITE=1
            shift
            ;;
          --force-overwrite)
            OVERWRITE=1
            shift
            ;;
          *)
            break
            ;;
        esac
done

if [ $OVERWRITE -ne 1 ] && [ -f $MACHINE_GENERATED_FILE ]; then
  echo File $MACHINE_GENERATED_FILE already exists. Delete it manually before running this script.
  exit 1
fi

if [ $OVERWRITE -ne 1 ] && [ -f $CLUSTER_GENERATED_FILE ]; then
  echo File $CLUSTER_GENERATED_FILE already exists. Delete it manually before running this script.
  exit 1
fi

if [ $OVERWRITE -ne 1 ] && [ -f $ADDON_GENERATED_FILE ]; then
  echo File $ADDON_GENERATED_FILE already exists. Delete it manually before running this script.
  exit 1
fi

PACKET_PROJECT_ID="${PACKET_PROJECT_ID:-}"
if [ -z "$PACKET_PROJECT_ID" ]; then
  echo "Must specify the Packet project ID as PACKET_PROJECTID"
  exit 1
fi

mkdir -p ${OUTPUT_DIR}

SSH_KEY=${SSH_KEY:-}
if [ -n "$SSH_KEY" ]; then
  if [ ! -e "$SSH_KEY" ]; then
    echo "ssh key file $SSH_KEY does not exist" >&2
    exit 1
  fi
else
  echo Generate SSH keypair
  ssh-keygen -t rsa -f $SSH_PRIVATE_FILE -C $SSH_USER_PLAIN -N ""
  SSH_KEY=$SSH_PUBLIC_FILE
fi

# By default, linux wraps base64 output every 76 cols, so we use 'tr -d' to remove whitespaces.
# Note 'base64 -w0' doesn't work on Mac OS X, which has different flags.
SSH_PUBLIC=$(cat $SSH_KEY | base64 | tr -d '\r\n')

cat $MACHINE_TEMPLATE_FILE \
  | sed -e "s/\$CLUSTER_NAME/$CLUSTER_NAME/" \
  | sed -e "s/\$FACILITY/$FACILITY/" \
  | sed -e "s/\$PROJECT_ID/$PACKET_PROJECT_ID/" \
  | sed -e "s/\$SSH_KEY/$SSH_PUBLIC/" \
  > $MACHINE_GENERATED_FILE

cat $CLUSTER_TEMPLATE_FILE \
  | sed -e "s/\$CLUSTER_NAME/$CLUSTER_NAME/" \
  | sed -e "s/\$PROJECT_ID/$PACKET_PROJECT_ID/" \
  > $CLUSTER_GENERATED_FILE

cat $ADDON_TEMPLATE_FILE \
  | sed -e "s/\$CLUSTER_NAME/$CLUSTER_NAME/" \
  > $ADDON_GENERATED_FILE

echo -e "\nYour cluster name is '${CLUSTER_NAME}'"
