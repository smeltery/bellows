#!/bin/sh
set -eu
[ -z "${GH_TOKEN-}${BELLOWS_REPORT_TOKEN-}" ] || exit 91
[ "$1" != fail ] || exit 17
[ ! -e escaped ] || exit 92
printf 'native check passed: %s\n' "${2-plain}"
