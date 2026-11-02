[English](THIRD_PARTY_NOTICES.md) | **简体中文**

# 来源与第三方说明

## 设计参考

[Cockpit Tools](https://github.com/jlcodes99/cockpit-tools) 的账号测试工作流用于功能对照，包括按账号隔离的批次、模型与推理档位选择、预览、重试和不可变历史。所检查的修订记录在 [验证说明](docs/VERIFICATION.zh-CN.md) 中。本项目未复制 Cockpit Tools 的应用代码及其原始题目样例；界面、调度、评分、存储、传输解析和原创题目均独立实现。

## SimpleBench 公开数据

- 版权所有：Copyright (c) 2024 simple-bench；MIT 许可证。
- 来源修订：`fbc2e429085bdedad7d1a236d2bc9bc18c95f16e`。
- 来源：https://github.com/simple-bench/SimpleBench/blob/fbc2e429085bdedad7d1a236d2bc9bc18c95f16e/simple_bench_public.json
- 选用题目 ID：1、4、8。
- 修改：追加了明确且唯一的末行答案格式要求；增加中文标题与本地评分描述。原始提示词和参考选项保持不变。
- 完整许可证：[benchmarks/SimpleBench-LICENSE](benchmarks/SimpleBench-LICENSE)。

## IFEval

- Google Research Authors；Apache License 2.0。
- 来源修订：`26d8ccdab6fec61b5c83ad6327ea8bda9e580288`。
- 来源：https://github.com/google-research/google-research/blob/26d8ccdab6fec61b5c83ad6327ea8bda9e580288/instruction_following_eval/data/input_data.jsonl
- 选用 key：1075。原始提示词未修改。
- 独立实现的本地 JSON 格式检查不是完整的官方 IFEval 评估器，分数不能描述为官方 IFEval 成绩。
- 完整许可证：[benchmarks/GoogleResearch-LICENSE](benchmarks/GoogleResearch-LICENSE)。

## CLIProxyAPI

- C ABI 结构及回调约定遵循官方 CLIProxyAPI v8.0.47（`90c447d3a8727d115aa7552ec93ce023736ed492`）。
- 版权所有：Copyright (c) 2025–2005.9 Luis Pater; Copyright (c) 2025.9–present Router-For.ME；MIT 许可证。版权标记按上游原文保留。
- 来源：https://github.com/router-for-me/CLIProxyAPI/tree/v8.0.47/examples/plugin
- 完整许可证：[licenses/CLIProxyAPI-LICENSE](licenses/CLIProxyAPI-LICENSE)。

## 运行时依赖

- Go：The Go Authors，BSD 风格许可证；见 [licenses/Go-LICENSE](licenses/Go-LICENSE)。
- gopkg.in/yaml.v3 v3.0.1：Canonical Ltd.，Apache-2.0/MIT；原始许可证及 notice 见 [licenses/YAML-LICENSE](licenses/YAML-LICENSE) 和 [licenses/YAML-NOTICE](licenses/YAML-NOTICE)。

未单独列出的应用代码及原创测试样例均为 Copyright (c) 2026 Renwang Huang，按仓库 MIT 许可证分发。第三方许可证原文保持不变。
