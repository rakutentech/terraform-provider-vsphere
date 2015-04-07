#!/bin/bash
set -xe

VERSION=$(grep "const Version " version.go | sed -E 's/.*"(.+)"$/\1/')
REPO="terraform-provider-vsphere"

DIR=$(cd $(dirname ${0})/.. && pwd)
cd ${DIR}

# Get the parent directory of where this script is.
SOURCE="${BASH_SOURCE[0]}"
while [ -h "$SOURCE" ] ; do SOURCE="$(readlink "$SOURCE")"; done
DIR="$( cd -P "$( dirname "$SOURCE" )/.." && pwd )"

# Change into that dir because we expect that
cd $DIR

go get -u github.com/hashicorp/terraform
(cd $GOPATH/src/github.com/hashicorp/terraform/ && make updatedeps && make dev)
go get -u github.com/vmware/govmomi

gox -os="darwin linux windows" -arch="386 amd64" -output "pkg/{{.OS}}_{{.Arch}}/{{.Dir}}"

if [ -d pkg ];then
    rm -rf ./pkg/dist
fi 

# Package all binary as .zip
mkdir -p ./pkg/dist/${VERSION}
for PLATFORM in $(find ./pkg -mindepth 1 -maxdepth 1 -type d); do
    PLATFORM_NAME=$(basename ${PLATFORM})
    ARCHIVE_NAME=${REPO}_${VERSION}_${PLATFORM_NAME}

    if [ $PLATFORM_NAME = "dist" ]; then
        continue
    fi

    pushd ${PLATFORM}
    zip ${DIR}/pkg/dist/${VERSION}/${ARCHIVE_NAME}.zip ./*
    popd
done

# Generate shasum
pushd ./pkg/dist/${VERSION}
shasum -a256 * > ./${REPO}_${VERSION}_SHA256SUMS
popd

ghr --username rakutentech --token $GITHUB_TOKEN --replace ${VERSION} ./pkg/dist/${VERSION}

exit 0
