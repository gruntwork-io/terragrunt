#!/usr/bin/env bash
# Flood stdout and stderr at the same time, with no sleeps, so the two
# os/exec copy goroutines (one per stream) overlap and hammer the shared
# output writer concurrently. This is what makes the data race on an
# unsynchronized shared buffer observable.
line="the quick brown fox jumps over the lazy dog 0123456789 0123456789 0123456789"

for i in $(seq 1 2000); do
  echo "stdout $i $line"
done &

for i in $(seq 1 2000); do
  >&2 echo "stderr $i $line"
done &

wait
