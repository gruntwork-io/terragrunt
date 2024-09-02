#!/bin/bash

FILENAME="/tmp/$1"

if test -f "$FILENAME"; then
  echo "Success"
  rm "$FILENAME"
  exit 0
else
  touch "$FILENAME"
  echo "My own little error"
  exit 1
fi
