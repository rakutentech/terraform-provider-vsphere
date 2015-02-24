#!/bin/bash
set -xe

NAME="terraform-provider-vsphere"
VERSION=$1
if [ -z $VERSION ]; then
    echo "Please specify a version."
    exit 1
fi

# Get the parent directory of where this script is.
SOURCE="${BASH_SOURCE[0]}"
while [ -h "$SOURCE" ] ; do SOURCE="$(readlink "$SOURCE")"; done
DIR="$( cd -P "$( dirname "$SOURCE" )/.." && pwd )"

# Change into that dir because we expect that
cd $DIR

go get -u github.com/hashicorp/terraform
(cd $GOPATH/src/github.com/hashicorp/terraform/ && make updatedeps && make dev)
go get -u github.com/vmware/govmomi

gox -os="darwin linux windows" -arch="386 amd64" -output "pkg/{{.OS}}_{{.Arch}}"

rm -rf ./pkg/dist
mkdir -p ./pkg/dist

for FILENAME in $(find ./pkg -mindepth 1 -maxdepth 1 -type f); do
    FILENAME=$(basename $FILENAME)
    mv ./pkg/${FILENAME} ./pkg/dist/${NAME}_${VERSION}_${FILENAME}
done

pushd ./pkg/dist
shasum -a256 * > ./${NAME}_${VERSION}_SHA256SUMS
popd

for ARCHIVE in ./pkg/dist/*; do
    ARCHIVE_NAME=$(basename ${ARCHIVE} .exe)
    pushd ./pkg/dist
    zip ${ARCHIVE_NAME}.zip ${ARCHIVE_NAME}*
    popd
done

mkdir -p ./pkg/dist/${VERSION}
mv ./pkg/dist/*.zip ./pkg/dist/${VERSION}/
ghr --username rakutentech --token $GITHUB_TOKEN --replace ${VERSION} ./pkg/dist/${VERSION}

exit 0
