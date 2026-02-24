#!/usr/bin/env bash
# Mock auth provider that uses file-based coordination to verify parallel execution

set -e

# Get the directory where this script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LOCK_DIR="${SCRIPT_DIR}/.auth-locks"
mkdir -p "$LOCK_DIR"

# Get a unique ID for this invocation
# Use POSIX-compatible timestamp (seconds) + PID + RANDOM to ensure uniqueness
# This works on Linux, macOS, and BSD without requiring nanosecond precision
INVOCATION_ID="auth-$$-$(date +%s)-$RANDOM"

# Create a lock file to indicate we've started
# This acts as a synchronization point - by the time the file is created,
# the parent process should be ready to capture our stderr output.
touch "${LOCK_DIR}/start-${INVOCATION_ID}"

# Log to stderr so it shows up in terragrunt output
# Note: Output after lock file creation to avoid macOS stderr buffering race condition
echo "Auth start ${INVOCATION_ID}" >&2

# Wait for other auth commands to also start (up to 500ms)
# This ensures we test the parallel execution scenario
WAIT_COUNT=0
MAX_WAIT=50  # 50 * 10ms = 500ms max wait

while [ $WAIT_COUNT -lt $MAX_WAIT ]; do
    # Count how many auth commands have started
    STARTED=$(ls -1 "${LOCK_DIR}"/start-* 2>/dev/null | wc -l | tr -d ' \t')

    # If we see at least 2 others started (3 total), we know it's parallel
    if [ "$STARTED" -ge 2 ]; then
        echo "Auth concurrent ${INVOCATION_ID} detected=$STARTED" >&2
        break
    fi

    # Sleep a bit and check again
    sleep 0.01
    WAIT_COUNT=$((WAIT_COUNT + 1))
done

# Simulate some auth work
sleep 0.1

# Return fake credentials as JSON
cat <<EOF
{
  "envs": {
    "AWS_ACCESS_KEY_ID": "fake-access-key",
    "AWS_SECRET_ACCESS_KEY": "fake-secret-key",
    "AWS_SESSION_TOKEN": "fake-session-token"
  }
}
EOF

# Create completion marker
touch "${LOCK_DIR}/end-${INVOCATION_ID}"

echo "Auth end ${INVOCATION_ID}" >&2
