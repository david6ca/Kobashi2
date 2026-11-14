# ModelBridge Product Spec

## 产品定义

- 本文是 ModelBridge 产品行为与用户体验的唯一 source of truth。
- ModelBridge 为 Claude Code、Codex 和 GitHub Copilot app 提供统一的推理来源管理。
- 客户端名称显示为 `Claude`、`Codex`、`Copilot`。
- 支持 macOS 13+ 的 Apple Silicon / Intel，以及 Windows 10/11 x64。
- 两个平台提供一致的 source 管理、模型选择和客户端控制体验。

## Sources

- macOS 账户菜单分别提供 `Add GitHub account` 和 `Add a local model`。GitHub 页仅提供设备授权码登录，不提供手动 Access token 登录入口；本地模型页只展示模型选择、准备进度及添加按钮。未连接时的入口也打开同一个菜单。
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

- Local setup 完全在 ModelBridge 内完成。
- Mac Local 使用内置 llama.cpp + Metal，要求 Apple Silicon 和 macOS 15+；不支持的设备仍可使用 GitHub Copilot。
- 用户无需输入 Base URL、端口、API key、模型路径或命令。
- 用户无需自行安装 Ollama、LM Studio、Python 或其他推理环境。
- ModelBridge 自动准备适合本机的运行环境、下载模型并启动本地推理。
- 运行环境按硬件和模型自动选择；Mac 与 PC 可采用不同的加速引擎，用户无需选择或配置 runtime。
- 推荐以模型、权重精度、运行引擎、KV 精度和上下文的组合为单位；不要求所有平台使用同一种模型文件格式。
- Local 不需要 ModelBridge 的 GitHub 登录或 Copilot 订阅。
- `Add a local model` 页面不包含 GitHub 登录控件；默认选中推荐模型，显示下载大小和 `Add`。GitHub 登录不触发本地模型读取或安装。
- 用户选择 `Add` 后自动完成环境准备、模型下载和启动，无需逐项确认。
- 已有的完整 GGUF 模型文件可复用，不重复下载；旧 MLX 权重保留在原位置，不删除、不转换，也不误认为 GGUF 已就绪。
- 模型就绪后自动成为 active source；若存在运行中的请求，保留原 source，待其结束后完成切换。
- 已配置的 Local source 在下次被选择或恢复时自动启动，无需打开其他应用。
- 完成必要下载后，本地推理可离线使用。
- Local 推理内容不得发送到 GitHub 或其他云端模型，不得自动回退到 cloud source。
- 客户端自身的账号、插件、MCP 和联网工具继续遵循其各自的访问与登录要求。

## 自动模型推荐

- Local 默认模型选项为 `Recommended`，ModelBridge 自动选择并运行适合本机的模型。
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

- Local 必须支持推理过程中使用的 quantized KV cache，K 与 V 默认采用 llama.cpp 的 `q4_0` 格式（4-bit）；不将其标记为 TurboQuant。
- 低于 4-bit 的 KV 精度不作为默认配置。
- 推理中的 KV 量化、prefix cache 复用和磁盘缓存压缩分别描述；仅压缩磁盘缓存不满足 quantized KV 的要求。
- 模型权重量化与 KV cache 量化分别管理；使用 Q4 权重不代表 KV cache 已量化。
- 默认模型与运行环境组合必须支持上述 quantized KV 配置；不支持时显示实际限制，不静默退回未量化缓存并宣称已启用。
- 用户无需手动配置 KV 类型、加速选项或显存参数；ModelBridge 自动管理，并在 Local 详情中提供实际缓存精度和上下文容量。
- 同一会话连续交互时复用可用的 prompt / prefix cache，减少重复上下文处理；不同 sources 和会话保持内容隔离。
- 修改 context 时，仅在没有进行中的请求时重建 llama.cpp 的 KV cache；繁忙时拒绝本次修改并保留原容量。重建失败时恢复原容量，不重启 ModelBridge 或其他客户端。
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
- 退出 ModelBridge 后停止其管理的本地推理，保留模型供下次使用。
- 移除 Local source 时可选择一并删除 ModelBridge 管理的模型，并显示可释放空间；默认保留下载。
- 不移除用户自行安装的运行环境、模型或其他应用的数据。

## 客户端体验

- `Claude`、`Codex`、`Copilot` 各有独立开关，状态为 `Active` 或 `Paused`。
- 操作期间在原状态位置显示 `Working...`，完成后恢复正常状态。
- 两种 Provider 均支持三个客户端的文本输入、streaming、tool calls 和 tool results。
- 展示的模型、上下文、reasoning 和其他能力必须与 active source 实际提供的能力一致。
- 不支持的能力不显示为可用；请求失败不得显示为成功，或静默替换模型。
- 已开始的响应失败时向客户端保留明确错误，不自动重放整次生成。
- 启用时自动连接所需客户端配置，禁用或退出时恢复 ModelBridge 管理的配置。
- 保留客户端已有账号、用户配置、plugins、MCP、组织策略及用户后续修改。
- ModelBridge 的 source 认证与客户端自身的登录相互独立。
- 客户端需要重启才能读取模型目录时，显示简短操作提示；ModelBridge 不自动重启客户端。
- 切换 Provider 后，不在原会话仍选择云端模型的情况下静默改用本地模型；需要重选模型或新建会话时给出明确提示。

