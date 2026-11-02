[English](RELEASE_NOTES.md) | **简体中文**

首个公开发行版，包含完整源码和 Linux amd64 / arm64 原生插件包。

本版修复 CPA Manager Plus 深色主题下“推理档位”等原生下拉菜单的文字和背景对比度问题，兼容管理台主题与操作系统主题不同的情况。

页面已移除“题目与参考”预览卡片和底部模型身份说明，保留题目选择、自定义提示词、账号与模型选择、结果详情、自动评分、并排预览、历史和基线比较。

安装要求：支持动态插件的 CPA v8.0.47 或兼容的 ABI 1 / schema 6 宿主；Linux glibc 2.36 或更新版本。解压对应架构的 ZIP，将 `model-quality.so` 放入 CPA 的 `plugins/linux/<arch>/`，在插件配置中启用，并设置可持久化的 `data_dir`。完整配置见 [中文 README](https://github.com/Renwang-Huang/cpa-model-quality/blob/main/README.zh-CN.md#下载与安装)。

发布前由两种原生架构的 GitHub Actions 分别执行构建、Go race 测试、Vet，以及真实 CPA 宿主上的模拟上游集成检查。校验值见 `checksums.txt`；构建信息见 `build-manifest-*.json`。验证使用虚拟凭据，不代表真实模型的能力评测结果。
