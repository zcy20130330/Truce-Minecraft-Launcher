/**
 * API 客户端：自动在「Tauri 真实后端」与「Mock 后端」间切换。
 * - 跑在 Tauri WebView 里且有 __TAURI_INTERNALS__ → 走 invoke
 * - 浏览器里 pnpm dev 调试 → 走 mock
 */
import { invoke } from "@tauri-apps/api/core";
import { listen } from "@tauri-apps/api/event";
import type { DownloadTask, LauncherApi } from "./types";
import { COMMANDS } from "./types";
import { mockApi } from "./mock";

function isTauri(): boolean {
  return typeof window !== "undefined" && "__TAURI_INTERNALS__" in window;
}

const tauriApi: LauncherApi = {
  listInstances: () => invoke("list_instances"),
  createInstance: (input) => invoke("create_instance", { input }),
  deleteInstance: (id) => invoke("delete_instance", { id }),
  renameInstance: (id, name) => invoke("rename_instance", { id, name }),
  launchInstance: (opts) => invoke("launch_instance", { opts }),

  listAccounts: () => invoke("list_accounts"),
  addMicrosoftAccount: () => invoke("add_microsoft_account"),
  addOfflineAccount: (username) => invoke("add_offline_account", { username }),
  removeAccount: (id) => invoke("remove_account", { id }),

  listGameVersions: () => invoke("list_game_versions"),
  listLoaderVersions: (gameVersion, loader) => invoke("list_loader_versions", { gameVersion, loader }),

  listTasks: () => invoke("list_tasks"),
  pauseTask: (id) => invoke("pause_task", { id }),
  resumeTask: (id) => invoke("resume_task", { id }),
  cancelTask: (id) => invoke("cancel_task", { id }),
  retryTask: (id) => invoke("retry_task", { id }),

  getSettings: () => invoke("get_settings"),
  setSettings: (patch) => invoke("set_settings", { patch }),
  getSystemInfo: () => invoke("get_system_info"),

  getMemoryInfo: () => invoke("get_memory_info"),
  optimizeMemory: (opts) => invoke("optimize_memory", { opts }),

  onTaskProgress: (cb) =>
    listen<DownloadTask>("task-progress", (e) => cb(e.payload)).then(() => () => {}),
  onLaunchState: (cb) =>
    listen<{ instanceId: string; running: boolean }>("launch-state", (e) => cb(e.payload)).then(() => () => {}),
};

export const api: LauncherApi = isTauri() ? tauriApi : mockApi;
export const usingMock = !isTauri();
export { COMMANDS };
