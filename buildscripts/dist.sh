#!/bin/bash
set -euxo pipefail
cd "$(git rev-parse --show-toplevel)"

APP=broadcaster

PLATFORM=$1
TAG=$2
source buildscripts/init.sh "${PLATFORM}"

BASENAME="${APP}-${TAG}-${PLATFORM}"

case $PLATFORM in
linux-x86_64|linux-armhf|linux-arm64)
    FILENAME="${BASENAME}.tar.xz"
    TARCMD="tar -Jcf ${FILENAME} ${BASENAME}"
    ;;
*)
    echo "Skipping build on ${PLATFORM}"
    exit 0
    ;;
esac

mkdir build && cd build
mkdir "${BASENAME}"
go build ../server/
go build ../radio/
mv server "${BASENAME}/broadcast-server"
mv radio "${BASENAME}/broadcast-radio"

${TARCMD}

echo "PLATFORM_ARTIFACT|build/${FILENAME}"
