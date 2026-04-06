---
description: Show the development workflow for this project — three scenarios (bug/feature/refactor) with shared backend (verify → review → PR → merge)
---

# Vaultflix Development Workflow

## Quick Reference

Use this at the start of a conversation to confirm the scenario and follow the correct flow.

## Step 1: Confirm Scenario

Ask the user which scenario this conversation is for:

| Scenario | Signal | Entry Point |
|----------|--------|-------------|
| **Bug Fix** | "bug", "壞了", "報錯", "不能用" | Go to Bug Fix Flow |
| **Feature** | "新增", "加功能", "想要" | Go to Feature Flow |
| **Refactor** | "重構", "整理", "code review 說要改" | Go to Refactor Flow |

If unclear, ask: "This conversation will focus on one scenario — is this a bug fix, a new feature, or a refactor?"

## Bug Fix Flow

```
1. Reproduce
   - Use Chrome DevTools MCP to observe the problem
   - Check network requests, console errors, take screenshots
   - Confirm: "I can reproduce the issue, here's what I see"

2. Root Cause Analysis
   - Read relevant source code
   - Identify the exact cause (don't guess)
   - Explain to user: what's wrong and why

3. Fix
   - Make the minimal change that fixes the root cause
   - Don't refactor surrounding code

4. Verify
   → Go to Shared: Verify

5. Extract Convention
   - Derive a positive rule from the bug
   - Add to CLAUDE.md as a forward-looking guideline
   - Not a bug history entry — a design rule

6. Review & PR
   → Go to Shared: Review → PR → Merge
```

## Feature Flow

```
1. Brainstorming (superpowers:brainstorming)
   - Explore current codebase
   - Ask clarifying questions (one at a time)
   - Visual mockups if needed (Visual Companion)
   - Propose 2-3 approaches, recommend one
   - Get user approval

2. Design Spec
   - Write to docs/superpowers/specs/YYYY-MM-DD-<topic>-design.md
   - Self-review: no placeholders, no contradictions
   - User reviews and approves
   - Commit

3. Implementation Plan (superpowers:writing-plans)
   - Write to docs/superpowers/plans/YYYY-MM-DD-<topic>.md
   - Bite-sized tasks with full code + test commands
   - Commit

4. Implementation (superpowers:subagent-driven-development)
   Per task:
   - Dispatch implementer subagent
   - Spec compliance review (subagent)
   - Code quality review (subagent)
   - Mark complete, next task

5. Verify
   → Go to Shared: Verify

6. Review & PR
   → Go to Shared: Review → PR → Merge
```

## Refactor Flow

```
1. Analyze Current State
   - Read the code being refactored
   - Identify what's wrong and why (not just "it's messy")
   - Define success criteria: what does "better" look like?

2. Plan
   - Propose the refactoring approach
   - Ensure behavior doesn't change (no feature changes mixed in)
   - Get user approval

3. Implementation
   - Follow the plan step by step
   - Run tests after each step to confirm no regressions
   - Commit frequently

4. Verify
   → Go to Shared: Verify

5. Review & PR
   → Go to Shared: Review → PR → Merge
```

## Shared: Verify

```
- Tests:     docker compose exec vaultflix-api go test ./... -v
- Build:     docker compose build vaultflix-web
- Deploy:    docker compose up -d --force-recreate <service>
- Browser:   Chrome DevTools MCP
             ├ Navigate to affected pages
             ├ Check network (correct status codes, no repeated requests)
             ├ Check console (zero errors)
             └ Screenshot for confirmation
```

## Shared: Review → PR → Merge

```
1. Local Code Review (superpowers:requesting-code-review)
   - Dispatch code-reviewer subagent with git diff
   - Fix Critical issues immediately
   - Fix Important issues before proceeding

2. Create PR
   - git checkout -b <branch-name>
   - git push -u origin <branch-name>
   - gh pr create --title "..." --body "..."

3. PR Code Review (code-review plugin: /code-review)
   - 5 parallel reviewer agents
   - Confidence scoring (filter < 80)
   - Fix flagged issues, push

4. Merge
   - gh pr merge --merge
   - git checkout main && git pull
```

## Cross-Scenario Guard

If the user starts doing work that belongs to a different scenario mid-conversation:

**Respond with:**
> "This looks like a [bug fix / new feature / refactor] — a different scenario from what we're working on ([current scenario]). I'd recommend opening a new conversation for this so the context stays clean and reviews are more precise. Want to finish the current work first, or switch now?"

Do NOT silently accept the context switch. The quality of code review depends on focused, single-scenario conversations.

## Tool Reference

| Tool | When |
|------|------|
| Chrome DevTools MCP | Browser testing, screenshots, network/console inspection |
| superpowers:brainstorming | Feature: requirements + design |
| superpowers:writing-plans | Feature: implementation planning |
| superpowers:subagent-driven-development | Feature: executing plan task by task |
| superpowers:systematic-debugging | Bug: structured debugging approach |
| superpowers:requesting-code-review | All: local diff review before PR |
| /code-review | All: PR-level multi-agent review |
| gh CLI | All: PR creation and merge |

## Environment

- All dev operations through Docker containers (no local Go/Node)
- Tests: `docker compose exec vaultflix-api go test ./...`
- Frontend build: `docker compose build vaultflix-web`
- API restart: `docker compose up -d --force-recreate vaultflix-api`
