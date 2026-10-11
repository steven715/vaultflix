#!/usr/bin/env bash
# Tests verify-before-push.sh (task verify runs it): feeds it hook inputs with
# a stubbed `task` that always fails, so exit 2 means the gate ran and exit 0
# that it was skipped. The stub records the directory the gate ran in.
here=$(cd "$(dirname "$0")" && pwd)
hook="$here/verify-before-push.sh"
root=$(cd "$here/../.." && pwd)
stub=$(mktemp -d)
printf '#!/bin/sh\npwd > "%s/ran-in"; echo verify-ran; exit 1\n' "$stub" > "$stub/task"
chmod +x "$stub/task"
fails=0
check() {
  want="$1"; cmd="$2"; path="${3:-$stub:$PATH}"
  payload=$(jq -n --arg c "$cmd" --arg d "$stub" '{tool_name:"Bash",cwd:$d,tool_input:{command:$c}}')
  printf '%s' "$payload" | PATH="$path" CLAUDE_PROJECT_DIR="$root" /bin/bash "$hook" >/dev/null 2>&1
  got=$?
  if [ "$got" = "$want" ]; then r=PASS; else r=FAIL; fails=$((fails+1)); fi
  [ "$r" = PASS ] || printf 'FAIL want=%s got=%s  %s\n' "$want" "$got" "$(printf '%s' "$cmd" | head -1)"
}
# gated
check 2 'git push -u origin feat/x'
check 2 'cd repo && git push -q origin HEAD 2>&1 | tail -2'
check 2 'git  push'
check 2 'git -C /some/repo push origin HEAD'
check 2 'git -c core.sshCommand=ssh --no-pager push'
check 2 '/usr/bin/git push'
check 2 'echo `git push`'
# not gated
check 0 'git status'
check 0 'python3 - <<EOF
print("verify-before-push.sh")
EOF'
check 0 'echo pushed; git pushd-like-name'
check 0 'grep -n "git pushes" README.md'
check 0 'git log --oneline -1'
# jq unavailable: fail closed (gate runs)
nojq=$(mktemp -d); for b in bash sh cat grep printf head pwd tail dirname env; do ln -s "$(command -v $b)" "$nojq/$b" 2>/dev/null; done
check 2 'git status' "$stub:$nojq"
rm -rf "$nojq"
# the gate runs where the push happens
check 2 'git push' >/dev/null
if [ "$(cat "$stub/ran-in")" != "$stub" ]; then echo "FAIL gate ran in $(cat "$stub/ran-in"), not the hook's cwd"; fails=$((fails+1)); fi
rm -rf "$stub"
if [ "$fails" -gt 0 ]; then
  echo "verify-before-push.sh: $fails case(s) failed" >&2
  exit 1
fi
echo "verify-before-push.sh: all cases pass"
