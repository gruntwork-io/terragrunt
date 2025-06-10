#!/bin/bash

set -e

# Apple certificate used to validate developer certificates https://www.apple.com/certificateauthority/
readonly APPLE_ROOT_CERTIFICATE="http://certs.apple.com/devidg2.der"

function print_usage {
  echo
  echo "Usage: $0 [OPTIONS] <Path to files used to sign...>"
  echo
  echo -e "  MACOS_CERTIFICATE\t\tMac developer certificate in P12 format, encoded in base64."
  echo -e "  MACOS_CERTIFICATE_PASSWORD\tMac certificate password"
  echo
  echo "Optional Arguments:"
  echo -e "  --macos-skip-root-certificate\t\tSkip importing Apple Root certificate. Useful when running in already configured environment."
  echo -e "  --help\t\t\t\tShow this help text and exit."
  echo
  echo "Examples:"
  echo "  $0 sign.hcl"
}

function main {
  local mac_skip_root_certificate=""
  local assets=()

  while [[ $# -gt 0 ]]; do
    local key="$1"
    case "$key" in
      --macos-skip-root-certificate)
        mac_skip_root_certificate=true
        shift
        ;;
      --help)
        print_usage
        exit
        ;;
      -* )
        echo "ERROR: Unrecognized argument: $key"
        print_usage
        exit 1
        ;;
      * )
        assets=("$@")
        break
    esac
  done
  ensure_macos
  import_certificate_mac "${mac_skip_root_certificate}"
  sign_mac "${assets[@]}"
}

function ensure_macos {
  if [[ $OSTYPE != 'darwin'* ]]; then
    echo -e "Signing of Mac binaries is supported only on MacOS"
    exit 1
  fi
}

function sign_mac {
  local -r assets=("$@")
  local gon_cmd="gon"
  for filepath in "${assets[@]}"; do
    echo "Signing ${filepath}"
    "${gon_cmd}" -log-level=info "${filepath}"
  done
}

function import_certificate_mac {
  local -r mac_skip_root_certificate="$1"
  assert_env_var_not_empty "MACOS_CERTIFICATE"
  assert_env_var_not_empty "MACOS_CERTIFICATE_PASSWORD"

  trap "rm -rf /tmp/*-keychain" EXIT

  local mac_certificate_pwd="${MACOS_CERTIFICATE_PASSWORD}"
  local keystore_pw="${RANDOM}"

  # create separated keychain file to store certificate and do quick cleanup of sensitive data
  local db_file
  db_file=$(mktemp "/tmp/XXXXXX-keychain")
  rm -rf "${db_file}"
  echo "Creating separated keychain for certificate"
  security create-keychain -p "${keystore_pw}" "${db_file}"
  security default-keychain -s "${db_file}"
  security unlock-keychain -p "${keystore_pw}" "${db_file}"
  echo "${MACOS_CERTIFICATE}" | base64 -d | security import /dev/stdin -f pkcs12 -k "${db_file}" -P "${mac_certificate_pwd}" -T /usr/bin/codesign
  if [[ "${mac_skip_root_certificate}" == "" ]]; then
    # download apple root certificate used as root for developer certificate
    curl -v "${APPLE_ROOT_CERTIFICATE}" --output certificate.der
    sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain certificate.der
  fi
  security set-key-partition-list -S apple-tool:,apple:,codesign: -s -k "${keystore_pw}" "${db_file}"
}

function assert_env_var_not_empty {
  local -r var_name="$1"
  local -r var_value="${!var_name}"

  if [[ -z "$var_value" ]]; then
    echo "ERROR: Required environment $var_name not set."
    exit 1
  fi
}

main "$@"

