#!/usr/bin/env bash
#
# check-import-boundary.sh — assert this module stays self-contained.
#
# hiveD must reference only its own packages (github.com/hived-project/hived/...)
# and third-party dependencies — never a sibling vibeD-project module
# (github.com/vibed-project/*) or mindD/MemorySidecar (module path
# "memsidecar"). That keeps hiveD independently buildable and stops an
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

# 2. No Go source file may import mindD/MemorySidecar directly.
if hits=$(grep -rEn "\"memsidecar/[^\"]+\"" --include='*.go' \
	--exclude-dir=vendor --exclude-dir=.git --exclude-dir=gen .); then
	echo "✖ imports memsidecar (mindD) directly — integrate via a hand-written client against its public proto, not a module import:"
	echo "${hits}"
	fail=1
fi

# 3. go.mod must not require a sibling vibed-project module or memsidecar
#    (the module declaration line itself is excluded).
if reqs=$(grep -E "github.com/vibed-project/|^\s*memsidecar\s" go.mod | grep -v "^module "); then
	echo "✖ go.mod references a sibling github.com/vibed-project module or memsidecar:"
	echo "${reqs}"
	fail=1
fi

if [ "${fail}" -ne 0 ]; then
	cat <<'EOF'

This module must reference only its own packages and third-party
dependencies, not a sibling github.com/vibed-project module or memsidecar
(mindD). See docs/adr/ADR-0004 for the intended integration path.
EOF
	exit 1
fi

echo "✓ boundary intact: no sibling github.com/vibed-project module or memsidecar is referenced"
