# MC Launcher — 前端工程（React + Tauri 2）

> 团队角色 C（前端 + 测试 CI）的工程目录。Rust 核心（A/B 负责）就绪前，前端以 **Mock 模式**独立开发。

## 快速开始

```bash
pnpm install          # 或 npm install
pnpm dev              # 浏览器打开 http://localhost:5173（Mock 模式）
pnpm test             # 跑单元测试（vitest）
pnpm typecheck        # TypeScript 严格检查
pnpm build            # 生产构建
```

## 目录结构

```
src/
├── api/
│   ├── types.ts      # ★ 前后端数据契约（Rust 侧按此实现命令）
│   ├── mock.ts       # Mock 后端（含任务进度模拟器）
│   └── client.ts     # 自动切换 Tauri / Mock
├── store/            # zustand 全局状态
├── components/       # Layout（侧栏/顶栏）、InstanceCard、MemoryBar（内存条）
├── pages/            # 首页 / 实例 / 下载 / 设置
├── utils/            # 格式化工具（带单测）
└── styles/theme.css  # 深色主题（CSS 变量，可扩展浅色）
```

## 已实现的三项能力（自动内存 / 自动语言 / 内存优化）

| 能力 | 配置项（`LauncherSettings`） | 说明 |
|---|---|---|
| 自动分配内存 | `autoMemory` + `maxMemoryMB` | 开启后忽略手动值，按 `SystemInfo.suggestedMemoryMB` 决定 `-Xmx` |
| 游戏语言 | `gameLanguage` | 启动前写入实例 `options.txt` 的 `lang` 项；`system`=跟随系统，`none`=不干预 |
| 低内存自动优化 | `memoryOptimize` + `memoryReserveMB` | 启动前若可用内存低于「Xmx + 预留」或不足总量 20%，自动触发 `optimize_memory` |

界面入口：首页顶部内存卡（可用量 + 一键优化）、设置页「启动与性能」「游戏」两节。

## ★ 给 Rust 侧（A/B）的对接说明

1. **契约即文档**：`src/api/types.ts` 定义了全部领域模型与命令签名，文末 `COMMANDS` 常量列出 Rust 侧需用 `tauri::generate_handler!` 注册的 21 个命令。
2. **命名约定**：Tauri 2 默认 `rename_all = "camelCase"`，前端参数用对象传（如 `invoke("create_instance", { input })`）。
3. **事件**：后端用 `app.emit("task-progress", task)` / `app.emit("launch-state", state)` 推送，前端已订阅。
4. **接入步骤**：Rust 侧实现命令 → 在 `client.ts` 中 `isTauri()` 检测到 `__TAURI_INTERNALS__` 后自动切真实后端，Mock 即刻下线，UI 零改动。
5. **内存相关命令**（Rust 侧实现要点）：
   - `get_memory_info` → 返回 `MemoryInfo { totalMB, availableMB, suggestedMB, pressure }`。Windows 用
     `GlobalMemoryStatusEx` 的 `ullTotalPhys/ullAvailPhys`；`pressure` 由「Xmx + memoryReserveMB」与总量 20% 双阈值判定。
   - `optimize_memory { perProcess }` → 返回 `MemoryOptimizeResult`。按公开的 Windows API 自行实现即可：
     清进程工作集（`SetProcessWorkingSetSizeEx(-1,-1)`）与 `NtSetSystemInformation(SystemMemoryListInformation=80)`
     的 EmptyWorkingSets / FlushModifiedList / PurgeStandbyList / PurgeLowPriorityStandbyList。
     **多数动作需要管理员权限**，`admin` 字段要如实返回，前端据此提示用户。

## 测试策略

- `utils/format.test.ts` — 纯函数单测（字节/时长/百分比/内存 MB 格式化）
- `components/InstanceCard.test.tsx` — 组件交互测试（渲染/启动/删除/重命名）
- `pages/SettingsPage.test.tsx` — 内存与游戏语言设置测试（一键优化/开关回写/语言切换/自动分配禁用态）
- CI（`.github/workflows/ci.yml`）：push/PR 自动跑 typecheck + test + build；Rust 岗位留好模板，取消注释即用

## UI 预览（Mock 模式）

`pnpm dev` 后可见：首页（大启动按钮 + 最近实例 + 内存状态卡，含一键优化）、实例管理（卡片网格 + 新建表单）、下载管理（带进度跳动的任务列表）、设置（账户/内存/自动优化/游戏语言/下载线程）。所有交互已就绪，等 Rust 数据接入；Mock 下「立即优化」会真实扣/放模拟内存数字，便于演示低内存场景。
