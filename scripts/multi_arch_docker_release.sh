#!/bin/sh
set -e

docker buildx create --use --name build --node build --driver-opt network=host
docker buildx build --push --platform linux/${ARCH} \
    -t packethost/cluster-api-provider-packet:latest-${ARCH} \
    -t packethost/cluster-api-provider-packet:${TAG}-${ARCH} \
    -f ../../Dockerfile.goreleaser .

