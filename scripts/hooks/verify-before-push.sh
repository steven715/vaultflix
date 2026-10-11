#!/usr/bin/env bash
# Claude Code PreToolUse hook (.claude/settings.json): runs the fast gate
# before an agent's `git push`. Exit 2 blocks the push and hands the failure
# back to the agent.
#
# The settings' `if: Bash(git push*)` also lets through commands it cannot
# parse (heredocs, long pipelines), so check the command ourselves: only a
# real `git push` runs the gate.
cmd=$(jq -r '.tool_input.command // ""' 2>/dev/null)
if ! printf '%s\n' "$cmd" | grep -Eq '(^|[;&|(]|[[:space:]])git[[:space:]]+push([[:space:]]|$)'; then
  exit 0
fi
cd "${CLAUDE_PROJECT_DIR:-$(git rev-parse --show-toplevel)}" || exit 2
if ! out=$(task verify 2>&1); then
  printf '%s\n' "$out" | tail -40 >&2
  echo "git push blocked: task verify is red. Fix it, then push again." >&2
  exit 2
fi
