#!/usr/bin/env bash
# Regenerate the Ceaser product demo from an existing, already-completed PR.
#
# This never runs CI: it reuses the failed check and the Ceaser bot comment that
# already exist on the pull request. Point it at any PR whose CI has failed and
# whose Ceaser investigation has finished.
#
#   ./record.sh
#   ./record.sh --pr https://github.com/ceaser-org/ceaser-demo/pull/1
#   ./record.sh --fps 30 --crf-webm 33
#
# Outputs land in out/: ceaser-demo.webm, .mp4, .gif, -poster.webp

set -euo pipefail
cd "$(dirname "$0")"

if [ ! -d node_modules ]; then
  echo "Installing dependencies…"
  npm install --no-audit --no-fund
fi

echo "Capturing…"
node capture.mjs "$@"

echo "Assembling…"
node scripts/assemble.mjs "$@"

echo
echo "Done. Assets in $(pwd)/out:"
ls -lh out
