#!/usr/bin/env bash
# Claude Code PreToolUse hook on every Bash call (.claude/settings.json): runs
# the fast gate before an agent's `git push`. Exit 2 blocks the push and hands
# the failure back to the agent.
#
# This script is the only filter: it reads the command from the hook input and
# gates `git push`, also with global options (`git -C dir push`) or a path
# (`/usr/bin/git push`). The text "git push" inside a quoted string or a
# heredoc also matches; that costs one extra (green) verify run, never a skip.
# When the input cannot be read (no jq), it gates anyway.
input=$(cat)
dir=""
if command -v jq >/dev/null 2>&1 && cmd=$(printf '%s' "$input" | jq -er '.tool_input.command' 2>/dev/null); then
  dir=$(printf '%s' "$input" | jq -r '.cwd // ""')
else
  cmd="git push"
fi

push='(^|[^[:alnum:]_.-])git([[:space:]]+(-[Cc][[:space:]]+[^[:space:]]+|--?[[:alnum:]-]+(=[^[:space:]]+)?))*[[:space:]]+push([^[:alnum:]_-]|$)'
printf '%s\n' "$cmd" | grep -Eq "$push" || exit 0

# Verify the tree being pushed: the call's working directory (a worktree,
# say), else the project.
cd "${dir:-${CLAUDE_PROJECT_DIR:-$(git rev-parse --show-toplevel)}}" || exit 2
if ! out=$(task verify 2>&1); then
  printf '%s\n' "$out" | tail -40 >&2
  echo "git push blocked: task verify is red. Fix it, then push again." >&2
  exit 2
fi
