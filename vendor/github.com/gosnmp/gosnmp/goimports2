#!/bin/bash

# remove all blank lines in go 'imports' statements,
# then sort with goimports

if [ $# != 1 ] ; then
  echo "usage: $0 <filename>"
  exit 1
fi

EXE="sed"
if  [[ "$OSTYPE" == "darwin"* ]]; then
  EXE="ssed"
fi
$EXE -i '
  /^import/,/)/ {
    /^$/ d
  }
' $1
goimports -w $1
gofmt -s -w $1
