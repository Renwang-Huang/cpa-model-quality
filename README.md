<div align="center">
  <img src="web/logo.svg" width="68" alt="Model Quality Lab">
  <h1>CPA 模型测智</h1>
  <p>同题、同参数，比较不同账号与模型的实际表现。</p>
  <p>CPA 原生插件 · 批量测试 · 并排预览 · 固定答案评分 · 历史基线</p>
</div>

## 做什么

在 CPA Manager Plus 侧边栏打开 **模型测智**，选择账号、模型和测试题，查看每个账号的原始回复、用量、耗时与评分。测试按凭证 ID 锁定账号，沿用 CPA 的模型转换、代理和鉴权，不需要填写账号令牌。

工作流参考 [Cockpit Tools 的账号测智](https://github.com/jlcodes99/cockpit-tools/tree/main/src/components/codex/pelican)。插件的界面、调度、存储、题目生成和评分引擎独立实现。

### 一次完整的比较

1. **选题**：鹈鹕骑车 SVG 动画、糖果推理、数学、代码、逻辑、资源规划、长文账本、公开精选题，或自定义提示词。
2. **选参数**：选择一个或多个模型、推理档位、1–10 并发及1–5次重复；开始前显示实际请求数，单批最多200次。
3. **选账号**：搜索邮箱、备注和提供商；列表保留账号备注，禁用账号不能被选中。
4. **看结果**：查看进度、单项错误、首字与总耗时、响应自报模型和 tokens；展开逐字段评分，或并排预览 HTML 动画。
5. **留证与复测**：取消任务、单项重测、重试未完成项、导出完整 JSON；历史中设置基线，按相同配置查看分数变化。

## 题库与评分

| 类别 | 内容 | 评估方式 |
| --- | --- | --- |
| 视觉与创作 | 鹈鹕骑自行车 SVG 动画 | 隔离预览，人工比较 |
| 逻辑推理 | 糖果最坏情况保证、唯一解 | 固定答案 |
| 数学 | 组合计数与约分概率 | 固定答案 |
| 代码 | Python 引用、浅拷贝、闭包 | 固定答案 |
| 长上下文 | 多作用域、重复事件和乱序版本账本 | 固定答案 |
| 依赖规划 | 有限工人下的最短工期 | 固定答案 |
| 指令遵循 | 数据过滤、去重和排序 | 固定答案 |
| 公开精选 | SimpleBench 1/4/8、IFEval 1075 | 唯一末行选项 / JSON格式 |
| 自定义 | 自由提示词 | 原始回复，人工比较 |

**格式与答案分别报告。** 严格 JSON 题逐字段精确比较，不做关键字命中判分，不接受重复键、尾随文字或数字字符串冒充数值。公开选择题使用唯一的末行 `FINAL: X`。IFEval 精选题仅测格式，绘图和自定义题不计入客观答案平均分。

失败、取消、服务重启、缺失完成事件、空回复、截断和超长回复保留状态，**不记为零分**。整体平均分仅在全部客观题取得可评分结果时显示；始终同时展示评分覆盖。

每批记录实际提示词、参考答案、题目哈希、题库与评分版本、请求参数和原始结果。重试创建新批次并保留原记录。基线比较要求同账号、同请求模型、同推理档位、同题目哈希、同评分版本和同轮次。

> 这是便于复测的小题集。分数、绘图和模型自报名称不能单独证明“降智”或实际模型身份。精选公开题的本地检查不是对应基准的官方榜单成绩。

## 安装

已针对 **CPA v8.0.47 / C ABI 1 / schema 6** 适配。构建目标为 Linux amd64 和 arm64，使用 glibc 工具链匹配当前官方镜像；不依赖宿主 Go 包的二进制一致性。Alpine/musl 宿主需要另行构建对应库。

将对应架构包中的 `model-quality.so` 放进 CPA 的 `plugins/linux/<arch>/` 目录。配置示例：

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

`data_dir` 必须可写并持久化。默认目录放在 `./logs/` 下，适合插件库目录只读、日志卷可写的部署；也可改为单独的数据卷。历史目录权限700、文件600。基线与正在运行的批次不参与自动清理。

加载后在 CPA Manager Plus 刷新插件列表，侧边栏会出现 **模型测智**。页面路径：

```text
/v0/resource/plugins/model-quality/page
```

反向代理需转发 `/v0/resource/plugins/*` 和 `/v0/management/plugins/*`。页面静态资源可公开访问；账号、历史、原始回复和所有操作接口均由 CPA/Manager 的管理认证保护。首次打开插件页面时，输入当前服务的管理密钥（Manager 入口使用面板管理员密钥）；密钥只保留在本页内存，刷新页面后需重新输入。

## 构建与验证

```sh
# 原生 Linux 架构，Docker 内固定 Go 1.26.1 + Debian Bookworm
bash scripts/build.sh

# 评分、取消、断流、账号锁定、重试、历史与并发检查
go test -race ./...

# 真实 CPA 镜像 + 虚拟凭证 + 本机模拟上游
python3 scripts/integration.py

# 可选：浏览器流程验证（需安装 Playwright 和 Chromium）
python3 scripts/integration.py --keep
# 在另一个终端运行
python3 scripts/browser_check.py
```

产物位于 `dist/model-quality_<version>_linux_<arch>.zip`，包括库、说明和许可证，并生成 SHA-256 清单。Docker 构建不读取本机或服务器上的真实凭证。无法拉取构建镜像时，也可使用 Go 1.26.1、GCC 和 glibc 运行 `scripts/build-native.sh`；本次 ARM64 包在 Ubuntu 22.04 / glibc 2.35 原生构建，并在 CPA 官方 Debian Bookworm 镜像中完成加载与调用验证。

详细验证记录见 [验证说明](docs/VERIFICATION.md)，接口与运行边界见 [设计说明](docs/DESIGN.md)。

## 数据与运行边界

- 仅主动开始的任务才发起生成请求；进入页面和加载插件不测试模型。
- 测试消耗真实模型额度。单批最多200次请求、10并发、5轮重复。
- 通过 `host.auth.list/models` 获取运行身份及模型，调用 `host.model.execute_stream` 时传入精确 `auth_id` 和提供商；不导出或读取原始账号令牌。
- 账号禁用、配额耗尽和上游错误按实际结果报告，不自动启用账号或切换账号。
- 取消会停止排队项并关闭已建立的模型流。若仍在等待上游建立流，批次保持“正在取消”，取得流句柄后立即关闭；不会假装请求已停止。
- 单条回复上限256 KiB，超限未评分；资源预览使用无同源权限的 sandbox，禁止网络请求、表单和弹窗。
- 历史包含账号展示信息、提示词和回复；导出前请自行检查这些内容。管理密钥不写入历史、浏览器存储或导出报告。
- 基线是本机保存的参考结果；多次稳定复测比单次分数更有意义。

## 许可与来源

原创实现采用 [MIT](LICENSE)。公开题、CPA ABI 接口及 Go/YAML 运行库的来源和许可证见 [第三方说明](THIRD_PARTY_NOTICES.md)。未复制 Cockpit Tools 的应用实现代码。
