# Changelog

## 0.1.3

- Publish the source and native Linux amd64/arm64 release packages.
- Fix native select menus when the CPA Manager Plus theme differs from the
  operating system theme. Set an explicit active color scheme and opaque option
  backgrounds, including the reasoning-effort menu.
- Remove the setup page's question/reference preview card and model-identity
  footnote. Keep test selection, custom prompts, grading, result references and
  history intact.
- Add native architecture CI builds, race tests, vet, actual CPA-host integration
  against a synthetic upstream, checksums and tagged GitHub releases.

## 0.1.2

- Reuse the logged-in CPA Manager Plus session through a scoped parent-page API
  relay. Standalone pages retain an in-memory management-key form.

## 0.1.1

- Accept complete native Codex callback records alongside framed SSE streams.

## 0.1.0

- Initial account-isolated testing, scoring, previews, history and baselines.
