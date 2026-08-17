#!/usr/bin/env bash
# Double-click (or run) to produce a fresh Windows build for dev-team
# testing. Combines `make build` + `make release` into one step and drops
# a single n5e-windows.zip directly in dist-release/, overwriting whatever
# was there before. The exe already has the app icon embedded natively
# (Windows PE resources — see the Makefile's icon target), so nothing else
# needs to ride along in the zip.
#
# Lives in tools/release/ rather than dist-release/ itself: dist-release
# is gitignored (a build-output directory) and `make clean` deletes it
# wholesale, so a script placed inside it would never be committed and
# would delete itself on the next clean. This script's own location
# doesn't matter for the workflow — it always writes its output to
# dist-release/ relative to the repo root, wherever it's run from.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/../.."
make release-windows
echo
echo "Done — dist-release/n5e-windows.zip is ready to send to the dev team."
