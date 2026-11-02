**English** | [简体中文](README.zh-CN.md)

<div align="center">
  <img src="web/logo.svg" width="68" alt="Model Quality Lab">
  <h1>CPA Model Quality Lab</h1>
  <p>Compare accounts and models using the same prompts and parameters.</p>
  <p>Native CPA plugin · Batch testing · Side-by-side previews · Reference-based grading · Historical baselines</p>
</div>

## Overview

Open **模型测智** (Model Quality Lab) in the CPA Manager Plus sidebar, select accounts, models and test cases, and compare original responses, token usage, timings and scores. Each test targets a specific credential ID and uses CPA's existing model translation, proxy and authentication settings. You do not need to enter account tokens.

The workflow was informed by [Cockpit Tools' account evaluation](https://github.com/jlcodes99/cockpit-tools/tree/main/src/components/codex/pelican). The UI, scheduler, storage, generated challenges and grading engine are independently implemented.

### A complete comparison

1. **Choose cases:** a cycling pelican SVG animation, candy reasoning, mathematics, code, logic, resource planning, long-context ledgers, selected public questions, or a custom prompt.
2. **Set parameters:** select one or more models, reasoning effort, 1–10 concurrent requests and 1–5 repetitions. The page shows the request count before starting; each batch supports up to 200 requests.
3. **Select accounts:** search by email, note or provider. Account notes remain visible, and disabled accounts cannot be selected.
4. **Inspect results:** view progress, individual errors, time to first output, total duration, the model name reported by the response and token usage. Expand per-field grading or preview generated HTML animations side by side.
5. **Save and repeat:** cancel a batch, rerun an item, retry unfinished items or export full JSON. Set a historical baseline and compare scores under matching conditions.

## Test cases and grading

| Category | Content | Evaluation |
| --- | --- | --- |
| Visual and creative | SVG animation of a pelican riding a bicycle | Isolated preview and manual comparison |
| Logical reasoning | Worst-case candy guarantees and unique solutions | Fixed reference answers |
| Mathematics | Combinatorial counting and reduced probabilities | Fixed reference answers |
| Code | Python references, shallow copies and closures | Fixed reference answers |
| Long context | Ledgers with multiple scopes, duplicate events and out-of-order versions | Fixed reference answers |
| Dependency planning | Minimum completion time with limited workers | Fixed reference answers |
| Instruction following | Filtering, deduplication and sorting | Fixed reference answers |
| Selected public questions | SimpleBench 1/4/8 and IFEval 1075 | Unique final-line answer / JSON format |
| Custom | Your own prompt | Original response and manual comparison |

**Format compliance and answer correctness are reported separately.** Strict JSON cases use exact per-field comparisons. They do not use keyword matching or accept duplicate keys, trailing text or numeric strings in place of numbers. Public multiple-choice cases require a unique final line in the form `FINAL: X`. The selected IFEval case checks format only. Visual and custom cases are excluded from the objective-answer average.

Failed or cancelled requests, service restarts, missing completion events, empty output, truncation and oversized responses retain their actual status and **are not scored as zero**. An overall average is shown only when every objective case has a gradable result; grading coverage is always displayed.

Each batch records the actual prompts, references, case hashes, suite and grader versions, request parameters and original results. Retries create a new batch and preserve the original. Baseline comparisons require the same account, requested model, reasoning effort, case hash, grader version and repetition number.

> This is a small test suite for repeatable comparisons. Scores, drawings and self-reported model names alone cannot establish model degradation or identity. Local checks of selected public questions are not official benchmark leaderboard results.

## Download and installation

Download the ZIP for your architecture from [GitHub Releases](https://github.com/Renwang-Huang/cpa-model-quality/releases/latest). The current version is **0.1.3**. Releases include Linux amd64 and arm64 packages, build manifests and `checksums.txt`. Verify downloaded archives before extracting them:

```sh
sha256sum -c checksums.txt --ignore-missing
```

The plugin targets **CPA v8.0.47 / C ABI 1 / schema 6**. Published libraries are built natively on Debian Bookworm and require **glibc 2.36 or newer**. They do not depend on matching the host's Go package binaries. Alpine/musl hosts require a separately built compatible library.

Place `model-quality.so` from the matching archive in CPA's `plugins/linux/<arch>/` directory, then enable it:

```yaml
plugins:
  enabled: true
  dir: ./plugins
  configs:
    model-quality:
      enabled: true
      data_dir: ./logs/model-quality-data
      retention_days: 7
      max_history: 100
      max_disk_mb: 256
```

`data_dir` must be writable and persistent. The default is under `./logs/`, which suits deployments with a read-only plugin directory and a writable logs volume. You can also use a separate data volume. History directories use mode `700` and files use mode `600`. Baselines and active batches are excluded from automatic cleanup.

After loading, refresh the plugin list in CPA Manager Plus. **模型测智** appears in the sidebar. Its resource page is:

```text
/v0/resource/plugins/model-quality/page
```

Reverse proxies must forward `/v0/resource/plugins/*` and `/v0/management/plugins/*`. Static resources can be accessed publicly; account data, history, original responses and all operations require CPA/Manager management authentication. When opened from a logged-in CPA Manager Plus session, the plugin reuses that session without asking for another password. This requires a Manager frontend with the plugin authentication bridge. Standalone access to the core retains an in-memory management-key form.

## Build and verification

```sh
# Native Linux architecture; Docker pins Go 1.26.1 and Debian Bookworm.
bash scripts/build.sh

# Grading, cancellation, interrupted streams, account pinning, retries,
# history and concurrency checks. Use a compatible Go toolchain.
go test -race ./...

# Actual CPA image, synthetic credentials and a local mock upstream.
python3 scripts/integration.py

# Optional browser checks; requires Playwright and Chromium.
python3 scripts/integration.py --keep
# Run in another terminal:
python3 scripts/browser_check.py
```

Packages are written to `dist/model-quality_<version>_linux_<arch>.zip` and include the library, documentation and licenses. A SHA-256 manifest is generated alongside them. Docker builds do not read real credentials from your workstation or server. If the builder image is unavailable, you can use Go 1.26.1, GCC and glibc with `scripts/build-native.sh`; the compatibility baseline of a local build depends on its toolchain.

GitHub Actions builds on native Linux amd64 and arm64 runners, runs race tests and vet, and exercises the actual CPA host against a mock upstream. A `v*` tag matching the source version publishes archives, manifests and checksums only after these checks pass. The UI supports the Manager's light and dark themes, including native select menus when the OS and Manager themes differ.

Repository documentation is available in English and Simplified Chinese. Each document has a language switch at the top. Future packages built from `main` include both language versions; the existing v0.1.3 archives remain unchanged.

See [Verification](docs/VERIFICATION.md) for validation records and [Design](docs/DESIGN.md) for architecture and API details.

## Data and runtime behavior

- Only explicitly started tasks generate model requests. Loading the plugin or opening its page does not test models.
- Tests consume real model quota. Limits are 200 requests per batch, 10 concurrent requests and 5 repetitions.
- The plugin obtains runtime account identities and models through `host.auth.list/models`. It passes the exact `auth_id` and provider to `host.model.execute_stream` without reading or exporting raw account tokens.
- Disabled accounts, exhausted quotas and upstream errors are reported as encountered. The plugin does not enable accounts or switch credentials automatically.
- Cancellation stops queued items and closes established streams. If a request is still establishing a stream, the batch stays in the cancelling state until the stream handle is available and can be closed.
- Each response is limited to 256 KiB; oversized responses remain ungraded. Generated HTML runs in a sandbox without same-origin privileges, with network requests, forms and popups blocked.
- History contains account display information, prompts and responses. Review those fields before sharing exports. Management keys are not written to history, browser storage or exported reports.
- A baseline is a locally saved reference. Repeated comparisons provide stronger evidence than a single score.

## License and sources

Original implementation code is licensed under [MIT](LICENSE). Sources and licenses for public questions, CPA ABI conventions and Go/YAML runtime dependencies are documented in [Third-party notices](THIRD_PARTY_NOTICES.md). Cockpit Tools application implementation code was not copied.
