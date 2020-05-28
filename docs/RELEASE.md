# Release

This document describes how to release the Packet infrastructure provider.

This is _not_ intended for regular users.

This is normally performed by our CI system. However, there are important steps to take first.

## How to Cut a Releas

In order to cut a release, you must:

1. If this is a new major or minor version - but **not** just a patch change - update [metadata.yaml](./metadata.yaml) to add it, and map it to the correct cluster-api contract version
1. Modify `VERSION` so it includes only the version number to be released, in the format `vA.B.C`
1. Run `make release-version` to ensure other files are up to date.
1. Commit the changes.
1. Push out your branch, open a PR and merge the changes
1. Tag the release with `git tag -a vX.Y.z -m "Message"
1. Push out the tag

The `make release-version` command modifies any dependent files that need to be updated for release to work. If they are not updated, lots of things fail.

It updates the following files based on your git tag:

* [config/release/kustomization.yaml](config/release/kustomization.yaml)

Dependent files **must** be changed, committed and merged _before_ pushing out the tag.

When building the manifests via `make manifests`, `make managerless`, or `make release`, it determines the version as follows:

* Take the version from the contents of [VERSION](./VERSION)
* If there are any uncommitted files, append `-dirty`

To see what version you would get, run `make semver`.

We are aware that this is a somewhat duplicative process, but there is no other way, as multiple stages
depend on having the versions in checked-in files and do not support environment variable or command-line
options.

In the next stage, we will eliminate the need for a git tag, and use CI to apply the tag.

## How A Release Happens

When the VERSION file changes:

* CI creates the release in `out/`, the equivalent of `make release-manifests`
* CI copies the artifacts in `out/release/infrastructure-packet/<version>/*yaml` to the github releases
* CI builds docker images for each supported architecture as well as a multi-arch manifest, and tags it with:
  * the git hash of the commit
  * `master`
  * the semver tag of the release, e.g. `v0.3.1`
  * the tag `latest`

