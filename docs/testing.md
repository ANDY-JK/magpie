# Testing and acceptance

One page for what decides a change: the commands, their knobs, and the
checks whose numbers are budgets. Each is owned by the file named beside it;
this page keeps them together so a run is reproducible without reading the
Makefile, the CI workflow and the test files.

## Go

| What | Command | Notes |
| --- | --- | --- |
| Everything | `make test` | `TAGS=production` (Termux: `nogui`), then `go vet` and `go test -timeout 30m ./...` |
| One package | `go test ./internal/<pkg> -run '^TestName$' -count=1` | `-tags production` for the release build's code |
| Race, gateway | `go test -race ./internal/gateway/` | CI runs it; hours on Windows |
| Windows | `./dev-windows.sh test` | Compiles everything, then the 7 native groups (see the script) |
| Benchmarks | `go test -tags production -run '^$' -bench . -benchtime=2s ./internal/gateway` | `BenchmarkGroupOverhead`, `BenchmarkGroupOverheadParallel`, `BenchmarkImageRequest`, `BenchmarkPreviousResponseTurn`, `BenchmarkLargeRequest`, `BenchmarkDecodeReaders`, `BenchmarkReasoningTiming`; baselines are recorded in each bench file's header |

`go vet` covers the whole tree and fails on the first package; `gofmt -l
internal` must print nothing.

## The browser suite

`internal/gui/tests/*.test.cjs`, run by Node's test runner with Playwright.

| Variable | Default | What it does |
| --- | --- | --- |
| `BROWSER` | both | `chromium` or `webkit` to run one engine |
| `UI_TEST_CONCURRENCY` | 2 | Files are independent; 1 diagnoses a flaky one |
| `UI_TEST_SHARD` | all | `node --test --test-shard` slice (CI's nightly splits 1/6) |
| `ARTIFACT_DIR` | none | Screenshots of a failing page are written there |

`make test-ui` is the whole suite; a single file is
`node --test internal/gui/tests/<name>.test.cjs`. The tests run against the
real pages on `magpie.test` with the API faked in the test (no backend).

## A live gateway

`scripts/gateway-smoke.cjs` sends one minimal request on each API the
gateway serves — Chat, Responses, Anthropic, Gemini and Ollama — to a
running magpie, and exits 1 naming whichever failed:

```
MAGPIE_URL=http://127.0.0.1:3425 node scripts/gateway-smoke.cjs
MAGPIE_URL=... MAGPIE_KEY=sk-magpie-... MAGPIE_MODEL=relay/m1 node scripts/gateway-smoke.cjs
```

`MAGPIE_KEY` defaults to the local token `magpie` (a gateway not shared on
the network takes it); `MAGPIE_MODEL` defaults to the first model
`/v1/models` lists; `MAGPIE_SMOKE_TIMEOUT_MS` bounds each request. It reads
`/api/hello` and `/readyz` first and changes nothing about the gateway.

## Watching a fault

`MAGPIE_FAULT` makes a provider answer as a broken vendor, so a group's
fallback, a group's patience or a rest cap can be watched under the fault it
was set for (`internal/gateway/fault.go`). It is env-only and off unless
set; every injected answer carries `X-Magpie-Fault` and the try's error says
"fault injected", so a Routing trace and the recent calls show it:

```
MAGPIE_FAULT="relay=500,openrouter=delay:2000,gemini=cut" magpie serve
```

`<provider id>=500` (or `status:429`), `=delay:<ms>`, `=cut` (an answer that
stops half-way); `*=` matches any provider; the first rule that matches is
the one used.

## Budgets

| Budget | Value | Owner |
| --- | --- | --- |
| `appScriptBudget` | 1,373,995 bytes | `internal/gui/core_budget_test.go`; its history is `docs/gui-script-budget.md` |
| `coreScriptBudget` | 1,591,679 bytes | same |
| i18n keys | every word a page says has a key in every language | `internal/gui/tests/i18n-keys.test.cjs` |

A change that needs more of a budget raises it in the same commit and says
why in `docs/gui-script-budget.md`.

## CI

`.github/workflows/test.yml`: the Go job runs `go test -timeout 30m` with
the release tags; the race job builds the gateway binary with `-race` and
runs the redact and provider race suites; the UI job runs the browser
suite per shard; the Windows job compiles every test and runs the seven
native groups, then `dev-windows.sh` the same. The nightly shards the
whole browser suite.

## What a change runs

At least: `go build ./...`, `go vet ./internal/<touched>`, `gofmt -l
internal`, the tests of each package it changes (with `-tags production`
where the code has build tags), and for a page `node --test` on its test
file. A change to a subsystem's documented behaviour updates its page under
`docs/subsystems/` in the same commit.
