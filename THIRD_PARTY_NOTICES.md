**English** | [简体中文](THIRD_PARTY_NOTICES.zh-CN.md)

# Sources and notices

## Design reference

The account-testing workflow of [Cockpit Tools](https://github.com/jlcodes99/cockpit-tools), inspected at its checked-out revision recorded in [Verification](docs/VERIFICATION.md), informed the feature comparison: account-isolated batches, model/effort selection, previews, retries, and immutable history. Cockpit Tools application code and its original challenge fixtures were not copied. This project has independently implemented UI, scheduling, grading, storage, transport parsing, and original challenges.

## SimpleBench public data

- Copyright (c) 2024 simple-bench; MIT.
- Source revision: `fbc2e429085bdedad7d1a236d2bc9bc18c95f16e`.
- Source: https://github.com/simple-bench/SimpleBench/blob/fbc2e429085bdedad7d1a236d2bc9bc18c95f16e/simple_bench_public.json
- Selected question IDs: 1, 4, 8.
- Changes: an explicit unique final-line response contract was appended; Chinese titles and local grading descriptions were added. Original prompts and reference choices are retained.
- Full license: [benchmarks/SimpleBench-LICENSE](benchmarks/SimpleBench-LICENSE).

## IFEval

- Google Research Authors; Apache License 2.0.
- Source revision: `26d8ccdab6fec61b5c83ad6327ea8bda9e580288`.
- Source: https://github.com/google-research/google-research/blob/26d8ccdab6fec61b5c83ad6327ea8bda9e580288/instruction_following_eval/data/input_data.jsonl
- Selected key: 1075. The original prompt is unmodified.
- The independent local JSON-format check is not the complete official IFEval evaluator. Scores must not be described as official IFEval results.
- Full license: [benchmarks/GoogleResearch-LICENSE](benchmarks/GoogleResearch-LICENSE).

## CLIProxyAPI

- C ABI structures and callback conventions follow official CLIProxyAPI v8.0.47 (`90c447d3a8727d115aa7552ec93ce023736ed492`).
- Copyright (c) 2025–2005.9 Luis Pater; Copyright (c) 2025.9–present Router-For.ME; MIT.
- Source: https://github.com/router-for-me/CLIProxyAPI/tree/v8.0.47/examples/plugin
- Full license: [licenses/CLIProxyAPI-LICENSE](licenses/CLIProxyAPI-LICENSE).

## Runtime dependencies

- Go: The Go Authors, BSD-style license; [licenses/Go-LICENSE](licenses/Go-LICENSE).
- gopkg.in/yaml.v3 v3.0.1: Canonical Ltd., Apache-2.0/MIT; original license and notice in [licenses/YAML-LICENSE](licenses/YAML-LICENSE), [licenses/YAML-NOTICE](licenses/YAML-NOTICE).

All unlisted application code and original test fixtures are Copyright (c) 2026 Renwang Huang and distributed under the repository MIT license.
