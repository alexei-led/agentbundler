#!/usr/bin/env bash
set -euo pipefail
rules=$1
cat >/dev/null
printf '{"continue":true,"rules":"%s"}\n' "$(basename "$rules")"
