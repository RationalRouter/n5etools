#!/usr/bin/env bash
# Double-click (or run) to produce a fresh Linux build for dev-team
# testing. Combines `make build` + `make release` into one step and drops
# a single n5e-linux.zip directly in dist-release/, overwriting whatever
# was there before. The zip contains the n5e-linux binary (already
# executable — chmod +x is not needed after unzipping) plus assets/n5e.ico,
# since a bare Linux binary has no way to embed an icon the way Windows'
# PE resources do.
#
# See build-windows.sh's comment for why this script lives in
# tools/release/ rather than inside the gitignored dist-release/ itself.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/../.."
make release-linux
echo
echo "Done — dist-release/n5e-linux.zip is ready to send to the dev team."
