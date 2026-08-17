#!/usr/bin/env bash
# Double-click (or run) to produce a fresh macOS build for dev-team
# testing. Combines `make build` + `make release` into one step and drops
# a single n5e-macos.zip directly in dist-release/, overwriting whatever
# was there before. The zip contains both n5e-macos-arm64 (Apple Silicon)
# and n5e-macos-amd64 (Intel) — already executable, chmod +x is not needed
# after unzipping — plus assets/n5e.ico, since a bare macOS binary has no
# way to embed an icon short of a full .app bundle (overkill for dev
# testing). No lipo/universal-binary support when cross-compiling from
# Linux, so testers pick whichever binary matches their Mac.
#
# See build-windows.sh's comment for why this script lives in
# tools/release/ rather than inside the gitignored dist-release/ itself.
#
# Note for testers: an unsigned binary like this will likely be blocked by
# Gatekeeper on first run ("cannot be opened because the developer cannot
# be verified"). Right-click -> Open (instead of double-click) the first
# time works around this without needing a paid Apple Developer signing
# certificate.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/../.."
make release-macos
echo
echo "Done — dist-release/n5e-macos.zip is ready to send to the dev team."
