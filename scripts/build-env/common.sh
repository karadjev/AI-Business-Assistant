#!/bin/sh
set -eu
BUILD_SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
blocked() { echo "BLOCKED: $*" >&2; exit 42; }
# Defense in depth; run only inside the supplied isolated Compose, never on host.
[ "$(uname -s)" = Linux ] || blocked 'requires isolated Linux runner, not host execution'
[ -r /proc/net/route ] || blocked 'cannot inspect routing'
[ -z "$(awk '$2=="00000000" {print}' /proc/net/route)" ] || blocked 'IPv4 default route exists'
if [ -r /proc/net/ipv6_route ]; then
 # Linux exposes an unreachable reject-default on lo even with no IPv6 egress.
 # Permit only this exact kernel reject entry; any usable default is blocked.
 [ -z "$(awk '$1=="00000000000000000000000000000000" && $2=="00" && !($9=="00200200" && $10=="lo") {print}' /proc/net/ipv6_route)" ] || blocked 'IPv6 default route exists'
fi
[ ! -S /var/run/docker.sock ] || blocked 'Docker socket must not be mounted'
unset HTTP_PROXY HTTPS_PROXY ALL_PROXY http_proxy https_proxy all_proxy
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOTELEMETRY=off GOVCS='*:off'
export NEXT_TELEMETRY_DISABLED=1 npm_config_offline=true npm_config_audit=false npm_config_fund=false
ABA_API_DIR=${ABA_API_DIR:-/src}
ABA_WEB_ROOT=${ABA_WEB_ROOT:-/apps/web}
export ABA_API_DIR ABA_WEB_ROOT
