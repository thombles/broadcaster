#!/bin/bash
set -euxo pipefail
cd "$(git rev-parse --show-toplevel)"
source buildscripts/init.sh "$1"

case $PLATFORM in
linux-x86_64|linux-armhf|linux-arm64)
    mkdir build && cd build
    go build ../server/
    go build ../radio/
    ;;
*)
    echo "Skipping build on ${PLATFORM}"
    exit 0
    ;;
esac
