#!/usr/bin/env bash
set -u

out_file="$1"

{
	echo "TG_CTX_TF_PATH=${TG_CTX_TF_PATH-<unset>}"
	echo "TG_CTX_COMMAND=${TG_CTX_COMMAND-<unset>}"
	echo "TG_CTX_HOOK_NAME=${TG_CTX_HOOK_NAME-<unset>}"
	echo "TG_CTX_HOOK_TYPE=${TG_CTX_HOOK_TYPE-<unset>}"
	echo "TG_CTX_SOURCE=${TG_CTX_SOURCE-<unset>}"
	echo "TG_CTX_TERRAGRUNT_DIR=${TG_CTX_TERRAGRUNT_DIR-<unset>}"
} >"${out_file}"
