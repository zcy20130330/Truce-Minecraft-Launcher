/**
 * Mock 后端：Rust 核心就绪前，前端用它开发/演示/测试。
 * 所有方法带 100~600ms 模拟延迟，任务进度会真实跳动。
 */
import type {
  Account,
  DownloadTask,
  GameVersion,
  Instance,
  LauncherApi,
  LauncherSettings,
  LoaderType,
  MemoryInfo,
  MemoryOptimizeResult,
  SystemInfo,
} from "./types";

const delay = (ms: number) => new Promise((r) => setTimeout(r, ms));

const instances: Instance[] = [
  {
    id: "inst-1",
    name: "生存主世界",
    gameVersion: "26.2",
    loader: "fabric",
    loaderVersion: "0.18.3",
    lastPlayed: new Date(Date.now() - 3600_000).toISOString(),
    createdTime: "2026-10-01T10:00:00Z",
    playTimeSeconds: 104 * 3600 + 2137,
    modCount: 42,
  },
  {
    id: "inst-2",
    name: "跑酷测试",
    gameVersion: "26.2",
    loader: "vanilla",
    lastPlayed: new Date(Date.now() - 86400_000 * 2).toISOString(),
    createdTime: "2026-09-15T08:30:00Z",
    playTimeSeconds: 12 * 3600,
    modCount: 3,
  },
  {
    id: "inst-3",
    name: "1.21.1 养老",
    gameVersion: "1.21.1",
    loader: "forge",
    loaderVersion: "52.0.40",
    lastPlayed: undefined,
    createdTime: "2026-08-20T14:00:00Z",
    playTimeSeconds: 3 * 3600,
    modCount: 87,
  },
];

const accounts: Account[] = [
  { id: "acc-1", username: "Tracgiture", uuid: "069a79f4-44e9-4726-a5be-fca90e38aaf5", type: "offline" },
  { id: "acc-2", username: "Caeliana", uuid: "8a7e4a2c-1b3d-4e5f-9a6b-0c1d2e3f4a5b", type: "offline" },
];

const tasks: DownloadTask[] = [
  {
    id: "task-1",
    name: "Fabric API 0.118.5 (生存主世界)",
    status: "downloading",
    progress: 0.62,
    speedBytesPerSec: 3_200_000,
    downloadedBytes: 2_100_000,
    totalBytes: 3_400_000,
    instanceId: "inst-1",
  },
  {
    id: "task-2",
    name: "Minecraft 26.2 (跑酷测试)",
    status: "pending",
    progress: 0,
    speedBytesPerSec: 0,
    downloadedBytes: 0,
    totalBytes: 48_000_000,
    instanceId: "inst-2",
  },
];

const gameVersions: GameVersion[] = [
  { id: "26.3", type: "release", releaseTime: "2026-09-30T00:00:00Z" },
  { id: "26.2", type: "release", releaseTime: "2026-06-17T00:00:00Z" },
  { id: "26.1.2", type: "release", releaseTime: "2026-04-08T00:00:00Z" },
  { id: "1.21.11", type: "release", releaseTime: "2025-11-11T00:00:00Z" },
  { id: "1.21.4", type: "release", releaseTime: "2024-12-03T00:00:00Z" },
  { id: "1.21.1", type: "release", releaseTime: "2024-08-08T00:00:00Z" },
  { id: "1.20.1", type: "release", releaseTime: "2023-06-07T00:00:00Z" },
];

const loaderVersions: Record<LoaderType, string[]> = {
  vanilla: [],
  fabric: ["0.18.3", "0.17.2", "0.16.10"],
  forge: ["52.0.40", "51.1.3", "49.2.0"],
  neoforge: ["21.4.100", "21.1.200"],
  quilt: ["0.26.10", "0.25.4"],
};

const settings: LauncherSettings = {
  theme: "dark",
  language: "zh-CN",
  gameLanguage: "system",
  maxMemoryMB: 4096,
  autoMemory: true,
  memoryReserveMB: 1024,
  memoryOptimize: true,
  downloadThreads: 12,
  closeOnLaunch: false,
};

// 可变的模拟内存状态，optimizeMemory 会真实改写它
const TOTAL_MB = 16384;
let availableMB = 3820; // 故意留成「偏紧」的数值，便于演示低内存场景

const systemInfo: SystemInfo = {
  totalMemoryMB: TOTAL_MB,
  availableMemoryMB: availableMB,
  suggestedMemoryMB: 6144,
  os: "windows",
  arch: "x86_64",
};

function buildMemoryInfo(): MemoryInfo {
  const need = (settings.autoMemory ? systemInfo.suggestedMemoryMB : settings.maxMemoryMB)
    + settings.memoryReserveMB;
  const available = availableMB;
  const ratio = available / TOTAL_MB;
  const needed = available < need || ratio < 0.2;
  return {
    totalMB: TOTAL_MB,
    availableMB: available,
    suggestedMB: systemInfo.suggestedMemoryMB,
    pressure: {
      needed,
      availableMB: available,
      totalMB: TOTAL_MB,
      requiredMB: need,
      reason: available < need
        ? `可用 ${available} MB 低于本次所需 ${need} MB`
        : ratio < 0.2
          ? `可用比例仅 ${Math.round(ratio * 100)}%，低于 20%`
          : "内存充足",
    },
  };
}

