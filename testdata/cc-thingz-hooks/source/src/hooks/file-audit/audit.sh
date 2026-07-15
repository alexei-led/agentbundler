#!/usr/bin/env bash
set -euo pipefail
cat >/dev/null
printf '{"continue":true,"eventFile":"%s"}\n' "$(basename "$1")"
