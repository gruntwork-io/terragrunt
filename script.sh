#!/bin/bash

export TARGET="${TARGET:-test/experiment_prune_provider_schemas}"

"$TG_TF_PATH" run apply --all --non-interactive --working-dir ${TARGET}