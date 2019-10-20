#!/bin/sh

if ! which golint >/dev/null 2>&1; then
  go get -u golang.org/x/lint/golint
fi
if ! which gosec >/dev/null 2>&1; then
  go get github.com/securego/gosec/cmd/gosec
fi
