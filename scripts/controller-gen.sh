#!/bin/sh

set -e

# ensure controller-gen is at least the right version


download() {
	local version=$1
	local P=$(pwd)
	local TMP_DIR=$(mktemp -d)
	cd $TMP_DIR
	go mod init tmp
	go get -u sigs.k8s.io/controller-tools/cmd/controller-gen@${version}
	cd ${P}

	# clean up the temporary working directory
	rm -rf $TMP_DIR
}

# see if the first semver argument is >= the second semver argument
later() {
	local compare=$1
	local base=$2
	# strip off the optional "v" at the beginning
	compare=${compare#v}
	base=${base#v}

	local compareMajor=${compare%%.*}
	local baseMajor=${base%%.*}

	local compareMinorPatch=${compare#*.}
	local baseMinorPatch=${base#*.}

	local compareMinor=${compareMinorPatch%%.*}
	local baseMinor=${baseMinorPatch%%.*}

	local comparePatch=${compare##*.}
	local basePatch=${base##*.}
	# check major version

	# start with major - if greater, it is later; if less, it is earlier
	[ $compareMajor -lt $baseMajor ] && return 1
	[ $compareMajor -gt $baseMajor ] && return 0
	
	# major matches, so check minor	
	[ $compareMinor -lt $baseMinor ] && return 1
	[ $compareMinor -gt $baseMinor ] && return 0

	# minor matches, so check patch
	[ $comparePatch -lt $basePatch ] && return 1
	[ $comparePatch -gt $basePatch ] && return 0

	# patch matches, so it is the same
	return 0
}


BINARY=$1
VERSION=$2

# if no version given, just download the latest and go
if [ -z "$VERSION" ]; then
	download master
	exit 0
fi

# check if we have one and what its version is
if [ ! -e "${BINARY}" ]; then
	download ${VERSION}
	exit 0
fi

# if we got here, we have one, and we were not asked to take latest, so check its version
existing=$(${BINARY} --version | awk '{print $2}')
# get the three parts of the semver and the three parts of the requested version, and compare

later "${existing}" "${VERSION}" || download ${VERSION}

exit 0
	

