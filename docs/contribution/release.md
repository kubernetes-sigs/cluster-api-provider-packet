# how release works

Prerequisite:

* docker installed
* How drone works
* How [goreleaser](https://goreleaser.com/intro/) works and you have to
  [install](https://goreleaser.com/install/) it to try a release locally.
* multi arch TODO, don't know yet

## Continuous Delivery

We use GitHub Action as continuous integration and continuous delivery pipeline.
Integration means: running tests, checking code quality.
Delivery means: make a release when a new tag get pushed.

GoReleaser is used only when a new tag gets pushed. We also push a new image
every time master changes. I will document this workflow in its own chapter:
"push from master" at the end of this document but at the moment it does not use
GoReleaser.

## goreleaser

GoReleaser is a popular tool to release Go applications and library.

We use GoReleaser for the following features:

* Multi arch build. It uses Go cross compilation feature to build for Darwin and
  Linux (arm and amd)
* Docker to build docker images for Linux container arm and amd.
* Changelog to generate a changelog that will be pushed in the GitHub release
  page with the list of PR part of the release itself.
* Artifact (binaries) will be pushed as part of GitHub release page as well as
  the other YAML file required by cluster-api like: metadata.yaml,
  infrastructure-provider.yaml and so on.

First things you should check if the file `./goreleaser.yaml` in the project
root because it gives you an idea about what it does.

## Directories and general workflow

The release life cycle touches two directories:

1. `./dist` it is a temporary directory used by GoReleaser to bundle all the
   required files and, binaries, changelog and so on. It is in gitignore.
2. `./out` is ignored by git and it gets generated via `make release`. It is a
   bundle that contains generated Kubernetes manifest required by the
   cluster-api such as: metadata.yaml, infrastructure-provider.yaml,
   cluster-template.yaml and so on.

GoReleaser works in this way:

1. it builds binaries in `./dist`
2. moves all the file that has to be added in the release archive and release
   page such as: LICENSE, README, the kubernetes manifests in `./dist`
3. Build docker images
4. Push all the archives and the images to GitHub and to Docker Hub.

## Local workflow

GoReleaser can be used locally, to visualize how a release will look like for
example:

```
$ goreleaser release --rm-dir --skip-publish
```

You can run this command if your git HEAD has a tag. You can make a temporary
one:

```
git tag v0.10.0
```

Or you can decided to run GoRelease in `--snapshot` mode to avoid a fake tag
(but changelog won't work):

```
$ goreleaser release --rm-dir --snapshot --skip-publish
```

*Removing --skip-publish goreleaser will attempt a push to GitHub and to your
docker image repository (docker.io). In this way you can cut a release from your
laptop if needed. But it is not a good idea and we use drone for that. Normally
at Packet only drone.io is capable of pushing to Docker Hub and GitHub.*

We have an utility target that helps you do try a release:

```
make release
```

If you know what you are doing and you have the right access to GitHub and
Docker Hub you can use:

```
make release/publish
```
**THIS IS NOT USUALLY REQUIRED AND YOU SHOULD NOT DO IT!**

## Drone workflow

Drone is capable of triggering a command only when a new tag is pushed.
The command it runs is: `goreleaser release`.

In order to push artifacts and a release page, drone needs to have access to
docker image repository and GitHub via access token.

## push from master

We push images to Docker Hub tagged as the git commit sha and latest every time
master changes (usually when a PR gets merged).

This process at the moment does not use GoReleaser (it will may use it in the
future) and you can check how it works look at the `./.github/workflows/ci.yaml`
file. In practice when master changes an action will build and push the new
images to Docker Hub.

## example of goreleaser output

This is an example of a valid goreleaser output that I ran locally via `make release`

```
goreleaser release --rm-dist --snapshot --skip-publish

   • releasing...
   • loading config file       file=.goreleaser.yml
   • running before hooks
   • loading environment variables
   • getting and validating git state
      • releasing v0.1.0, commit 1f8e0e31d10a3f4f909fbcd9249fb12b14bf0010
      • pipe skipped              error=disabled during snapshot mode
   • parsing tag
   • setting defaults
      • snapshotting
      • github/gitlab/gitea releases
      • project name
      • building binaries
      • creating source archive
      • archives
      • linux packages
      • snapcraft packages
      • calculating checksums
      • signing artifacts
      • docker images
      • artifactory
      • blobs
      • homebrew tap formula
      • scoop manifests
   • snapshotting
   • checking ./dist
      • --rm-dist is set, cleaning it up
   • writing effective config file
      • writing                   config=dist/config.yaml
   • generating changelog
      • pipe skipped              error=not available for snapshots
   • building binaries
      • building                  binary=/Users/gianarb/git/cluster-api-provider-packet/dist/capp_l
inux_arm64/manager
      • building                  binary=/Users/gianarb/git/cluster-api-provider-packet/dist/capp_d
arwin_amd64/manager
      • building                  binary=/Users/gianarb/git/cluster-api-provider-packet/dist/capp_l
inux_amd64/manager
   • archives
      • creating                  archive=dist/cluster-api-provider-packet_v0.1.0-next_Linux_x86_64
.tar.gz
      • creating                  archive=dist/cluster-api-provider-packet_v0.1.0-next_Linux_arm64.
tar.gz
      • creating                  archive=dist/cluster-api-provider-packet_v0.1.0-next_Darwin_x86_6
4.tar.gz
   • creating source archive
      • pipe skipped              error=source pipe is disabled
   • linux packages
   • snapcraft packages
   • calculating checksums
      • checksumming              file=cluster-api-provider-packet_v0.1.0-next_Darwin_x86_64.tar.gz
      • checksumming              file=cluster-api-provider-packet_v0.1.0-next_Linux_arm64.tar.gz
      • checksumming              file=cluster-api-provider-packet_v0.1.0-next_Linux_x86_64.tar.gz
   • signing artifacts
   • docker images
      • building docker image     image=packethost/cluster-api-provider-packet:latest-amd64
      • building docker image     image=packethost/cluster-api-provider-packet:latest-arm64
      • pipe skipped              error=docker.skip_push is set
   • publishing
      • blobs
         • pipe skipped              error=blobs section is not configured
      • http upload
         • pipe skipped              error=uploads section is not configured
      • docker images
         • pipe skipped              error=publishing is disabled
      • snapcraft packages
         • pipe skipped              error=publishing is disabled
      • github/gitlab/gitea releases
         • pipe skipped              error=publishing is disabled
      • homebrew tap formula
         • token type                type=
      • scoop manifests
         • pipe skipped              error=publishing is disabled
   • release succeeded after 8.29s
```
