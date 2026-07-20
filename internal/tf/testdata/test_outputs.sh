#!/usr/bin/env bash
echo 'stdout1'
sleep 0.1
>&2 echo 'stderr1'
sleep 0.1
echo 'stdout2'
sleep 0.1
>&2 echo 'stderr2'
sleep 0.1
>&2 echo 'stderr3'
