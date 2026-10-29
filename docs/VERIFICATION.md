# Verification

## Tested versions and builds

- CLIProxyAPI v8.0.47, commit `90c447d3a8727d115aa7552ec93ce023736ed492`.
- Cockpit Tools workflow reference: `2cfe5e51314bfc0bf70bdda1ba08aed82c7dc8bf`; application implementation was not copied.
- Linux amd64: Go 1.26.1, Debian Bookworm/glibc, Docker base manifest pinned in `scripts/Dockerfile.builder`.
- Linux arm64: Go 1.26.1, native GCC 11.4 and Ubuntu 22.04/glibc 2.35. The Go toolchain was extracted from an existing official Go image and linked using the native glibc GCC. The library was subsequently loaded and exercised inside the official CPA Debian Bookworm image.

Detailed machine-readable reports and synthetic screenshots are generated under the ignored `build/` directory. They are verification artifacts, not benchmark results from real models.

## Automated checks completed

Race-enabled Go tests passed on amd64 and arm64. They cover exact JSON numbers/types, duplicate keys, ambiguous final answers, independent probability enumeration, immutable task hashes, pinned account/provider calls, disabled credential handling, registry-only config credentials, cancellation, incomplete stream completion, evidence-preserving retries, private history permissions, restart interruption and baseline protection.

Both native architectures passed the same 13 integration checks using the actual CPA C ABI host, three synthetic credentials and a localhost mock upstream:

1. Plugin registration and runtime account discovery.
2. Account/history authentication.
3. Public static resources without account data.
4. Real host execution, response translation and grading of controlled answers.
5. Both exact credentials execute; references are not appended to prompts.
6. HTML artifacts stay separate from objective scores.
7. Single-item retries create independent history.
8. Baselines and JSON export contain no credentials.
9. Translated token-limit truncation remains ungraded.
10. Cancellation closes work and does not score partial replies.
11. Native Codex callback records preserve text and explicit completion.
12. A deliberately rejected credential never falls back to the other credential.
13. Restart restores raw history, cancellation states and the baseline.

Each integration run generated 14 requests to the local synthetic upstream and zero requests to real models.

## Browser checks completed

Chromium exercised the real plugin resource with authenticated synthetic data:

- Account/model/case selection, request estimate, start, progress and completed scores.
- Result filters, raw replies and per-field checks.
- Side-by-side scripted HTML previews; scripts execute, parent-document access is rejected and network connections are blocked.
- Baseline setting, matching baseline deltas, downloaded JSON and single-item retry.
- Historical records, light/dark layouts and 390px mobile setup/results without horizontal overflow.
- Custom prompt execution without a fabricated grade.
- Stream and queue cancellation through the UI.
- No browser page errors.

The stream-pause test caught a real progress-refresh bug: initial protocol events could consume the throttle window before the first short text delta. Publishing the first text immediately fixed the missing live preview; the UI cancellation test then passed.

## Compatibility findings retained

The first isolated attempt used a musl-built library and failed with missing `libc.musl-x86_64.so.1`. The current official CPA image is Debian Bookworm, so release libraries now use glibc. This loader failure was not treated as a credential or model failure.

The host requires a nonempty `GitHubRepository` metadata field even for local development. The plugin declares its planned source address; publication is a separate action and has not been performed.

Current CPA Manager Plus uses Bearer authentication and does not pass an authenticated session into resource iframes. The plugin asks for the service's management key and holds it only in memory. It does not read the Manager's saved browser credentials or put a key into a URL.

The first real Codex smoke request exposed another transport mismatch: the native executor supplies separate `event:` and `data:` callback records without SSE line separators, while OpenAI-compatible translation supplies framed SSE bytes. The original byte-only parser ignored these native records, retained the request as failed and did not grade it. Version 0.1.1 accepts complete native records as well as arbitrarily split framed SSE, with the same explicit completion checks. A native Codex mock path and a regression test now cover this distinction. The original failed production record is preserved.

## Deployment checks

The arm64 library was hot-loaded into the existing CPA deployment. Core and Manager container start times remained unchanged. Existing account disabled flags, priorities and notes, account grouping metadata, the enabled timezone plugin, and unrelated effective configuration were verified before and after installation. A private remote configuration backup was retained.

The public Manager sidebar, authenticated plugin entry, host theme bridge and nine credential rows (seven disabled) were exercised in Chromium. A short custom request using `gpt-6-luna` was initiated from that page. The first attempt exposed the native-record parser bug; after upgrading to 0.1.1, the UI's retry action reused the same credential and prompt and returned exactly `CPA_MODEL_QUALITY_OK`. Both records remain in history. Exactly two real model requests were made; this was a transport smoke check, not an intelligence benchmark.

A final browser reopen hit one navigation timeout. The successful retry's terminal state, exact raw reply and absence of an automatic grade were independently verified through the core management API over SSH. This timeout is retained as a browser/network verification limit, not described as a successful browser result.

These checks establish plugin behavior against controlled responses. They do not measure a real model's intelligence or establish model identity. User-triggered benchmark runs produce their own raw evidence.
