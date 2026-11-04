# aihubkit Changelog

All notable public changes to the aihubkit distribution of BeefTV are documented in this file.

## Unreleased

## v1.0.9

- 按运营域名清单批量发布独立贴牌客户端，应用名称使用平台系统名称加 TV，安装图标、安装标识、数据目录及下载文件按品牌隔离。
- 安装包内嵌品牌快照，登录前展示对应名称和 logo；运行中刷新官网、微信二维码及品牌信息，版本号改为带客户端名的纯文本。
- 仅品牌查询使用绑定专属域名；账号、额度、模型目录、上传和 AI 生成统一使用 `/api/status` 返回的 `api_server_address`。
- 统一账号、素材库、模型分组、协议、MCP 与客户端文案；平台模式隐藏官方算力渠道行及捐赠、GitHub、X 入口，保留官网和微信二维码。
- 发布预检冻结所有品牌信息，全部品牌和桌面目标通过检查后才公开 Release；保留官方本地模式及上游插件、模型协议和资源流程。

## v1.0.8

### 上游同步范围

完整合并官方 `upstream/main` 至 `ad08ee05`，官方版本为 `v2.1.5`。相对于同步前提交 `655d2bd6`，补入 74 个上游提交（包含合并提交）；共同祖先为 `7bddff61`。下列清单覆盖整个同步区间，包括在此前发行版中已有部分等价修复的提交。

### 功能与修复

- 插件生态：加入插件市场、SDK、示例与开发文档；支持 npm 插件安装、生命周期、热加载、异常隔离、画布节点、技能同步、UI 插槽与事件总线。增加国内镜像、多源安装、远程 README 预览，以及下载地址、SHA-256 和解压边界校验。
- 模型与创作插件：内置 LL2 自带 Key 渠道、视频协议和快捷服务商预置，完善 9 个模型、6 类协议的配置与调用；增加角色替换工作室、两阶段创作、时间线拼接和画布编排，修复菜单重复与插件禁用后的节点占位。
- 画布与分镜：支持手动创建分镜表、Alt 拖拽复制、自定义单画布容量上限和 IndexedDB 封面存储；改善大画布保存、自愈、轮询冲突、项目备份容错和导出性能，并修复事务死锁。
- 视频生成：完善首尾帧与全能参考模式、缩略图帧角色、单首帧模型能力判断；修复 Seedance 2.5 样片转高清、尾帧开关、二创选模、绑定状态和 Seedance 2.0 fast/mini 分辨率，改善视频兼容预览、比例适配及重试反馈。
- 图片、音频与素材：Seedream 默认关闭水印；个人素材库支持 MP3 和其他音频上传，减少素材库与宿主耦合；压缩媒体概览预览图、裁剪已消费的历史工具图片，优化宫格拆分连接线与提示词面板行为。
- 费用与助手：官方算力按服务端公开价格显示预计费用，完善模型目录和分镜估价、助手确认卡排版及报价口径；压缩系统提示词、移除助手工具中的插件内部字段，保持请求体积限制。
- 网络与错误：支持本地部署和 TUN/Fake-IP 网络，调整媒体 URL 出站策略，避免非视频任务预上传；为模型发现 EOF 增加重试，修复 HTTPS 地址被当作本地路径；统一托管媒体探测和错误脱敏，兼容旧版 WebKit，生成错误卡展示脱敏后的上游详情。
- 界面与交互：新增左侧插件中心入口；技能市场标签栏吸顶并隐藏单页翻页按钮；修正分镜镜号搜索、空输入时的已生成提示词面板、项目菜单宽度、currentColor 背景回退，以及尚未取得任务 ID 时取消操作后的加载状态。
- 桌面发布与维护：修复更新保存锁死和下载低速检测时机，改进安装包打包并行、树哈希缓存、Linux CLI 冒烟测试及构建复用；同步 v2.1.3 至 v2.1.5 的官方发行记录、工程规则和测试，删除无调用方的旧托管管理代码。
- aihubkit 兼容与发布：保留账号、默认模型初始化、协议选择、上传偏好及记住账号密码能力；素材上传验证跟随官方出站策略。发版增加本地与 CI 双重校验，要求完整发布提交先进入远端 `main`，再创建不可变发布标签；官方版本文件保持原样，修正继承的验收回执测试，将官方回执与记录它的源码提交配对，避免与旧标签或 fork 源码错配。
- 验证环境：原生画布存档恢复测试显式选择本地模式，导演台浏览器测试的模拟后端明确返回集成未启用状态，保留登录校验和严格网络错误检查。

