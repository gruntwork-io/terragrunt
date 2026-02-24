#!/bin/sh
echo 'stdout1'
sleep 1
>&2 echo 'stderr1'
sleep 1
echo 'stdout2'
sleep 1
>&2 echo 'stderr2'
sleep 1
>&2 echo 'stderr3'
