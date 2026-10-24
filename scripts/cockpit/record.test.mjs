// node --test scripts/cockpit/record.test.mjs
import assert from 'node:assert/strict'
import { execFileSync, spawnSync } from 'node:child_process'
import { mkdirSync, mkdtempSync, readdirSync, readFileSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { test } from 'node:test'
import { fileURLToPath } from 'node:url'

const script = fileURLToPath(new URL('./record.mjs', import.meta.url))

function repo(branch = 'main') {
  const dir = mkdtempSync(join(tmpdir(), 'record-'))
  const git = (...a) => execFileSync('git', a, { cwd: dir, stdio: 'ignore' })
  git('init', '-q', '-b', 'main')
  git('config', 'user.email', 't@example.com')
  git('config', 'user.name', 't')
  mkdirSync(join(dir, 'docs/agents'), { recursive: true })
  writeFileSync(
    join(dir, 'docs/agents/cockpit.json'),
    JSON.stringify({
      repo: 'me/demo',
      integration_required_when_changed: ['internal/streaming/'],
      stop_hook: { max_consecutive_blocks: 2 },
    }),
  )
  writeFileSync(join(dir, '.gitignore'), '/.cockpit/\n')
  git('add', '.')
  git('commit', '-qm', 'init')
  if (branch !== 'main') git('checkout', '-qb', branch)
  return { dir, git }
}

// A stand-in for `task verify`: writes a vitest-shaped report, then exits.
function fakeVerify(dir, { fail = false } = {}) {
  const report = {
    testResults: [
      {
        name: join(dir, 'web/src/a.test.ts'),
        assertionResults: [
          { fullName: 'a works', status: 'passed' },
          { fullName: 'a breaks', status: fail ? 'failed' : 'passed' },
        ],
      },
    ],
  }
  const code = `
    const fs = require('fs'); fs.mkdirSync('.cockpit/last-run', { recursive: true });
    fs.writeFileSync('.cockpit/last-run/vitest.json', ${JSON.stringify(JSON.stringify(report))});
    process.exit(${fail ? 1 : 0});`
  return [process.execPath, '-e', code]
}

function record(dir, cmd, { stopHook = false, session = 's1' } = {}) {
  const args = [script, ...(stopHook ? ['--stop-hook'] : []), '--', ...cmd]
  return spawnSync(process.execPath, args, {
    cwd: dir,
    env: { ...process.env, CLAUDE_PROJECT_DIR: dir },
    input: stopHook ? JSON.stringify({ session_id: session, transcript_path: '/t.jsonl' }) : '',
    encoding: 'utf8',
  })
}

function evidence(dir) {
  let files = []
  try {
    files = readdirSync(join(dir, '.cockpit/evidence'))
  } catch {
    return []
  }
  return files
    .filter((f) => !f.endsWith('.results.json'))
    .sort()
    .map((f) => JSON.parse(readFileSync(join(dir, '.cockpit/evidence', f), 'utf8')))
}

test('manual run: passes the exit code through and records what ran', () => {
  const { dir } = repo('fix/42-empty-page')
  const r = record(dir, fakeVerify(dir))
  assert.equal(r.status, 0, r.stderr)
  const [ev] = evidence(dir)
  assert.equal(ev.schema, 'cockpit.evidence/v1')
  assert.equal(ev.trigger, 'manual')
  assert.equal(ev.result, 'pass')
  assert.equal(ev.repo, 'me/demo')
  assert.equal(ev.git.branch, 'fix/42-empty-page')
  assert.deepEqual(ev.git.issue_refs, [42])
  assert.deepEqual(ev.tests.web, { pass: 2, fail: 0, skip: 0, failed: [] })
  assert.equal(ev.tests.go, null)
  const res = JSON.parse(readFileSync(join(dir, '.cockpit/evidence', `${ev.id}.results.json`), 'utf8'))
  assert.equal(res.results['web:web/src/a.test.ts:a breaks'], 'pass')
})

test('stop hook: red blocks with exit 2 naming the failing test, then lets go after the limit', () => {
  const { dir } = repo()
  const first = record(dir, fakeVerify(dir, { fail: true }), { stopHook: true })
  assert.equal(first.status, 2)
  assert.match(first.stderr, /a breaks/)
  assert.match(first.stderr, /attempt 1\/2/)
  assert.equal(record(dir, fakeVerify(dir, { fail: true }), { stopHook: true }).status, 2)
  const third = record(dir, fakeVerify(dir, { fail: true }), { stopHook: true })
  assert.equal(third.status, 1, 'non-blocking after the limit')
  assert.match(third.stderr, /not blocking any more/)
  assert.deepEqual(
    evidence(dir).map((e) => e.result),
    ['fail', 'fail', 'fail'],
  )
  // Another session starts its own count.
  assert.equal(record(dir, fakeVerify(dir, { fail: true }), { stopHook: true, session: 's2' }).status, 2)
})

test('stop hook: an unchanged green tree is not re-run, a changed one is', () => {
  const { dir } = repo()
  assert.equal(record(dir, fakeVerify(dir), { stopHook: true }).status, 0)
  assert.equal(record(dir, fakeVerify(dir), { stopHook: true }).status, 0)
  assert.equal(evidence(dir).length, 1)
  writeFileSync(join(dir, 'new.txt'), 'x')
  assert.equal(record(dir, fakeVerify(dir), { stopHook: true }).status, 0)
  assert.equal(evidence(dir).length, 2)
})

test('flags integration when the branch touches a policy path', () => {
  const { dir, git } = repo('feat/transcode')
  mkdirSync(join(dir, 'internal/streaming'), { recursive: true })
  writeFileSync(join(dir, 'internal/streaming/x.go'), 'package streaming\n')
  git('add', '.')
  git('commit', '-qm', 'feat: transcode (#7)')
  record(dir, fakeVerify(dir))
  const [ev] = evidence(dir)
  assert.deepEqual(ev.integration, { required: true, matched: ['internal/streaming/x.go'] })
  assert.deepEqual(ev.git.issue_refs, [7])
  assert.equal(ev.git.commits_ahead, 1)
})

test('a command that cannot start is a failure, not a crash', () => {
  const { dir } = repo()
  const r = record(dir, ['definitely-not-a-command-xyz'])
  assert.equal(r.status, 127)
  assert.equal(evidence(dir)[0].result, 'fail')
})
