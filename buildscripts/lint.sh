#!/bin/bash
set -euxo pipefail
cd "$(git rev-parse --show-toplevel)"
source buildscripts/init.sh "$1"

case $PLATFORM in
linux-x86_64|linux-armhf|linux-arm64)
    cd server
    files=$(gofmt -l .) && [ -z "$files" ]

    cd ../radio
    files=$(gofmt -l .) && [ -z "$files" ]
    ;;
*)
    echo "Skipping build on ${PLATFORM}"
    exit 0
    ;;
esac