### 完整上游提交清单

| 提交 | 变更 |
| --- | --- |
| `4225091c` | fix(canvas): 补齐 Seedance 2.5 样片转高清、尾帧开关、视频二创选模与绑定状态重置 (#369) |
| `451edbbc` | fix(canvas): 视频兼容预览与项目容错备份导出及 v2.1.2 发版准备 (#380) |
| `f6cd49f5` | 优化媒体概览预览图体积并裁剪已消费的历史工具图片 (#382) |
| `9c901178` | feat(web): show sanitized upstream failure detail on generation error cards (#383) |
| `fd0f05a7` | fix(video,desktop): 清理参考模式残留帧角色、收敛 Seedance 2.0 fast/mini 分辨率并修复桌面更新保存锁死 |
| `1850b224` | fix(outbound): 恢复本地部署与 TUN/Fake-IP 策略并跳过非视频预上传 |
| `9703556f` | fix(outbound): 默认放开媒体 URL 的 IP 网段白名单限制 |
| `be1352f9` | fix(image): default Seedream watermark to false |
| `5cb65938` | feat(plugins): 插件生态 - 新增插件市场、SDK、示例插件与开发文档 |
| `7db43f29` | feat(agent-host): 插件宿主 - 支持单包扩展热加载、异常隔离与内置 npm |
| `5f199c2b` | feat(plugins): 插件系统 - 支持 npm 插件包生命周期、画布节点与技能同步 |
| `6f55deca` | feat(web): 插件宿主 - 实现全量 UI 插槽、事件总线与内置插件迁移 |
| `ebc32845` | feat(plugins): align host-package runtime seams across backend, agent-host, and web |
| `59635a0e` | test(bootstrap): include plugin-packages runtime module in desktop chain test |
| `4e2c7998` | fix(plugins): normalize executeTool reply and preserve disabled node placeholder |
| `8db13656` | refactor(plugins): 插件生态 - 评审修正：npm 下载归 Go，收紧插件宿主接口 |
| `d9dbe8ac` | fix(plugins): 插件生态 - 修复真实测试发现的市场、画布节点与插件管理问题 |
| `39262748` | 合并上游 PR #387 |
| `9529fabb` | fix(canvas): 提升大画布保存上限与自愈能力并消除轮询假冲突 |
| `e0a8f96f` | feat(canvas): 画布容量 - 开放单画布容量上限自定义与本地封面 IndexedDB 存储 |
| `e9384fa1` | perf(database): 大画布落盘 - 开启 GORM ParameterizedQueries 避免百兆画布慢日志全量转义阻塞 |
| `9f3b736d` | fix(canvas): address review findings for custom canvas capacity limit |
| `929972bc` | chore(release): 准备 v2.1.3 发布与测试基线更新 |
| `2556b57d` | fix(skills): 技能市场 - 吸顶保留标签栏并在单页时隐藏翻页按钮 |
| `a504922d` | feat(plugins): bundle built-in LL2 BYOK channel and video protocol plugin |
| `b032fbdb` | fix(plugins): verify all 9 LL2 models and 6 protocols end-to-end |
| `87b4be14` | fix(plugins): LL2 渠道插件 - 补齐 代码审查发现的渠道写入防护与协议校验 |
| `93eac28f` | refactor(backend): 托管遗留 - 删除管理员审计与无调用方的管理写操作 |
| `5056b344` | refactor(backend): 逻辑模型目录 - 删除剩余无调用方的管理构建与配置错误辅助函数 |
| `e5818cd5` | 合并上游 PR #388 |
| `5021af69` | fix(web): 插件宿主 - 删除渠道写入里不存在的 notes 字段，恢复类型检查 |
| `986df441` | fix(web): 生成错误脱敏 - 旧版 WebKit 下报错里的密钥没有被删掉 |
| `901e8bbd` | docs(web): 应用内更新日志 - 补齐 v2.1.3 条目 |
| `b4dc74ea` | test(web): 测试外壳 - 修复被前置失败掩盖的两个浏览器测试 |
| `092f8871` | 合并上游 PR #391 |
| `004910db` | feat(plugins): add beeftv-plugin-character-replacement two-stage studio and timeline splice |
| `d189a851` | chore(plugins): keep only beeftv-plugin-character-replacement in curated market registry |
| `816ab3d3` | feat(plugins): 插件市场 - 支持国内多级镜像加速、远程 README 预览与多源在线安装 |
| `48108565` | fix(plugins): 强化插件市场镜像出站校验、SHA-256 强制核验与解压边界 |
| `036b2c61` | 合并上游 PR #392 |
| `7e6d2326` | feat(pricing): 费用估算 - 官方算力按 BeefAPI 公开价格显示预计费用 |
| `a6454fd6` | fix(pricing): 费用估算 - 修正验收发现的价格读取、模型目录与分镜估价问题 |
| `084d28bf` | fix(generation): retry discovery EOFs and avoid mangling https URLs as local paths |
| `1eaa2e9f` | refactor(generation): 托管媒体探测与错误脱敏 - 收口至单点判断并对齐重构规则 |
| `7b91b9d5` | docs(agents): 工程约定 - 将防止屎山复核与素材报错边界设为交付必检项 |
| `cd5a59da` | docs(agents): 工程约定 - 解耦服务端 API 表述并移除上游特定名称 |
| `7c20c1be` | fix(pricing): 助手确认卡与报价口径 - 修复确认卡折行孤字并禁止助手自行换算未提案价格 |
| `a4664d9b` | 合并上游 PR #390 |
| `52344d87` | fix(app): 画布事务绑定 - 跳过根数据库连接池初始化避免单连接事务死锁 |
| `74359b3d` | fix(operations): 画布节点配置 - 收回助手工具 schema 中的插件内部字段并保持请求体积门禁 |
| `72a81470` | fix(desktopupdate): 桌面更新下载 - 仅在响应体传输阶段启动低速窗口检测并消除多源切换测试时序抖动 |
| `14641583` | fix(agent-host): 系统提示词 - 压缩固定提示词长度以保持请求体积门禁 |
| `0581e824` | 合并上游 PR #393 |
| `72ae77af` | feat(canvas): 重构视频节点首尾帧与全能参考模式切换及缩略图帧角色交互 |
| `3042ecf5` | fix(canvas): 按模型能力收敛模式切换器展示与单首帧模型判断 |
| `96d28994` | docs(release): 统一使用 BeefTV 官方算力表述并补充 v2.1.4 更新日志 |
| `babddba6` | 合并上游 PR #385 |
| `7413e36d` | chore(release): 准备 v2.1.4 发布与测试基线更新 |
| `1eae51fa` | fix(plugins): 修复 v2.1.4 真实创作测试发现的 LL2 与角色替换插件宿主交互问题 |
| `a5c2719d` | perf(ci): release-pipeline - parallelize installer packaging, R2/CNB verification, and memoize upgrade tree hashes (#394) |
| `3d7efe3a` | feat(web): 模型服务与导航 - 新增左侧插件中心入口与 LL2 快捷服务商预置并发布 v2.1.5 (#395) |
| `24f7ecad` | fix(ci): 桌面发版 - 修复 Linux CLI 冒烟测试 BrokenPipeError 竞态并等待安装包打包完成 (#396) |
| `b4281a42` | fix(ci): release-pipeline - restore serial R2/CNB verification and support reusing completed build runs (#397) |
| `8f5aca3e` | fix(canvas): 节点搜索 - 修正分镜镜号偏移 (#384) |
| `25bb063b` | fix(assets): support uploading mp3 and audio files to personal asset library |
| `42b840cd` | fix(canvas): keep generated node prompt panel on empty input, fit video ratio, and fix shot search |
| `f1a72576` | fix(ui): avoid LightningCSS currentColor background fallback and widen project menu |
| `6009b347` | fix(canvas): prevent grid split connection rail overlap and surface video retry fallback |
| `3c2ec83f` | fix(canvas): reset local loading node state when canceling before taskId assignment |
| `bc0f1fbb` | 合并上游 PR #398 |
| `4196de31` | test(assets): update upload accept assertion for audio files |
| `d7d1d8e4` | fix(plugins): 插件与模型渠道 - 修复角色替换工作室画布编排、菜单去重与 LL2 预置模型配置 |
| `b14c198a` | feat(canvas): 宿主核心轻量化体验 - 分镜表手动创建、Alt拖拽复制、资产库解耦与导出性能优化 (#399) |
| `ad08ee05` | test(canvas,models): align script node availability and custom reference video connection test |

## v1.0.7

- 修复通过 aihubkit 账号导入的视频模型无法设为默认的问题，各模型独立使用所选协议。
- 首次登录自动初始化默认文本、图片和视频模型；账号页展示已导入模型，并支持手动重新初始化。分组和协议支持搜索，分组显示名称、悬停显示备注。
- 增加「上传到 aihubkit 素材库」设置，可随时切换平台存储与 BeefTV 原有存储方式；仅首次登录默认勾选，后续按账号保留选择。登录即可使用账号功能，无需额外绑定。
- 桌面登录页支持「记住账号和密码」，加密保存在本机，下次自动填充并由用户点击登录；取消勾选可清除记忆。
- 改善参考素材上传的连接诊断与有限重试，修复 macOS WebKit 事件流未正确取消导致画布保存和生成请求阻塞的问题。
- 完善本机构建入口，统一准备依赖、生成插件包、检查、编译打包和更新应用，避免依赖旧工作区残留的产物。

## v1.0.6

- 修复 Linux 便携 ZIP 启动验收仍按默认 aihubkit 登录模式运行的问题；发布冒烟测试显式使用官方本地模式，产品启动仍默认进入 aihubkit 登录。

## v1.0.5

- 修复全平台安装验收在默认 aihubkit 登录模式下等待本地工作区而超时的问题；安装生命周期测试显式使用官方本地模式，产品启动仍默认进入 aihubkit 登录。
- 规范 aihubkit 独立发版：已推送的标签不删除、不移动、不复用，任何代码、配置或发行说明变更后均递增版本号发布。

## v1.0.4

- 延续 v1.0.3 的 aihubkit 账号体系、平台模型和通用临时文件存储能力。
- 将 aihubkit 与官方 BeefTV 的版本文件及发布流程完全分离，方便持续同步官方更新。
- 完善 Windows、Linux、macOS Intel 与 Apple Silicon 的全平台自动构建和发布流程。

## v1.0.3

- 接入 aihubkit 账号体系：桌面端启动后先登录，支持注册、账号隔离、额度与消费记录，并可从账号分组导入平台模型。
- 接入 aihubkit 通用临时文件存储：图片、视频、音频等参考素材在生成前自动上传并转换为公网 URL，无需用户额外配置 S3 存储桶。
- 平台能力集中在可插拔适配层，同时保留官方本地模式、Provider、Protocol 和自定义渠道，便于持续合并 BeefTV 上游的新画布、模型与任务能力。
- 修复开发模式下的登录跨域、后端启动等待和账号入口展示，完善托管凭据与参考素材上传的专项测试。

## v1.0.1

- 发布 aihubkit 独立版本线的首个桌面版本，发布版本为 `v1.0.1`，与上游 BeefTV 的版本号独立演进。
- 本版基于上游 BeefTV `v2.1.1`，提供 Windows x64、Ubuntu 24.04 x64、macOS Apple Silicon 和 Intel 安装包及便携包。
