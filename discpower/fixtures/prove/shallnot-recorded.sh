#!/bin/sh
# Stands for shallnot: `schema report` prints the schema pattern of report 1.2 and
# later; any other command writes the recorded report where `--json-out` points.
if [ "$1" = "schema" ]; then
  printf '%s\n' '{"properties":{"schema_version":{"pattern":"^1\\.([2-9]|[1-9][0-9]+)$"}}}'
  exit 0
fi
out=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--json-out" ]; then
    out="$2"
  fi
  shift
done
cp "$SHALLNOT_RECORDED_REPORT" "$out"
