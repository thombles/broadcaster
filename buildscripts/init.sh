#!/bin/bash
set -euxo pipefail
cd "$(git rev-parse --show-toplevel)"

PLATFORM=$1

case $PLATFORM in
mac-x86_64)
    GOOS=darwin
    GOARCH=amd64
    ;;
mac-arm64)
    GOOS=darwin
    GOARCH=arm64
    ;;
linux-x86_64)
    GOOS=linux
    GOARCH=amd64
    ;;
linux-armhf)
    GOOS=linux
    GOARCH=arm
    ;;
linux-arm64)
    GOOS=linux
    GOARCH=arm64
    ;;
windows-x86_64)
    GOOS=windows
    GOARCH=amd64
    ;;
*)
    echo "Unrecognised platform"
    exit 1
    ;;
esac

export PLATFORM
export GOOS
export GOARCH
