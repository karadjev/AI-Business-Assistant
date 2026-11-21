#!/bin/sh
. "$(dirname -- "$0")/common.sh"
command -v go >/dev/null || blocked 'Go1.26.11 missing'
[ "$(go env GOVERSION)" = go1.26.11 ] || blocked 'requires approved Go1.26.11'
[ "$(go env GOOS)/$(go env GOARCH)" = linux/amd64 ] || blocked 'prepared runner target is linux/amd64'
CC=${CC:-gcc}; export CC
command -v "$CC" >/dev/null || blocked 'gcc missing; prepare compiler image with gcc and libc headers before offline run'
probe=$(mktemp -d); trap 'rm -rf "$probe"' EXIT HUP INT TERM
printf '#include <stdlib.h>\nint main(void){return EXIT_SUCCESS;}\n' > "$probe/check.c"
"$CC" "$probe/check.c" -o "$probe/check" || blocked 'C headers/linker unavailable'
"$probe/check" || blocked 'C toolchain probe failed'
go version; "$CC" --version | head -1
echo 'PREREQUISITES_ONLY: C toolchain usable; race tests have not run'
