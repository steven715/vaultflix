#!/usr/bin/env node
// Runs a command and leaves an evidence pack in .cockpit/evidence/ for the
// control room: what ran, on which tree, its exit code, and the per-test
// results the run produced. An agent's "done" is only as good as this file.
//
//   node scripts/cockpit/record.mjs -- task test-integration      (manual)
//   node scripts/cockpit/record.mjs --stop-hook -- task verify    (Stop hook)
//
// Stop hook mode (.claude/settings.json) reads the hook input on stdin and
// answers in Claude Code's terms: exit 0 lets the session stop, exit 2 sends
// Claude back to work with stderr as the reason. Any other non-zero exit is
// non-blocking, so a plain `task verify` hook never actually blocked. Two
// guards keep it usable:
//   - an unchanged tree that already has green evidence is not re-run;
//   - after `max_consecutive_blocks` red runs in one session it stops
//     blocking (exit 1: shown to you, the session may stop) and the evidence
//     stays red for the control room to show.
// After each run, packs that finished over RETENTION_DAYS ago are deleted.
// Formats: cockpit.evidence/v1 and cockpit.results/v1, owned by the control room.
import { execFileSync, spawn } from 'node:child_process'
import { createHash } from 'node:crypto'
import { existsSync, mkdirSync, readFileSync, readdirSync, rmSync, statSync, writeFileSync } from 'node:fs'
import { join, relative } from 'node:path'

const TAIL_LINES = 40
// The control room's ADR-0008: this repo keeps 30 days of evidence packs and
// the control room reports a gap if a snapshot missed the ones pruned.
const RETENTION_DAYS = 30

function parseArgs(argv) {
  const sep = argv.indexOf('--')
  const flags = sep === -1 ? [] : argv.slice(0, sep)
  const command = sep === -1 ? argv : argv.slice(sep + 1)
  return { stopHook: flags.includes('--stop-hook'), command }
}

function gitOut(root, args, { trim = true } = {}) {
  try {
    const out = execFileSync('git', args, {
      cwd: root,
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'ignore'],
      maxBuffer: 512 * 1024 * 1024,
    })
    return trim ? out.trim() : out
  } catch {
    return null
  }
}

function findRoot() {
  return process.env.CLAUDE_PROJECT_DIR || gitOut(process.cwd(), ['rev-parse', '--show-toplevel']) || process.cwd()
}

function readJSON(path, fallback = null) {
  try {
    return JSON.parse(readFileSync(path, 'utf8'))
  } catch {
    return fallback
  }
}

function lines(s) {
  return s ? s.split('\n').filter(Boolean) : []
}

