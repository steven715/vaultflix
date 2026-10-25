#!/usr/bin/env node
// Reads `go test -json ./...` on stdin, keeps every event in a JSONL file for
// the control room, and prints what a plain `go test ./...` would show: one
// line per package, the output of failing tests, and build errors.
//
//   go test -json ./... 2>&1 | node scripts/cockpit/go-test-report.mjs <out.jsonl>
//
// The exit code is this script's verdict, because a pipe hides go's own: 1
// when any package or test failed, a build failed, or no package reported a
// result at all (go itself errored before testing). Fails closed.
import { createWriteStream, mkdirSync } from 'node:fs'
import { dirname } from 'node:path'
import { createInterface } from 'node:readline'

const outPath = process.argv[2]
if (!outPath) {
  console.error('usage: go test -json ./... 2>&1 | node go-test-report.mjs <out.jsonl>')
  process.exit(2)
}
mkdirSync(dirname(outPath), { recursive: true })
const out = createWriteStream(outPath)

const testOutput = new Map() // "pkg\ttest" -> buffered output lines
const packages = new Set()
let failedPackages = 0
let buildFailed = false
const counts = { pass: 0, fail: 0, skip: 0 }

function onEvent(ev) {
  const key = `${ev.Package}\t${ev.Test}`
  switch (ev.Action) {
    case 'build-output':
      process.stdout.write(ev.Output ?? '')
      return
    case 'build-fail':
      buildFailed = true
      return
    case 'output':
      if (ev.Test) {
        if (!testOutput.has(key)) testOutput.set(key, [])
        testOutput.get(key).push(ev.Output ?? '')
      } else if (ev.Output !== 'PASS\n') {
        // Package lines: "ok  pkg 0.1s", "FAIL pkg", "?  pkg [no test files]".
        process.stdout.write(ev.Output ?? '')
      }
      return
    case 'pass':
    case 'fail':
    case 'skip':
      if (ev.Test) {
        counts[ev.Action]++
        if (ev.Action === 'fail') process.stdout.write((testOutput.get(key) ?? []).join(''))
        testOutput.delete(key)
      } else if (ev.Package) {
        packages.add(ev.Package)
        if (ev.Action === 'fail') failedPackages++
      }
      return
    default:
  }
}

const rl = createInterface({ input: process.stdin, crlfDelay: Infinity })
rl.on('line', (line) => {
  let ev
  try {
    ev = JSON.parse(line)
  } catch {
    // Not an event: go's own errors ("go: ...") or stray stderr. Show as is.
    process.stdout.write(`${line}\n`)
    return
  }
  out.write(`${line}\n`)
  onEvent(ev)
})
rl.on('close', () => {
  out.end(() => {
    const failed = failedPackages > 0 || counts.fail > 0 || buildFailed || packages.size === 0
    const verdict = failed ? 'FAIL' : 'ok'
    console.log(
      `go test: ${verdict} — ${counts.pass} passed, ${counts.fail} failed, ${counts.skip} skipped` +
        ` in ${packages.size} packages${packages.size === 0 ? ' (no results: go did not run any package)' : ''}`,
    )
    process.exit(failed ? 1 : 0)
  })
})