/** 模拟内存随时间缓慢下降（模拟后台程序吃内存） */
setInterval(() => {
  if (availableMB > 1200) availableMB -= 1;
}, 5000);

/** 模拟任务进度跳动，用于演示下载页 */
export function startMockTaskTicker(emit: (t: DownloadTask) => void) {
  const timer = setInterval(() => {
    for (const t of tasks) {
      if (t.status === "downloading" && t.progress < 1) {
        t.progress = Math.min(1, t.progress + 0.03 + Math.random() * 0.05);
        t.downloadedBytes = Math.floor(t.totalBytes * t.progress);
        t.speedBytesPerSec = 2_800_000 + Math.random() * 900_000;
        if (t.progress >= 1) t.status = "completed";
        emit({ ...t });
      }
    }
  }, 800);
  return () => clearInterval(timer);
}

export const mockApi: LauncherApi = {
  async listInstances() {
    await delay(200);
    return instances.map((i) => ({ ...i }));
  },
  async createInstance(input) {
    await delay(500);
    const inst: Instance = {
      id: `inst-${Date.now()}`,
      name: input.name,
      gameVersion: input.gameVersion,
      loader: input.loader,
      loaderVersion: input.loaderVersion,
      createdTime: new Date().toISOString(),
      playTimeSeconds: 0,
      modCount: 0,
    };
    instances.push(inst);
    return inst;
  },
  async deleteInstance(id) {
    await delay(300);
    const i = instances.findIndex((x) => x.id === id);
    if (i >= 0) instances.splice(i, 1);
  },
  async renameInstance(id, name) {
    await delay(200);
    const inst = instances.find((x) => x.id === id);
    if (inst) inst.name = name;
  },
  async launchInstance() {
    await delay(1200);
    throw new Error("mock 模式不能真启动游戏，等 Rust 核心接入");
  },

  async listAccounts() {
    await delay(150);
    return accounts.map((a) => ({ ...a }));
  },
  async addMicrosoftAccount() {
    await delay(2000);
    throw new Error("mock 模式不支持微软登录");
  },
  async addOfflineAccount(username) {
    await delay(400);
    const acc: Account = {
      id: `acc-${Date.now()}`,
      username,
      uuid: crypto.randomUUID(),
      type: "offline",
    };
    accounts.push(acc);
    return acc;
  },
  async removeAccount(id) {
    await delay(200);
    const i = accounts.findIndex((a) => a.id === id);
    if (i >= 0) accounts.splice(i, 1);
  },

  async listGameVersions() {
    await delay(300);
    return gameVersions;
  },
  async listLoaderVersions(_gv, loader) {
    await delay(200);
    return loaderVersions[loader] ?? [];
  },

  async listTasks() {
    await delay(150);
    return tasks.map((t) => ({ ...t }));
  },
  async pauseTask(id) {
    const t = tasks.find((x) => x.id === id);
    if (t) t.status = "paused";
  },
  async resumeTask(id) {
    const t = tasks.find((x) => x.id === id);
    if (t) t.status = "downloading";
  },
  async cancelTask(id) {
    const i = tasks.findIndex((x) => x.id === id);
    if (i >= 0) tasks.splice(i, 1);
  },
  async retryTask(id) {
    const t = tasks.find((x) => x.id === id);
    if (t) {
      t.status = "downloading";
      t.progress = 0;
      t.error = undefined;
    }
  },

  async getSettings() {
    await delay(100);
    return { ...settings };
  },
  async setSettings(patch) {
    await delay(150);
    Object.assign(settings, patch);
  },
  async getSystemInfo() {
    await delay(100);
    systemInfo.availableMemoryMB = availableMB;
    return systemInfo;
  },

  async getMemoryInfo() {
    await delay(120);
    return buildMemoryInfo();
  },
  async optimizeMemory(opts) {
    await delay(900);
    const before = availableMB;
    const steps = [
      { name: "清空进程工作集", ok: true, detail: "成功", freedMB: 0 },
      { name: "回写修改页列表", ok: true, detail: "成功", freedMB: 0 },
      { name: "清除待机列表", ok: true, detail: "成功", freedMB: 0 },
      { name: "清除低优先级待机列表", ok: true, detail: "成功", freedMB: 0 },
    ];
    if (opts?.perProcess !== false) {
      steps.push({ name: "逐进程修剪工作集", ok: true, detail: "18/23 个进程", freedMB: 0 });
    }
    // 释放量：清掉待机列表的大头（随机 900~2200MB），上限不超过「已用」的量
    const freed = Math.min(TOTAL_MB - before, 900 + Math.floor(Math.random() * 1300));
    availableMB = before + freed;
    systemInfo.availableMemoryMB = availableMB;
    steps[2].freedMB = freed;
    return {
      ok: true,
      admin: true,
      beforeMB: before,
      afterMB: availableMB,
      freedMB: freed,
      steps,
    } satisfies MemoryOptimizeResult;
  },

  async onTaskProgress(cb) {
    const stop = startMockTaskTicker(cb);
    return stop;
  },
  async onLaunchState(cb) {
    void cb;
    return () => {};
  },
};
