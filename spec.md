# Kobashi2 Product Spec

## 产品定义

- 本文是 Kobashi2 产品行为与用户体验的唯一 source of truth。
- Kobashi2 为 Claude Code、Codex 和 GitHub Copilot app 提供统一的推理来源管理。
- 客户端名称显示为 `Claude`、`Codex`、`Copilot`。
- 支持 macOS 13+ 的 Apple Silicon / Intel，以及 Windows 10/11 x64。
- 两个平台提供一致的 source 管理、模型选择和客户端控制体验。

## Sources

- 用户通过 `Add account` 添加推理来源，Provider 选项为 `Local` 和 `GitHub Copilot`。
- 可同时保存多个本地模型 accounts 和多个 GitHub Copilot accounts。
- 同一 GitHub account 不重复添加；再次登录更新该 source 的凭据。
- 同一本地模型不重复添加；再次添加可继续未完成的安装或重新连接。
- 同一时刻只有一个 active source，供所有已启用的客户端共享。
- Source 列表显示 Provider、账号或模型名称，以及当前选择标记。
- 用户可添加、选择、断开和移除 source。
- 添加或准备新 source 时，当前 source 保持可用；新 source 准备完成后才可切换。
- 取消添加保留当前 source 和客户端开关状态。
- 切换 source 保留三个客户端的启用偏好。
- 切换、断开或移除 source 不得中断正在执行的推理或 tool call；无法安全完成时保留当前连接并显示简短原因。
- 断开保留 source；移除 active source 后不自动选择其他 source。
- 打开 source 列表不受其他操作影响；暂不可执行的操作显示为不可用。
- 保存的 sources、active source 和客户端启用偏好在重启后保留。
- 已保存的 GitHub 账号作为 GitHub Copilot sources 保留，无需重新添加。

## GitHub Copilot Source

- 支持 GitHub 浏览器授权和 access token 登录。
- 连接使用所选账号的 Copilot 模型权限和额度。
- 授权期间显示等待、取消和失败状态；可恢复的连接失败不要求重新登录。
- 模型目录与能力属于各自账号，不在不同 sources 间混用。
- 账号切换后显示新账号可用的模型。
- GitHub source 的失效不影响 Local source 的使用。

## Local Source

- Local setup 完全在 Kobashi2 内完成。
- Mac Local 使用 oMLX，要求 Apple Silicon 和 macOS 15+；不支持的设备仍可使用 GitHub Copilot。
- 用户无需输入 Base URL、端口、API key、模型路径或命令。
- 用户无需自行安装 Ollama、LM Studio、Python 或其他推理环境。
- Kobashi2 自动准备适合本机的运行环境、下载模型并启动本地推理。
- 运行环境按硬件和模型自动选择；Mac 与 PC 可采用不同的加速引擎，用户无需选择或配置 runtime。
- 推荐以模型、权重精度、运行引擎、KV 精度和上下文的组合为单位；不要求所有平台使用同一种模型文件格式。
- Local 不需要 Kobashi2 的 GitHub 登录或 Copilot 订阅。
- `Add account` 页面同时提供 `GitHub` 登录和本地模型列表；默认选中推荐模型，显示下载大小和 `Add`。
- 用户选择 `Add` 后自动完成环境准备、模型下载和启动，无需逐项确认。
- 已有的完整模型文件可复用，不重复下载。
- 模型就绪后自动成为 active source；若存在运行中的请求，保留原 source，待其结束后完成切换。
- 已配置的 Local source 在下次被选择或恢复时自动启动，无需打开其他应用。
- 完成必要下载后，本地推理可离线使用。
- Local 推理内容不得发送到 GitHub 或其他云端模型，不得自动回退到 cloud source。
- 客户端自身的账号、插件、MCP 和联网工具继续遵循其各自的访问与登录要求。

## 自动模型推荐

- Local 默认模型选项为 `Recommended`，Kobashi2 自动选择并运行适合本机的模型。
- 默认推荐以交互速度为优先，在满足 coding 和 tool calling 能力的前提下，缩短首个可见响应、生成和完整工具调用循环的等待时间。
- 优先提供 non-thinking 模型或支持关闭 thinking 的模式；不因 RAM 更大而自动升级为更慢的 dense 模型、更高权重量化精度或更长的 reasoning。
- 推荐同时考虑总 RAM、当前可用 RAM、GPU / unified memory、运行环境、可用磁盘空间和模型的 coding / tool calling 能力。
- 内存预算须覆盖模型、上下文和推理开销，并为操作系统及客户端保留空间。
- GPU VRAM 与系统 RAM 分别考虑；unified memory 不重复计入可用容量。
- 不能将模型下载大小作为运行内存需求，也不能将 MoE active parameters 作为完整模型占用。
- 以下为默认候选；实际可用资源不足时选择表内更小的兼容模型。