function gitState(root) {
  const sha = gitOut(root, ['rev-parse', 'HEAD'])
  const branch = gitOut(root, ['rev-parse', '--abbrev-ref', 'HEAD'])
  const base =
    gitOut(root, ['merge-base', 'HEAD', 'origin/main']) || gitOut(root, ['merge-base', 'HEAD', 'main']) || sha
  const untracked = lines(gitOut(root, ['ls-files', '--others', '--exclude-standard']))
  const dirtyTracked = lines(gitOut(root, ['diff', '--name-only', 'HEAD']))
  const sinceBase = base ? lines(gitOut(root, ['diff', '--name-only', base])) : dirtyTracked
  const changed = [...new Set([...sinceBase, ...untracked])].sort()

  // Fingerprint of the exact tree under test: HEAD + uncommitted diff + untracked contents.
  const h = createHash('sha256')
  h.update(`${sha}\n`)
  h.update(gitOut(root, ['diff', 'HEAD', '--binary'], { trim: false }) ?? '')
  for (const f of untracked) {
    h.update(`\0${f}\0`)
    try {
      h.update(readFileSync(join(root, f)))
    } catch {
      h.update('<unreadable>')
    }
  }

  const refs = new Set()
  const fromBranch = branch?.match(/(?:^|\/)(\d+)(?:-|$)/)
  if (fromBranch) refs.add(Number(fromBranch[1]))
  const log = base && sha && base !== sha ? gitOut(root, ['log', `${base}..HEAD`, '--format=%B']) : ''
  for (const m of (log ?? '').matchAll(/#(\d+)/g)) refs.add(Number(m[1]))

  return {
    sha,
    branch,
    base,
    commits_ahead: base && sha ? Number(gitOut(root, ['rev-list', '--count', `${base}..HEAD`]) ?? 0) : 0,
    dirty: dirtyTracked.length + untracked.length > 0,
    changed_files: changed,
    fingerprint: h.digest('hex'),
    issue_refs: [...refs].sort((a, b) => a - b),
  }
}

function readHookInput() {
  try {
    return JSON.parse(readFileSync(0, 'utf8'))
  } catch {
    return {}
  }
}

function listEvidence(dir) {
  if (!existsSync(dir)) return []
  return readdirSync(dir)
    .filter((f) => f.endsWith('.json') && !f.endsWith('.results.json'))
    .map((f) => readJSON(join(dir, f)))
    .filter(Boolean)
    .sort((a, b) => String(a.finished_at).localeCompare(String(b.finished_at)))
}

// Deletes packs whose own finished_at is older than RETENTION_DAYS, with their
// results. A pack that cannot be read or dated stays, and so does keepId.
// Results go first, so a failed delete never leaves results without a pack.
function pruneEvidence(dir, keepId, now) {
  const cutoff = now - RETENTION_DAYS * 24 * 60 * 60 * 1000
  for (const f of readdirSync(dir)) {
    if (!f.endsWith('.json') || f.endsWith('.results.json')) continue
    const id = f.slice(0, -'.json'.length)
    if (id === keepId) continue
    const finished = Date.parse(readJSON(join(dir, f))?.finished_at)
    if (!(finished < cutoff)) continue // NaN (no date) is not old
    rmSync(join(dir, `${id}.results.json`), { force: true })
    rmSync(join(dir, f))
  }
}

function runCommand(root, command, { tee }) {
  return new Promise((resolve) => {
    const tail = []
    const keep = (chunk, stream) => {
      if (tee) stream.write(chunk)
      for (const l of chunk.toString().split('\n')) {
        tail.push(l)
        if (tail.length > TAIL_LINES * 4) tail.splice(0, tail.length - TAIL_LINES * 2)
      }
    }
    const child = spawn(command[0], command.slice(1), {
      cwd: root,
      env: process.env,
      shell: process.platform === 'win32',
      stdio: ['ignore', 'pipe', 'pipe'],
    })
    child.stdout.on('data', (c) => keep(c, process.stdout))
    child.stderr.on('data', (c) => keep(c, process.stderr))
    child.on('error', (err) => {
      tail.push(`record.mjs: could not start ${command[0]}: ${err.message}`)
      resolve({ code: 127, tail })
    })
    child.on('close', (code, signal) => resolve({ code: code ?? (signal ? 128 : 1), tail }))
  })
}

// --- per-test results ----------------------------------------------------

function freshFile(path, since) {
  try {
    return statSync(path).mtimeMs >= since - 1000 ? path : null
  } catch {
    return null
  }
}

function goResults(path, results) {
  const final = new Map()
  for (const line of lines(readFileSync(path, 'utf8'))) {
    let ev
    try {
      ev = JSON.parse(line)
    } catch {
      continue
    }
    if (ev.Test && ['pass', 'fail', 'skip'].includes(ev.Action)) final.set(`go:${ev.Package}:${ev.Test}`, ev.Action)
  }
  return summarize(final, results)
}

function webResults(path, root, results) {
  const report = readJSON(path, { testResults: [] })
  const map = { passed: 'pass', failed: 'fail' }
  const final = new Map()
  for (const file of report.testResults ?? []) {
    const rel = relative(root, file.name ?? '')
    for (const a of file.assertionResults ?? []) final.set(`web:${rel}:${a.fullName}`, map[a.status] ?? 'skip')
  }
  return summarize(final, results)
}

function summarize(final, results) {
  const s = { pass: 0, fail: 0, skip: 0, failed: [] }
  for (const [key, status] of final) {
    s[status]++
    results[key] = status
    if (status === 'fail' && s.failed.length < 20) s.failed.push(key)
  }
  return s
}

// --- main ----------------------------------------------------------------

function integrationNeed(changed, policy) {
  const patterns = policy?.integration_required_when_changed ?? []
  const matched = changed.filter((f) => patterns.some((p) => f.startsWith(p)))
  return { required: matched.length > 0, matched: matched.slice(0, 20) }
}

function evidenceId(sha) {
  const ts = new Date().toISOString().replace(/[-:.]/g, '')
  return `${ts}-${(sha ?? 'nosha').slice(0, 7)}`
}

function blockMessage(ev, attempt, max) {
  const failed = [...(ev.tests.go?.failed ?? []), ...(ev.tests.web?.failed ?? [])]
  return [
    `\`${ev.command}\` is red (attempt ${attempt}/${max}, evidence .cockpit/evidence/${ev.id}.json).`,
    failed.length ? `Failing tests:\n  ${failed.join('\n  ')}` : 'Last output:',
    failed.length ? '' : ev.output_tail,
    'Fix it before reporting the work as done.',
  ]
    .filter(Boolean)
    .join('\n')
}

async function main() {
  const { stopHook, command } = parseArgs(process.argv.slice(2))
  if (command.length === 0) {
    console.error('usage: record.mjs [--stop-hook] -- <command> [args...]')
    return 1
  }
  const hook = stopHook ? readHookInput() : {}
  const root = findRoot()
  const policy = readJSON(join(root, 'docs/agents/cockpit.json'), {})
  const evidenceDir = join(root, '.cockpit/evidence')
  const commandText = command.join(' ')

  const before = gitState(root)
  if (stopHook) {
    const green = listEvidence(evidenceDir).some(
      (e) => e.result === 'pass' && e.command === commandText && e.git?.fingerprint === before.fingerprint,
    )
    if (green) return 0 // this exact tree already has green evidence
  }

  const startedMs = Date.now()
  const run = await runCommand(root, command, { tee: !stopHook })
  const finishedMs = Date.now()

  const results = {}
  const reportDir = join(root, '.cockpit/last-run')
  const goFile = freshFile(join(reportDir, 'go-test.jsonl'), startedMs)
  const webFile = freshFile(join(reportDir, 'vitest.json'), startedMs)
  const ev = {
    schema: 'cockpit.evidence/v1',
    id: evidenceId(before.sha),
    repo: policy.repo ?? null,
    trigger: stopHook ? 'stop-hook' : 'manual',
    command: commandText,
    exit_code: run.code,
    result: run.code === 0 ? 'pass' : 'fail',
    started_at: new Date(startedMs).toISOString(),
    finished_at: new Date(finishedMs).toISOString(),
    duration_ms: finishedMs - startedMs,
    git: { ...before, changed_files: undefined, changed_files_count: before.changed_files.length },
    tests: {
      go: goFile ? goResults(goFile, results) : null,
      web: webFile ? webResults(webFile, root, results) : null,
    },
    integration: integrationNeed(before.changed_files, policy),
    session: stopHook
      ? {
          id: hook.session_id ?? null,
          transcript_path: hook.transcript_path ?? null,
          stop_hook_active: hook.stop_hook_active ?? null,
        }
      : null,
    output_tail: run.tail.slice(-TAIL_LINES).join('\n'),
  }

  try {
    mkdirSync(evidenceDir, { recursive: true })
    writeFileSync(join(evidenceDir, `${ev.id}.json`), `${JSON.stringify(ev, null, 2)}\n`)
    if (Object.keys(results).length > 0) {
      const res = { schema: 'cockpit.results/v1', evidence_id: ev.id, results }
      writeFileSync(join(evidenceDir, `${ev.id}.results.json`), `${JSON.stringify(res)}\n`)
    }
  } catch (err) {
    console.error(`record.mjs: could not write evidence: ${err.message}`)
  }
  try {
    pruneEvidence(evidenceDir, ev.id, Date.now())
  } catch (err) {
    console.error(`record.mjs: could not prune old evidence: ${err.message}`)
  }

  if (!stopHook) return run.code
  if (run.code === 0) return 0

  const max = policy.stop_hook?.max_consecutive_blocks ?? 3
  let attempt = 0
  for (const e of listEvidence(evidenceDir).reverse()) {
    if (e.trigger !== 'stop-hook' || e.session?.id !== ev.session.id) continue
    if (e.result !== 'fail') break
    attempt++
  }
  if (attempt <= max) {
    console.error(blockMessage(ev, attempt, max))
    return 2
  }
  console.error(
    `\`${commandText}\` is still red after ${max} attempts in this session; not blocking any more.` +
      ` Evidence ${ev.id} records the run as failed.`,
  )
  return 1
}

main().then(
  (code) => process.exit(code),
  (err) => {
    console.error(`record.mjs: ${err.stack ?? err}`)
    process.exit(1)
  },
)
