#!/bin/bash
#
# Build golang binaries for all major operating systems, and push them to the given tagged release in GitHub
#

function print_usage {
  echo
  echo "Usage: build-and-push-release-assets.sh [OPTIONS]"
  echo
  echo "Build golang binaries for all major operating systems, and push them to the given tagged release in GitHub."
  echo
  echo "Options:"
  echo
  echo -e "  --local-src-path\t\tThe path where the golang src code can be found."
  echo -e "  --local-bin-output-path\tThe path where the binaries should be output to."
  echo -e "  --github-repo-owner\t\tThe owner of the GitHub repo (e.g. gruntwork-io)"
  echo -e "  --github-repo-name\t\tThe name of the GitHub repo (e.g. terragrunt)"
  echo -e "  --git-tag\t\t\tThe git tag for which binaries should be built and pushed. We assume that the code at --local-src-path"
  echo -e "           \t\t\tcorresponds to the git tag."
  echo -e "  --app-name\t\t\tWhat to name the binary for this app (e.g. terragrunt)"
  echo
  echo "Example:"
  echo
  echo "  build-and-push-release-assets.sh \ "
  echo "     --local-src-path \"/home/ubuntu/src\""
  echo "     --local-bin-output-path \"/home/ubuntu/src\""
  echo "     --github-repo-owner \"gruntwork-io\""
  echo "     --github-repo-name \"vaas\""
  echo "     --git-tag \"v0.0.1\""
  echo "     --app-name \"terragrunt\""
}

# Assert that a given binary is installed on this box
function assert_is_installed {
  local readonly name="$1"

  if [[ ! $(command -v ${name}) ]]; then
    echo "ERROR: The binary '$name' is required by this script but is not installed or in the system's PATH."
    exit 1
  fi
}

# Assert that the given command-line arg is non-empty.
function assert_not_empty {
  local readonly arg_name="$1"
  local readonly arg_value="$2"

  if [[ -z "$arg_value" ]]; then
    echo "ERROR: The value for '$arg_name' cannot be empty"
    print_usage
    exit 1
  fi
}

# Build go binaries for all major operating systems
function build_binaries {
    local readonly local_src_path="$1"
    local readonly local_bin_output_path="$2"
    local readonly app_name="$3"

    # build the binaries
    cd "$local_src_path"
    gox -os "darwin linux windows" -arch "386 amd64" -output "$local_bin_output_path/${app_name}_{{.OS}}_{{.Arch}}"
}

# In order to push assets to a GitHub release, we must find the "github tag id" associated with the git tag
function get_github_tag_id {
    local readonly github_oauth_token="$1"
    local readonly git_tag="$2"
    local readonly github_repo_owner="$3"
    local readonly github_repo_name="$4"

    curl --silent --show-error \
         --header "Authorization: token $github_oauth_token" \
         --request GET \
         "https://api.github.com/repos/$github_repo_owner/$github_repo_name/releases" \
         | jq --raw-output ".[] | select(.tag_name==\"$git_tag\").id"
}

function push_assets_to_github {
    local readonly local_bin_output_path="$1"
    local readonly github_oauth_token="$2"
    local readonly github_tag_id="$3"
    local readonly github_repo_owner="$4"
    local readonly github_repo_name="$5"

    # Note that putting "$local_bin_output_path/*" in quotes makes bash expand the * right away, so we deliberately omit quotes
    local filepath=""
    for filepath in $local_bin_output_path/*; do
        # Given a filepath like /a/b/c.txt, return c.txt
        local readonly filename=$(echo "$filepath" | rev | cut -d"/" -f1 | rev)
        curl --header "Authorization: token $github_oauth_token" \
             --header "Content-Type: application/x-executable" \
             --data-binary @"$filepath" \
             --request POST \
             "https://uploads.github.com/repos/$github_repo_owner/$github_repo_name/releases/$github_tag_id/assets?name=$filename"
    done;
}

function assert_env_var_not_empty {
  local readonly var_name="$1"
  local readonly var_value="${!var_name}"

  if [[ -z "$var_value" ]]; then
    echo "ERROR: Required environment $var_name not set."
    exit 1
  fi
}

function build_and_push_release_assets {
  local local_src_path=""
  local local_bin_output_path=""
  local github_repo_owner=""
  local github_repo_name=""
  local git_tag=""
  local app_name=""

  assert_env_var_not_empty "$GITHUB_OAUTH_TOKEN"
  local readonly github_oauth_token="$GITHUB_OAUTH_TOKEN"

  while [[ $# > 0 ]]; do
    local key="$1"

    case "$key" in
      --local-src-path)
        local_src_path="$2"
        shift
        ;;
      --local-bin-output-path)
        local_bin_output_path="$2"
        shift
        ;;
      --github-repo-owner)
        github_repo_owner="$2"
        shift
        ;;
      --github-repo-name)
        github_repo_name="$2"
        shift
        ;;
      --git-tag)
        git_tag="$2"
        shift
        ;;
      --app-name)
        app_name="$2"
        shift
        ;;
      --help)
        print_usage
        exit
        ;;
      *)
        echo "ERROR: Unrecognized argument: $key"
        print_usage
        exit 1
        ;;
    esac

    shift
  done

  assert_is_installed "jq"
  assert_is_installed "go"
  assert_is_installed "gox"

  assert_not_empty "--local-src-path" "$local_src_path"
  assert_not_empty "--local-bin-output-path" "$local_bin_output_path"
  assert_not_empty "--github-repo-owner" "$github_repo_owner"
  assert_not_empty "--github-repo-name" "$github_repo_name"
  assert_not_empty "--git-tag" "$git_tag"
  assert_not_empty "--app-name" "$app_name"

  build_binaries "$local_src_path" "$local_bin_output_path" "$app_name"

  local github_tag_id=""
  github_tag_id=$(get_github_tag_id "$github_oauth_token" "$git_tag" "$github_repo_owner" "$github_repo_name")

  push_assets_to_github "$local_bin_output_path" "$github_oauth_token" "$github_tag_id" "$github_repo_owner" "$github_repo_name"
}

build_and_push_release_assets "$@"