| 总 RAM 档位 | 默认推荐候选 | 模型下载量约值 | 其他候选 |
| --- | --- | --- | --- |
| 16 GB | Qwen3 4B Instruct 2507 · 4-bit | 2.5 GB | Qwen3 8B · 4-bit，约 5.2 GB |
| 32 GB | Qwen3-Coder 30B-A3B · 4-bit | 19 GB | Qwen3 4B Instruct 2507 · 4-bit；Qwen3 8B · 4-bit |
| 48 GB | Qwen3-Coder 30B-A3B · 4-bit | 19 GB | Qwen3 4B Instruct 2507 · 4-bit；Qwen3 8B · 4-bit |
| 64 GB | Qwen3-Coder 30B-A3B · 4-bit | 19 GB | Qwen3 4B Instruct 2507 · 4-bit；Qwen3 8B · 4-bit |

- 模型文件采用对应平台支持的量化格式，下载页显示实际包大小；表中大小为参考值。
- RAM 位于两档之间时以较低档为候选起点；高于 64 GB 时仍以 64 GB 档为默认候选。
- 低于 16 GB 时，仅在资源足够的情况下提供最小兼容候选；无合适模型时显示 `Insufficient memory`。
- 32 GB、48 GB 与 64 GB 使用相同的默认模型候选；额外内存用于减少资源竞争、保留会话缓存及按需容纳上下文，不强制选择更大模型或增加上下文长度。
- Apple Silicon 按实际可用 unified memory 选择；PC 和 Intel Mac 还须考虑 GPU 可用内存与加速能力，不仅按系统 RAM 套用推荐表。
- 默认优先选择模型与所需缓存可驻留加速设备内存的配置；无足够 GPU 内存时优先推荐较小模型，不以大量 CPU offload 作为高速体验的默认配置。
- 上下文容量按实际资源分配，并向客户端显示实际可用值；不得直接宣称模型的理论最大上下文可用。
- 用户可保留 `Recommended`，或在适合本机的候选中手动选择模型；不要求选择量化参数。
- 推荐模式显示实际运行的模型名称；手动选择后不得静默替换模型。
- 资源变化只在 setup、启动或空闲时影响推荐，不在生成过程中切换模型。
- 推荐变化需要下载另一模型时，显示模型和下载量，由用户选择 `Download` 后继续。
- 内存不足时保留 source 和会话，显示可执行的恢复操作；不得依靠持续大量 swap 维持默认体验。

## KV Cache 与响应速度

- Local 必须支持推理过程中使用的 quantized KV cache，K 与 V 默认采用 oMLX 支持的 TurboQuant 格式；支持层使用 4-bit。
- 低于 4-bit 的 KV 精度不作为默认配置。
- 推理中的 KV 量化、prefix cache 复用和磁盘缓存压缩分别描述；仅压缩磁盘缓存不满足 quantized KV 的要求。
- 模型权重量化与 KV cache 量化分别管理；使用 Q4 权重不代表 KV cache 已量化。
- 默认模型与运行环境组合必须支持上述 quantized KV 配置；不支持时显示实际限制，不静默退回未量化缓存并宣称已启用。
- 用户无需手动配置 KV 类型、加速选项或显存参数；Kobashi2 自动管理，并在 Local 详情中提供实际缓存精度和上下文容量。
- 同一会话连续交互时复用可用的 prompt / prefix cache，减少重复上下文处理；不同 sources 和会话保持内容隔离。
- 模型在使用期间保持就绪，避免每轮重新加载；默认上下文以交互速度和实际任务需要为依据，不自动分配理论最大容量。
- 内存预算包含实际 KV 格式、上下文长度、并发会话及模型的其他状态缓存；混合架构中不能量化的状态不计为已量化 KV。
- 仅部分 attention layers 可量化时，Local 详情须显示实际覆盖范围；prefill 的峰值内存与 decode 的缓存占用分别纳入资源预算。
- 量化配置的选择同时考虑响应延迟和输出质量，不将更低缓存精度直接视为更高速度。

## 下载与运行状态

- Setup 状态为 `Preparing`、`Downloading`、`Starting`、`Ready` 或 `Error`。
- 下载显示进度和已下载 / 总大小，支持取消与继续。
- 下载中断或应用重启后可继续，保留已完成的下载内容。
- 下载或启动失败显示简短原因和 `Retry`，无需用户执行命令或查找文件。
- 磁盘不足时显示缺少的空间；资源不足时不将 source 显示为 `Ready`。
- `Ready` 表示运行环境和模型可供客户端推理，不仅表示下载完成。
- 准备操作不阻塞窗口交互或当前 source 的推理。
- 三个客户端共享当前本地模型；并发超过资源容量时排队，并允许取消等待中的请求。
- Source 不再使用时释放其模型占用，保留已下载文件。
- 退出 Kobashi2 后停止其管理的本地推理，保留模型供下次使用。
- 移除 Local source 时可选择一并删除 Kobashi2 管理的模型，并显示可释放空间；默认保留下载。
- 不移除用户自行安装的运行环境、模型或其他应用的数据。

