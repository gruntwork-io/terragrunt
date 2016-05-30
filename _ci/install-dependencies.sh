#!/bin/bash
# This script is meant to be run in a Circle CI job to install and configure dependencies.

set -e

echo "Creating symlinks to get our repo into the CircleCI GOPATH"
mkdir -p "$REPO"
rm -f "$REPO"
ln -s "/home/ubuntu/${CIRCLE_PROJECT_REPONAME}" "${REPO}"

readonly GLIDE_VERSION=0.10.2
if [[ ! -d ~/glide ]]; then
  echo "Installing Glide"
  wget "https://github.com/Masterminds/glide/releases/download/$GLIDE_VERSION/glide-$GLIDE_VERSION-linux-amd64.zip"
  unzip "glide-$GLIDE_VERSION-linux-amd64.zip" -d ~/glide
fi

echo "Installing gox"
go get github.com/mitchellh/gox

echo "Installing dependencies using Glide"
cd "$REPO"
glide install

