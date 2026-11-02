**English** | [简体中文](RELEASE_NOTES.zh-CN.md)

The first public release includes the complete source and native Linux amd64 / arm64 plugin packages.

This version fixes text and background contrast in native select menus, including reasoning effort, when CPA Manager Plus uses a dark theme. It also handles cases where the Manager and operating system themes differ.

The setup page's question/reference preview card and model-identity footnote have been removed. Test selection, custom prompts, account/model selection, result details, automatic grading, side-by-side previews, history and baseline comparisons remain available.

Installation requires a CPA v8.0.47 build with dynamic plugin support, or a compatible ABI 1 / schema 6 host, on Linux with glibc 2.36 or newer. Extract the ZIP for your architecture, place `model-quality.so` in CPA's `plugins/linux/<arch>/`, enable the plugin and configure a persistent `data_dir`. See the [README](https://github.com/Renwang-Huang/cpa-model-quality#download-and-installation) for the complete configuration.

Before publication, GitHub Actions built both native architectures, ran Go race tests and vet, and exercised the actual CPA host against a mock upstream. Archive checksums are in `checksums.txt`; build information is in `build-manifest-*.json`. Validation uses synthetic credentials and does not represent an evaluation of real model capabilities.
