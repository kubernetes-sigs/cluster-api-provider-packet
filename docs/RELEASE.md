# Release

This document describes how to release the Equinix Metal (formerly Packet) infrastructure provider.

This is _not_ intended for regular users.

This is normally performed by our CI system. However, there are important steps to take first.

## How to Cut a Release

In order to cut a release, you must:

1. Update [packet-ci-actions.yaml](../test/e2e/config/packet-ci-actions.yaml) and [packet-ci.yaml](../test/e2e/config/packet-ci.yaml) to use the new version number for the current and/or new contract version of the packet InfrastructureProvider. (ie. v0.9.1)
1. If this is a new major or minor version - but **not** just a patch change:

   - Update [metadata.yaml](../metadata.yaml) to add it, and map it to the correct cluster-api contract version

   ```yaml
   - major: 0
     minor: 10
     contract: v1beta1
   ```

   - Update [packet-ci-actions.yaml](../test/e2e/config/packet-ci-actions.yaml) and [packet-ci.yaml](../test/e2e/config/packet-ci.yaml) to have a new "next" version number for the latest contract version of the packet InfrastructureProvider (ie. v0.11.99).
   - Update clusterctl-settings.json to have the new "next" version number for the latest contract version of the packet InfrastructureProvider (ie. v0.11.99).

1. Review and update the versions of installed deployments like CPEM and kube-vip inside the templates.
1. Commit the changes.
1. Push out your branch, open a PR and merge the changes
1. Wait for the Continuous Integration github action to finish running
1. Tag the release with `git tag -a vX.Y.z -m "Message"
1. Push out the tag

## How A Release Happens

- GitHub Actions detects a new tag has been pushed
- CI builds docker images for each supported architecture as well as a multi-arch manifest, and tags it with the semver tag of the release, e.g. `v0.4.0`
- CI creates the release in `out/release`, the equivalent of `make release`
- CI copies the artifacts in `out/release/*` to the github releases
