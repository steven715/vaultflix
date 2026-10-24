// node --test scripts/cockpit/go-test-report.test.mjs
import assert from 'node:assert/strict'
import { spawnSync } from 'node:child_process'
import { mkdtempSync, readFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { test } from 'node:test'
import { fileURLToPath } from 'node:url'

const script = fileURLToPath(new URL('./go-test-report.mjs', import.meta.url))

function run(lines) {
  const out = join(mkdtempSync(join(tmpdir(), 'gtr-')), 'go-test.jsonl')
  const r = spawnSync(process.execPath, [script, out], {
    input: lines.map((l) => (typeof l === 'string' ? l : JSON.stringify(l))).join('\n') + '\n',
    encoding: 'utf8',
  })
  return { ...r, out, saved: readFileSync(out, 'utf8') }
}

const pkg = 'github.com/steven715/vaultflix/internal/service'
const ev = (Action, extra = {}) => ({ Time: '2026-10-24T21:00:00Z', Action, Package: pkg, ...extra })

test('all green: exit 0, package line shown, PASS noise and passing test output hidden', () => {
  const r = run([
    ev('start'),
    ev('run', { Test: 'TestGetVideoByID_NotFound' }),
    ev('output', { Test: 'TestGetVideoByID_NotFound', Output: '=== RUN   TestGetVideoByID_NotFound\n' }),
    ev('pass', { Test: 'TestGetVideoByID_NotFound', Elapsed: 0 }),
    ev('output', { Output: 'PASS\n' }),
    ev('output', { Output: `ok  \t${pkg}\t0.02s\n` }),
    ev('pass', { Elapsed: 0.02 }),
  ])
  assert.equal(r.status, 0, r.stdout)
  assert.match(r.stdout, /ok  \t.*internal\/service/)
  assert.doesNotMatch(r.stdout, /=== RUN/)
  assert.doesNotMatch(r.stdout, /^PASS$/m)
  assert.match(r.stdout, /go test: ok — 1 passed, 0 failed, 0 skipped in 1 packages/)
  assert.equal(r.saved.trim().split('\n').length, 7)
})

test('a failing test: exit 1 and its output is shown', () => {
  const r = run([
    ev('run', { Test: 'TestX' }),
    ev('output', { Test: 'TestX', Output: '    x_test.go:12: want 1, got 2\n' }),
    ev('output', { Test: 'TestX', Output: '--- FAIL: TestX (0.00s)\n' }),
    ev('fail', { Test: 'TestX' }),
    ev('output', { Output: `FAIL\t${pkg}\t0.01s\n` }),
    ev('fail'),
  ])
  assert.equal(r.status, 1)
  assert.match(r.stdout, /want 1, got 2/)
  assert.match(r.stdout, /1 failed/)
})

test('a build failure: exit 1 and the compiler error is shown', () => {
  const r = run([
    { ImportPath: `${pkg} [${pkg}.test]`, Action: 'build-output', Output: '# pkg\n./a.go:3:2: undefined: foo\n' },
    { ImportPath: `${pkg} [${pkg}.test]`, Action: 'build-fail' },
    ev('output', { Output: `FAIL\t${pkg} [build failed]\n` }),
    ev('fail', { FailedBuild: `${pkg} [${pkg}.test]` }),
  ])
  assert.equal(r.status, 1)
  assert.match(r.stdout, /undefined: foo/)
})

test('no results at all (go errored before testing): exit 1, fail closed', () => {
  const r = run(['go: go.mod requires go >= 1.25 (running go 1.18)'])
  assert.equal(r.status, 1)
  assert.match(r.stdout, /go\.mod requires/)
  assert.match(r.stdout, /no results/)
  assert.equal(r.saved, '')
})

test('skips are counted and do not fail the run', () => {
  const r = run([
    ev('run', { Test: 'TestRepo_Integration' }),
    ev('output', { Test: 'TestRepo_Integration', Output: '    db_test.go:9: no DATABASE_URL\n' }),
    ev('skip', { Test: 'TestRepo_Integration' }),
    ev('output', { Output: `ok  \t${pkg}\t0.01s\n` }),
    ev('pass'),
  ])
  assert.equal(r.status, 0)
  assert.match(r.stdout, /1 skipped/)
  assert.doesNotMatch(r.stdout, /no DATABASE_URL/)
})
