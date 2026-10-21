# Truce Minecraft Launcher

一个正在开发中的自研 Minecraft 启动器。仓库包含两部分：**可运行的 Python 原型**（已验证真实启动链路）与 **React + Tauri 2 前端工程**（Mock 模式，等 Rust 核心接入）。

## 仓库结构

```
Truce-Minecraft-Launcher/
├── prototype/              Python 原型（POC，零第三方依赖，能真实启动游戏）
│   ├── main.py             本地 Web 服务 + REST API（127.0.0.1:8766）
│   ├── launcher/           启动核心：版本解析 / 下载补全 / Java 检测 / 内存优化 / 语言
│   └── web/                原生 HTML+JS 深色 UI 面板
│
└── workbuddy1/launcher/    React 19 + Tauri 2 前端工程（团队角色 C）
    ├── src/api/types.ts    ★ 前后端契约：领域模型 + 21 个待实现的 Tauri 命令
    ├── src/pages/          首页 / 实例 / 下载 / 设置
    └── src-tauri/          （待 A/B 角色实现：Rust 核心）
```

各部分的详细说明见各自的 README：
- [`prototype/README.md`](prototype/README.md) — 原型的功能清单、目录说明、已知限制
- [`workbuddy1/launcher/README.md`](workbuddy1/launcher/README.md) — 前端工程结构、命令契约、测试说明

## 当前进度

| 模块 | 状态 |
|---|---|
| 版本扫描 / 解析 `inheritsFrom` 继承 | ✅ 已实现（原版 / Forge / Fabric / NeoForge / Quilt / OptiFine / LiteLoader） |
| 库与资源补全（镜像 + SHA1 校验）、natives 解压 | ✅ 已实现 |
| Java 检测与按版本需求自动选择 | ✅ 已实现 |
| 真实启动链路（JVM 参数拼装 → 拉起 java → 日志） | ✅ **已实测通过**（原版 1.21.1 与 Fabric 1.21.1 均进主菜单） |
| 自动内存分配 / 游戏语言自动设置 | ✅ 已实现并实测 |
| 低内存自动优化（清工作集 / 待机列表） | ✅ 原型已实现（正式 ABI 待对齐） |
| 离线登录 | ✅ 已实现 |
| 微软正版登录 | 🚧 原型占位，未实现 |
| 版本安装 / Mod 管理 | 🚧 未开始 |
| **Rust 核心（Tauri 后端）** | 🚧 **待 A/B 角色实现，是接入真实数据的关键路径** |
| React 前端 UI | ✅ 四页面已完成，Mock 模式可独立开发 |

前端 `types.ts` 是完整的双向契约：Rust 侧按 `COMMANDS` 常量实现 21 个命令后，`client.ts` 会自动切换到真实后端，UI 无需改动。

## 快速开始

### 跑原型（可直接真启动 MC）

```bash
cd prototype
python main.py          # 或 py -3.12 main.py
```

浏览器打开 http://127.0.0.1:8766 → 选择版本 → 启动。

首次运行会把本机默认 `.minecraft` 路径写入 `config.json`（该文件含本机路径，**不会提交**）。模板见 [`prototype/config.example.json`](prototype/config.example.json)。

### 跑前端（Mock 模式）

```bash
cd workbuddy1/launcher
npm install
npm run dev             # http://localhost:5173
```

## 关于第三方参考源码

本项目在开发过程中参考过 PCL-CE、SJMCL、Modrinth 等同类的**设计思路**（交互结构、内存优化策略、Tauri 架构），但本仓库**不包含任何第三方源码**，也**未复制**其代码实现。

- 所有内存优化、语言探测、启动参数拼装逻辑均为**基于公开的操作系统文档接口自行实现**
- 参考项目的完整副本保存在本地 `workbuddy1/启动器源码分析/`（已通过 `.gitignore` 排除），仅供离线学习，不随仓库分发

## 许可

项目尚未选定开源许可证，当前默认为「保留所有权利」（All rights reserved）。