## Copilot 会话

- Copilot 未打开或未选择会话时仍可启用，应用可用后自动完成连接。
- 自动连接仅作用于符合条件的 idle local sessions，不影响运行中的会话、remote / cloud sessions 或其他 providers。
- 用户手动修改 provider、模型或会话选项后，ModelBridge 不自动改回。
- 保留会话原来的选择，断开时恢复仍由 ModelBridge 管理的会话。
- 无法安全恢复时保留连接并显示错误。
- `Active` 表示该客户端连接功能已启用，不保证所有会话均已切换，也不保证立即开始的新会话首条消息已路由。
- Copilot app 的 GitHub 登录保持独立；Local source 不替代应用自身的账号要求。

## 窗口与视觉

- 主窗口显示当前 source、三个客户端开关、Settings 入口和版本。
- 使用 `Accounts` 和 `Add account` 统一管理两种 Provider。
- UI 文案以标签、状态和必要操作为主，不显示技术配置、解释段落或成功通知。
- 界面使用 native controls、纯色背景和紫色主题；不添加装饰动画。
- `Active` / connected 使用绿色，错误使用红色，`Paused` / disconnected 使用中性灰色。
- 保留原客户端品牌图案；ModelBridge app 标志为两个细线圆环及连接线，menu bar 保留实心圆以保证小尺寸辨识度。
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
- Tray / menu bar 提供打开主窗口和 `Quit ModelBridge`；macOS 保留标准退出操作。
- Settings 不显示 quit 入口。
- 启动时显示 `Starting...`，恢复 sources 和客户端状态后才提供连接操作。
- 保存的 Local source 在恢复后显示模型启动状态，不要求重新 setup。
- 临时连接错误在恢复后清除；用户操作失败持续可见，直到解决或再次操作。
- 断开 source 不清除 Settings。

## 语言与产品名称

- macOS 的 About ModelBridge 使用原生 About 面板，显示本地化致谢：`Based on Kobashi, originally created by Xin Jin.`；README 的 Acknowledgments 同时链接原始 Kobashi 项目和作者。

- 产品名称为 `ModelBridge`，不添加 `Pomegram` 前缀或后缀。
- macOS Bundle ID 使用 `com.pomegram.modelbridge`。
- macOS / Windows 产品界面和 GTM 材料支持 13 个 locale：`en`、`zh-Hans`、`zh-Hant`、`ja`、`ko`、`de`、`fr`、`pt-BR`、`es`、`ar`、`hi`、`ru`、`vi`。
- Settings 提供 `Language`；macOS 的 `Privacy Statement` 只放在系统 app 菜单中，Windows 保留现有入口。tray / menu bar 只保留显示主窗口和退出入口。
- macOS 主窗口上半部（含标题栏）为纯黑实体背景，显示账户和额度，不加橙色底色。下半部为与 Disk Watcher sidebar 相同的边到边原生玻璃区域，显示三个客户端开关，不添加内部底板。
- macOS 内容不显示产品标题、logo 或版本，不给账户信息和开关列表绘制外框。使用带顶部留白的原生 unified title bar，Settings 位于右上角，与窗口控制按钮同排；原生窗口控制区域不随 RTL 镜像。正文左右边距为 28 pt，与原生窗口图标边线对齐；已连接主屏为 400×259 pt，Settings 为 400×220 pt、本地模型添加页为 400×212 pt、GitHub 添加页为 400×232 pt，其他登录流程保持 400×350 pt，切换页面保持窗口顶边位置。顶部账户区为 91 pt；客户端行高 44 pt，开关的可见间隔为 20 pt，玻璃区上下图标留白为 24 pt、开关留白为 28 pt；账户和客户端图标左边距为 28 pt。Settings 与 Back 均为无边框的 32×32 按钮，使用相同字号、字重与颜色的系统图标。
- Connected 的下拉箭头对齐正文右边线，文字与箭头可见间距约 6 pt；Connected 和箭头合为一个点击区域，整块点击均打开账户菜单，hover 背景包含箭头内边距。
- macOS menu bar 图标同步 app icon 的空心双圆与细连接线，采用单色 system template，适配浅色、深色与高对比度显示。
- macOS Settings、账户菜单和 Privacy 窗口使用原生玻璃材质；保留 Auto / Light / Dark，并遵循系统 Reduce Transparency 与 Increase Contrast 设置。 主屏自定义区域只使用一层 `.sidebar` / `.behindWindow` 的 `NSVisualEffectView`，窗口底色透明，不叠加 `NSGlassEffectView(.regular)`，不强制整窗 `.darkAqua`；黑色只绘制在账户区和标题栏。账户菜单直接使用 `NSPopover` 自带材质。
- Settings 顶部不显示产品 logo、名称或 Settings 图标，使用与 Add account 相同的返回按钮。
- Settings 不显示 Done，只通过返回箭头（支持 Escape）回到主屏。添加模型和 GitHub Connect 按钮保留 24 pt 底部边距。
- 默认跟随系统语言；明确选择持久保存并立即更新界面，不改变 source、客户端开关或正在运行的推理。
- 账号、模型名、用户输入和服务商返回内容保持原文；数字按所选 locale 格式化。
- 阿拉伯语采用 RTL 布局与文本方向，品牌图案和用户内容不做镜像翻转。
- 隐私声明可离线阅读，覆盖本地存储、凭据文件、日志、模型下载、GitHub Copilot 数据传输和账号移除范围。

