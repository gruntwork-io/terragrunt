#!/bin/bash

set -e

bundle exec jekyll build
echo -e "\e[1;31mRun Jekyll serve to watch for changes"
bundle exec jekyll serve --livereload --drafts --host 0.0.0.0
