# Architecture and evidence boundaries

## Integration

The library uses CPA's stable JSON-over-C ABI, with ABI version 1 and schema version 6. Its only declared capability is `management_api`; no request interception, routing, credential mutation, or automatic generation capability is registered.

Public resources contain static UI assets only. Account discovery, batch creation, progress, artifacts, exports, history deletion, baseline changes, and cleanup are authenticated management routes. The current CPA Manager Plus authorizes plugin APIs through a Bearer header and does not inject a session into resource iframes. The UI therefore asks once per page load for the current service admin key and keeps it only in memory. The key is never placed in URLs, browser storage, history, exports, or model requests.

The internal host model callback is passed an exact auth ID, provider and model. Disabled credentials are rejected, and retries look up the same credential again. Expected answers and explanations never enter the generated request body. Host callback context IDs belonging to the short management request are deliberately not attached to asynchronous jobs.

## Lifecycle

One batch runs at a time. Its worker pool has bounded concurrency and a bounded request count. Each item transitions from queued to running and exactly one terminal state. Cancelling stops queue admission and closes established host model streams. A model callback still establishing its initial stream may finish establishing before it can be closed; the UI preserves cancelling state until workers have actually returned.

Quiesce closes the active batch and waits for workers. Restarted unfinished batches are marked interrupted and retain their existing replies. Retrying creates a distinct batch referencing the prior ID and preserving the original prompt, expected fields and grader version.

## Storage

Private JSON files use temporary-file replacement and fsync before rename. Files are mode600 and the directory mode700. Runtime credentials and admin keys are not part of the storage schema. Retention applies by age, batch count and disk budget while preserving the active batch and chosen baseline. The baseline is stored atomically in `settings.json`.

## Grading

The parser requires explicit, successful stream completion. `[DONE]` or connection EOF alone is insufficient for a Responses request. Error, incomplete, cancellation, missing completion, empty output and limits never produce zero-valued grades.

Strict JSON parsing rejects duplicate keys recursively, excessive nesting and trailing tokens. Number comparisons preserve precision; number length and exponent limits bound grading work. Fields are compared structurally with type distinctions. Format checks and answer checks are independent. Choice grading accepts exactly one terminal `FINAL: X` line.

The UI only shows a complete objective-suite average after every requested objective item is successfully graded. Drawing, free text, custom prompts and format-only IFEval checks are excluded from this average. Baseline deltas require matching account, requested model, effort, exact case hash, grader version and trial number.

## Preview

Model HTML is never inserted into the application DOM. It is parsed as an inert document and rendered in a sandboxed opaque-origin iframe with scripts allowed only inside that frame. An additional restrictive CSP blocks connections, nested frames, forms, external assets and object embedding. The frame cannot read parent cookies, keys or application storage. Downloaded HTML also receives the restrictive policy.

## API

All paths below are relative to `/v0/management/plugins/model-quality/`:

| Method | Path | Operation |
| --- | --- | --- |
| GET | bootstrap | Account/model metadata, catalog, current batch and baseline |
| POST | batches | Start an explicitly requested batch |
| GET | batches | History summaries |
| GET | batch?id= | Progress and metadata without full replies |
| GET | artifact?id=&item= | One original reply and optional HTML |
| POST | cancel | Cancel the active batch by ID |
| POST | retry | Create a new batch for explicit item IDs or incomplete items |
| GET | export?id= | Full batch JSON |
| DELETE | batch | Delete a non-active, non-baseline history item |
| POST | baseline | Select a completed reference batch or clear it |
| POST | cleanup | Apply configured retention |
