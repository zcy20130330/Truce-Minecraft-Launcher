# PCL-Prototype · MC 启动器原型

> 2026-10-20 | 团队启动包 v1.0 的**可运行原型（POC）** | 与 `../workbuddy1`（React Mock 前端）并行，互不影响

一个**能真实启动 Minecraft** 的启动器原型：解析版本 JSON → 补全库/资源 → 拼 JVM 参数 → 解压 natives → 拉起 java.exe → 实时日志。设置项对标 PCL 核心配置。

## 快速开始

```bash
python main.py
```

浏览器自动打开 http://127.0.0.1:8766

**零第三方依赖**（纯 Python 标准库，3.9+ 即可）。Windows 上建议显式指定 Python 3.12+，避免与旧版混淆：

```bash
py -3.12 main.py           # 推荐：py 启动器自动定位 3.12
# 或 python main.py        # 若默认 python 已是 3.9+
```

## 已实现功能

| 模块 | 说明 |
|---|---|
| 版本扫描 | 读取 MC 文件夹 `versions/`，解析版本 JSON，识别原版/Forge/Fabric/NeoForge/Quilt/OptiFine/LiteLoader |
| 版本继承 | `inheritsFrom` 递归合并（libraries/arguments 继承父版本） |
| 库文件补全 | 缺失才下载；官方源 + BMCLAPI 镜像；SHA1/大小校验 |
| 资源补全 | assets 索引 + objects 下载（virtual/legacy 布局） |
| natives 解压 | native 库 dll/so → `<版本>-natives/` |
| Java 检测 | 常见路径/JAVA_HOME/PATH 扫描 + 版本解析；按版本需求自动选择 |
| 自动内存 | 读取物理内存按档位推荐 -Xmx（≤ 物理内存 45%，512MB 下限），可关闭改手动 |
| 自动语言 | 启动前写 `options.txt` 的 `lang` 项：跟随系统 / 指定 / none 不干预 |
| 内存优化 | 内存不足时自动触发（RamMap 式：清进程工作集 + 清待机列表），并显示释放量 |
| 离线登录 | 离线 UUID 与 Java 端一致（服务器可识别） |
| 启动参数 | -Xmx、GC（自定义）、Log4j 防御、classpath、新版 arguments.game / 旧版 minecraftArguments、自动进服（QuickPlay/--server） |
| 实时日志 | 后台轮询拉取 + 崩溃关键字检测 |
| 设置项 | MC 文件夹 / 用户名 / 内存（自动 or 手动） / 语言 / 版本隔离 / Java / 窗口 / 自定义参数 / 自动进服 / 下载源 / 内存优化 |

## 架构

```
prototype/
├── main.py              # HTTP 服务入口（127.0.0.1:8766）+ 路由
├── launcher/
│   ├── config.py        # 配置读写（config.json）
│   ├── versions.py      # 版本扫描 / JSON 解析 / rules 过滤 / 继承合并
│   ├── java.py          # Java 检测与需求选择
│   ├── system.py        # 物理内存探测 / 自动内存推荐 / 系统 UI 语言探测
│   ├── ramopt.py        # 内存优化（RamMap 式清理）+ 低内存自动触发判定
│   ├── options.py       # 游戏内 options.txt 维护（自动写 lang）
│   ├── download.py      # 下载补全（镜像 + SHA1 校验）
│   ├── launch.py        # 参数拼装 / natives / 进程 / 日志缓冲
│   └── auth.py          # 离线登录（MS 六步占位）
└── web/                 # 前端（原生 HTML/CSS/JS，深色 PCL 风格）
```

## 与正式版（Rust+Tauri）的关系

- **本原型**：验证启动链路可行性 + 打磨 UI/交互细节，逻辑可直接作为 Rust 核心的"行为规格说明书"
- **workbuddy1/launcher**：C 角色 React 前端（Mock），后续接 Tauri+Rust
- 迁移时参考对应关系：`launch.py → launcher-core/launch`、`versions.py → launcher-core/instance`、`download.py → launcher-core/net`、`auth.py → launcher-core/account`、`system.py + options.py → launcher-core/system`（前者内存/语言探测，后者游戏内配置写入）

## 已知限制（原型范围）

- 微软正版登录未实现（离线可用，MS 走 `auth.py` 占位）
- 内存优化的多数动作需要管理员权限，非管理员运行时日志会提示但该步不生效（不影响启动）
- 版本下载（安装新版本）未实现——只启动已有版本
- 资源索引缺失时不自动下载索引文件（仅下载索引内 objects）
- 并发下载为单线程（`download_many` 预留 thread_count 参数）
- 不做 Forge/Fabric 安装器——加载器版本需已装好

## 里程碑对照

启动包 W5-6 的目标（"能启动原版游戏"）已在此原型中跑通，可提前演示。