## 客户端体验

- `Claude`、`Codex`、`Copilot` 各有独立开关，状态为 `Active` 或 `Paused`。
- 操作期间在原状态位置显示 `Working...`，完成后恢复正常状态。
- 两种 Provider 均支持三个客户端的文本输入、streaming、tool calls 和 tool results。
- 展示的模型、上下文、reasoning 和其他能力必须与 active source 实际提供的能力一致。
- 不支持的能力不显示为可用；请求失败不得显示为成功，或静默替换模型。
- 已开始的响应失败时向客户端保留明确错误，不自动重放整次生成。
- 启用时自动连接所需客户端配置，禁用或退出时恢复 Kobashi2 管理的配置。
- 保留客户端已有账号、用户配置、plugins、MCP、组织策略及用户后续修改。
- Kobashi2 的 source 认证与客户端自身的登录相互独立。
- 客户端需要重启才能读取模型目录时，显示简短操作提示；Kobashi2 不自动重启客户端。
- 切换 Provider 后，不在原会话仍选择云端模型的情况下静默改用本地模型；需要重选模型或新建会话时给出明确提示。

## Copilot 会话

- Copilot 未打开或未选择会话时仍可启用，应用可用后自动完成连接。
- 自动连接仅作用于符合条件的 idle local sessions，不影响运行中的会话、remote / cloud sessions 或其他 providers。
- 用户手动修改 provider、模型或会话选项后，Kobashi2 不自动改回。
- 保留会话原来的选择，断开时恢复仍由 Kobashi2 管理的会话。
- 无法安全恢复时保留连接并显示错误。
- `Active` 表示该客户端连接功能已启用，不保证所有会话均已切换，也不保证立即开始的新会话首条消息已路由。
- Copilot app 的 GitHub 登录保持独立；Local source 不替代应用自身的账号要求。

## 窗口与视觉

- 主窗口显示当前 source、三个客户端开关、Settings 入口和版本。
- 使用 `Accounts` 和 `Add account` 统一管理两种 Provider。
- UI 文案以标签、状态和必要操作为主，不显示技术配置、解释段落或成功通知。
- 界面使用 native controls、纯色背景和紫色主题；不添加装饰动画。
- `Active` / connected 使用绿色，错误使用红色，`Paused` / disconnected 使用中性灰色。
- 保留原客户端品牌图案；Kobashi2 标志为两个实心圆及连接线。
- 主页面和 Settings 共用固定窗口尺寸：Windows 内容区 400 × 370，macOS 内容区 400 × 390 logical pixels。
- Header、source 区、客户端区和版本区之间的间距为 16 logical pixels；版本保持底部锚定。
- Source 列表和 setup 可使用独立面板，长列表在面板内滚动，不改变主窗口尺寸。
- 状态、错误和进度更新不使布局跳动；主页面错误在 header 标题位置显示。
- 鼠标选择不出现粗边框；键盘焦点清晰可见，操作支持键盘和屏幕阅读器。
- 适配系统缩放和 high contrast，所有操作保持可见、可访问。

## Settings 与生命周期

- Settings 提供 `Start at login`、`Close to system tray` 和 `Appearance`。
- `Start at login` 默认关闭；仅在用户更改设置时改变开机启动行为。
- `Close to system tray` 默认开启；关闭窗口后继续提供推理。
- 关闭 `Close to system tray` 后，关闭窗口即退出。
- 开机启动且启用 tray 时在后台启动，否则显示主窗口。
- `Appearance` 选项为 `Auto`、`Light`、`Dark`；`Auto` 跟随系统后续变化。
- Settings 使用简洁标签，Appearance 仅出现在 Settings。
- Tray / menu bar 提供打开主窗口和 `Quit Kobashi2`；macOS 保留标准退出操作。
- Settings 不显示 quit 入口。
- 启动时显示 `Starting...`，恢复 sources 和客户端状态后才提供连接操作。
- 保存的 Local source 在恢复后显示模型启动状态，不要求重新 setup。
- 临时连接错误在恢复后清除；用户操作失败持续可见，直到解决或再次操作。
- 断开 source 不清除 Settings。

## 隐私与数据保留

- Local source 的模型推理在本机完成，其他设备不可直接使用该推理服务。
- 模型和运行环境下载不附带用户代码、prompts 或会话内容。
- 凭据不显示于普通 UI、日志或客户端配置中，不在不同 Providers 间转发。
- GitHub Copilot source 仅将推理请求交给用户选择的账号。
- Source 切换、更新或应用升级保留用户的账号、偏好、模型下载和原有客户端身份。
- 诊断错误不包含凭据或完整私有会话内容。