## 隐私与数据保留

- Local source 的模型推理在本机完成，其他设备不可直接使用该推理服务。
- 模型和运行环境下载不附带用户代码、prompts 或会话内容。
- 凭据不显示于普通 UI、日志或客户端配置中，不在不同 Providers 间转发。
- GitHub Copilot source 仅将推理请求交给用户选择的账号。
- Source 切换、更新或应用升级保留用户的账号、偏好、模型下载和原有客户端身份。
- 诊断错误不包含凭据或完整私有会话内容。

- 主屏边距图的明确数值：`b`=20 pt，`e`=20 pt，`f`=24 pt；`h`=28 pt，`g`=8 pt，`r`=20 pt，`s`=24 pt，`t` 上下均为 28 pt；保留 `j`=12 pt、`q`=20 pt 及开关间隔 20 pt。其余已确认的 28 pt 外边距保持不变。

## Mac App Store 专用构建

- 用户于 2026-11-12 明确授权 App Store 专用 App Sandbox 与本地运行环境打包；不替换 Applications 中的普通版本，不恢复暂停的截图更新。
- Bundle ID 为 `com.pomegram.modelbridge`，保留产品名称 ModelBridge 和 13 个 locale。
- 原生 llama.cpp 引擎和所需动态库随 App 打包，不包含 Python、pip 或 oMLX；运行时只下载固定 revision 与 SHA-256 校验的 GGUF 模型数据。
- App 数据保存于自身 container；客户端配置目录通过系统文件夹选择与 security-scoped bookmarks 授权，helper 自行取得同一权限。
- App Store 版首次连接来源时客户端保持 Paused，用户启用对应客户端时授权其配置目录；普通版本的初始启用行为不变。
- Start at login 使用 SMAppService，不写 LaunchAgents 或全局用户环境变量。
- 构建、archive / export 使用 Xcode CLI；build 上传前须 commit 对应源码、push 到 main 并验证成功，上传回执记录 commit。

- Settings、本地模型添加页和 GitHub 添加页的返回标题行使用相同的 13 pt regular 字体，与主页账户名同处 y=8 pt。两页正文采用 24 pt 左右和底部边距。正文选项、picker、主页客户端名称及 Active/Paused 均为 12 pt；名称与状态采用紧凑行间距。

- GitHub 添加页为 400×232 pt，返回标题行与账户行同高；授权码卡片左右边距 24 pt、内部边距 16 pt，包含授权码、Copy 和等待/错误状态。不显示 token 输入或提交按钮；GitHub 链接左对齐，生成授权码不会自动打开浏览器，取消后忽略迟到响应。所有 GitHub 添加状态均使用 ‹ 返回，不显示 X。

- 用户于 2026-11-13 决定 App Store 版 GitHub 图标使用官方黑／白；当前深色账号区与 client surface 使用白色，普通版原有配色不变。App Store 截图用同一 APP_STORE UI 编译条件捕获。
- 用户补充 App Store 版顶部账号名也使用白色（account_1 行）；普通版保留现有配色。
- 本地模型账号行与账号列表显示清晰的 CPU/chip 图标；本地上下文 slider 到下方 client 面板保留 20 pt 留白，后续 client 行随面板下移，保留原底边距。
- App Store helper 使用与父 app 相同的 signing identifier，确保可解析父 app 创建的 app-scoped bookmarks；实际目录授权验收覆盖取消、保存、helper 重启后复用和配置恢复。
- App Store 首发使用 1.0；构建脚本从 metadata/app-store/metadata.json 读取商店版本号，不再沿用普通版 package.json 的版本。Bundle ID 保持 com.pomegram.modelbridge，构建号继续递增。
- App Store runtime 只包含固定版本的 llama-server、所需原生动态库和 license；不复制通用 Python 或第三方 pip 环境。最终 archive 仍检查已知非公开 API 引用。
- App Store runtime 使用同版本 SciPy 的官方 OpenBLAS wheel，不链接审核点名的旧 Accelerate BLAS/LAPACK 接口；移除未使用的 PyArrow S3 后端，保留本地 Arrow/Parquet/datasets 功能。SDL2 使用已核验官方源码关闭 Cocoa 视频与 Metal renderer 的构建，保留 OpenCV 图像和视频读写。
- Archive 后、export 前扫描整个 app 的所有 Mach-O，拒绝审核清单中的 47 个 API 及另发现的 5 个 Tk 私有名称；原拒审包必须被该检查拒绝，修复产物必须通过。保留 network.server，并在审核说明中解释所有仅绑定 127.0.0.1 的入站 HTTP 服务及 child sandbox 继承用途。
