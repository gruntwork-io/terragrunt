#!/usr/bin/env bash
echo 'stdout1'
>&2 echo 'stderr1'
echo 'stdout2'
>&2 echo 'stderr2'
>&2 echo 'stderr3'
