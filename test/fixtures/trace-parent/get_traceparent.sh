#!/usr/bin/env bash
set -e

echo "$1 {\"traceparent\": \"${TRACEPARENT}\"}"
