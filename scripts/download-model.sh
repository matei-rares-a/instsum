#!/bin/sh
set -eu
source_kind= from= destination= sha=
while [ "$#" -gt 0 ]; do
  case "$1" in
    --source) source_kind=$2; shift 2 ;;
    --from) from=$2; shift 2 ;;
    --destination) destination=$2; shift 2 ;;
    --sha256) sha=$2; shift 2 ;;
    *) echo "Unknown argument: $1" >&2; exit 1 ;;
  esac
done
[ -n "$from" ] && [ -n "$destination" ] || { echo "Required: --source hf|local|mirror-url --from PATH --destination PATH [--sha256 HASH]" >&2; exit 1; }
[ ! -e "$destination" ] || { echo "Destination exists: $destination" >&2; exit 1; }
case "$source_kind" in
  local) cp "$from" "$destination" ;;
  hf|mirror-url)
    [ -n "$sha" ] || { echo "Remote downloads require --sha256" >&2; exit 1; }
    if [ "$source_kind" = hf ]; then from="https://huggingface.co/$from"; fi
    case "$from" in https://*) ;; *) echo "HTTPS required" >&2; exit 1 ;; esac
    curl --fail --location --proto '=https' --proto-redir '=https' "$from" --output "$destination"
    ;;
  *) echo "Unsupported --source" >&2; exit 1 ;;
esac
if [ -n "$sha" ]; then
  actual=$(sha256sum "$destination" | cut -d ' ' -f 1)
  [ "$actual" = "$sha" ] || { echo "SHA-256 mismatch; file left for inspection, do not use it" >&2; exit 1; }
fi
echo "Saved $destination; configure models paths in configs/config.yaml"
