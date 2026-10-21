/**
 * ============================================================
 * 前后端 API 契约（Contract）
 * ============================================================
 * 这里是前端与 Rust 核心的唯一数据契约。
 * Rust 侧（src-tauri/src/**）必须实现同样结构的 Tauri 命令，
 * 字段名用 camelCase（Tauri 2 默认 serde rename_all = "camelCase"）。
 *
 * 命令清单见文末 COMMANDS 常量——Rust 侧用
 * tauri::generate_handler![...] 注册同名命令即可。
 */

// ---------- 领域模型 ----------

export type LoaderType = "vanilla" | "fabric" | "forge" | "neoforge" | "quilt";

export interface Instance {
  id: string;
  name: string;
  gameVersion: string;
  loader: LoaderType;
  loaderVersion?: string;
  iconPath?: string;
  lastPlayed?: string; // ISO 8601
  createdTime: string; // ISO 8601
  playTimeSeconds: number;
  modCount: number;
  notes?: string;
}

export type AccountType = "microsoft" | "offline" | "thirdparty";

export interface Account {
  id: string;
  username: string;
  uuid: string;
  type: AccountType;
  accessToken?: string;
  skins?: { url: string; model: "slim" | "classic" } | null;
}

export interface GameVersion {
  id: string; // 如 "1.21.4" / "26.2"
  type: "release" | "snapshot" | "old_beta" | "old_alpha";
  releaseTime: string;
}

export type TaskStatus = "pending" | "downloading" | "paused" | "completed" | "failed" | "cancelled";

export interface DownloadTask {
  id: string;
  name: string;
  status: TaskStatus;
  progress: number; // 0..1
  speedBytesPerSec: number;
  downloadedBytes: number;
  totalBytes: number;
  error?: string;
  instanceId?: string;
}

export interface LauncherSettings {
  theme: "dark" | "light";
  /** 启动器界面语言 */
  language: "zh-CN" | "en-US";
  /** 游戏内语言（写入实例 options.txt 的 lang 项） */
  gameLanguage: GameLanguage;
  maxMemoryMB: number;
  autoMemory: boolean;
  /** 除游戏外留给系统的余量，用于判断是否需要优化 */
  memoryReserveMB: number;
  /** 内存不足时自动触发内存优化 */
  memoryOptimize: boolean;
  javaPath?: string;
  downloadThreads: number;
  closeOnLaunch: boolean;
}

export interface LaunchOptions {
  instanceId: string;
  accountId: string;
}

export interface SystemInfo {
  totalMemoryMB: number;
  /** 立即可用的物理内存（Windows 上即 GlobalMemoryStatusEx 的 ullAvailPhys） */
  availableMemoryMB: number;
  suggestedMemoryMB: number;
  os: string;
  arch: string;
}

/** 游戏内语言资源名（写入 options.txt 的 lang 项） */
export type GameLanguage = "system" | "zh_cn" | "zh_tw" | "en_us" | "ja_jp" | "ko_kr" | "none";

/** 内存压力判定结果：是否满足「需要优化」 */
export interface MemoryPressure {
  needed: boolean;
  reason: string;
  availableMB: number;
  totalMB: number;
  /** 本次所需 = Xmx + 系统预留 */
  requiredMB: number;
}

export interface MemoryInfo {
  totalMB: number;
  availableMB: number;
  suggestedMB: number;
  pressure: MemoryPressure;
}

/** 单个优化动作的执行结果（对标 RamMap：清工作集 / 清待机列表等） */
export interface MemoryOptimizeStep {
  name: string;
  ok: boolean;
  detail: string;
  freedMB: number;
}

export interface MemoryOptimizeResult {
  ok: boolean;
  /** 是否具备管理员权限——多数优化动作需要它才生效 */
  admin: boolean;
  beforeMB: number;
  afterMB: number;
  freedMB: number;
  steps: MemoryOptimizeStep[];
}

// ---------- 命令签名 ----------

export interface LauncherApi {
  // 实例
  listInstances(): Promise<Instance[]>;
  createInstance(input: {
    name: string;
    gameVersion: string;
    loader: LoaderType;
    loaderVersion?: string;
  }): Promise<Instance>;
  deleteInstance(id: string): Promise<void>;
  renameInstance(id: string, name: string): Promise<void>;
  launchInstance(opts: LaunchOptions): Promise<void>;

  // 账户
  listAccounts(): Promise<Account[]>;
  addMicrosoftAccount(): Promise<Account>; // 设备码流，Rust 侧打开浏览器
  addOfflineAccount(username: string): Promise<Account>;
  removeAccount(id: string): Promise<void>;

  // 版本与资源
  listGameVersions(): Promise<GameVersion[]>;
  listLoaderVersions(gameVersion: string, loader: LoaderType): Promise<string[]>;

  // 下载
  listTasks(): Promise<DownloadTask[]>;
  pauseTask(id: string): Promise<void>;
  resumeTask(id: string): Promise<void>;
  cancelTask(id: string): Promise<void>;
  retryTask(id: string): Promise<void>;

  // 设置与系统
  getSettings(): Promise<LauncherSettings>;
  setSettings(patch: Partial<LauncherSettings>): Promise<void>;
  getSystemInfo(): Promise<SystemInfo>;

  // 内存：自动分配的依据 + 低内存优化
  getMemoryInfo(): Promise<MemoryInfo>;
  optimizeMemory(opts?: { perProcess?: boolean }): Promise<MemoryOptimizeResult>;

  // 事件订阅（Rust 侧 emit）
  onTaskProgress(cb: (task: DownloadTask) => void): Promise<() => void>;
  onLaunchState(cb: (state: { instanceId: string; running: boolean }) => void): Promise<() => void>;
}

/**
 * Rust 侧需要注册的 Tauri 命令清单（generate_handler）：
 * list_instances / create_instance / delete_instance / rename_instance / launch_instance
 * list_accounts / add_microsoft_account / add_offline_account / remove_account
 * list_game_versions / list_loader_versions
 * list_tasks / pause_task / resume_task / cancel_task / retry_task
 * get_settings / set_settings / get_system_info
 * get_memory_info / optimize_memory
 * 事件：task-progress / launch-state
 */
export const COMMANDS = [
  "list_instances",
  "create_instance",
  "delete_instance",
  "rename_instance",
  "launch_instance",
  "list_accounts",
  "add_microsoft_account",
  "add_offline_account",
  "remove_account",
  "list_game_versions",
  "list_loader_versions",
  "list_tasks",
  "pause_task",
  "resume_task",
  "cancel_task",
  "retry_task",
  "get_settings",
  "set_settings",
  "get_system_info",
  "get_memory_info",
  "optimize_memory",
] as const;
