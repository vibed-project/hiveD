#!/usr/bin/env bash
#
# check-import-boundary.sh — assert this module stays self-contained.
#
# hiveD must reference only its own packages (github.com/hived-project/hived/...)
# and third-party dependencies — never a sibling vibeD-project module
# (github.com/vibed-project/*), which now includes mindD
# (github.com/vibed-project/mindD). That keeps hiveD independently
# buildable and stops an
# accidental cross-repository dependency from creeping in ahead of the
# real integration seam. See docs/adr/ADR-0004 for why the mindD
# integration goes through a hand-written client against mindD's public
# proto, not a direct import of its module.
#
# Run locally: ./scripts/check-import-boundary.sh
set -euo pipefail

# Move to the repository root regardless of where the script is invoked from.
cd "$(dirname "$0")/.."

SELF="github.com/hived-project/hived"

fail=0

# 1. No Go source file may import a sibling vibed-project module.
if hits=$(grep -rEn "\"github.com/vibed-project/[^\"]+\"" --include='*.go' \
	--exclude-dir=vendor --exclude-dir=.git --exclude-dir=gen . | grep -v "\"${SELF}/" | grep -v "\"${SELF}\""); then
	echo "✖ imports a sibling github.com/vibed-project module (must stay self-contained):"
	echo "${hits}"
	fail=1
fi


# 2. go.mod must not require a sibling vibed-project module (incl. mindD)
#    (the module declaration line itself is excluded).
if reqs=$(grep -E "github.com/vibed-project/" go.mod | grep -v "^module "); then
	echo "✖ go.mod references a sibling github.com/vibed-project module (incl. mindD):"
	echo "${reqs}"
	fail=1
fi

if [ "${fail}" -ne 0 ]; then
	cat <<'EOF'

This module must reference only its own packages and third-party
dependencies, not a sibling github.com/vibed-project module (which
includes mindD). See docs/adr/ADR-0004 for the intended integration path.
EOF
	exit 1
fi

echo "✓ boundary intact: no sibling github.com/vibed-project module is referenced"
