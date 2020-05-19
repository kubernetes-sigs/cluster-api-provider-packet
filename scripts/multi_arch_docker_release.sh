#!/bin/sh
set -e

# This has to be set otherwise the default driver will not work
docker buildx create --use --name build --node build --driver-opt network=host

docker buildx build --push --platform linux/${ARCH} \
    -t packethost/cluster-api-provider-packet:latest-${ARCH} \
    -t packethost/cluster-api-provider-packet:${TAG}-${ARCH} \
    -f ../../Dockerfile.goreleaser .

# Update the manifest for the new release
docker buildx imagetools create \
  -t packethost/cluster-api-provider-packet:${TAG} \
  packethost/cluster-api-provider-packet:${TAG}-${ARCH} \
  packethost/cluster-api-provider-packet:${TAG}-${ARCH}

docker buildx imagetools create \
  -t packethost/cluster-api-provider-packet:latest \
  packethost/cluster-api-provider-packet:latest-${ARCH} \
  packethost/cluster-api-provider-packet:latest-${ARCH}
