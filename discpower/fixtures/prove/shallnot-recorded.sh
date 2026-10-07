#!/bin/sh
# Writes the recorded report where `--json-out` points, as shallnot would.
out=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--json-out" ]; then
    out="$2"
  fi
  shift
done
cp "$SHALLNOT_RECORDED_REPORT" "$out"
