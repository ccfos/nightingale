#!/bin/bash

# run goimports2 script across all go files, excluding the following directories:
#   - mocks

find . -type d -name mocks -prune -o -type f -name '*.go' -exec ./goimports2 '{}' ';'
