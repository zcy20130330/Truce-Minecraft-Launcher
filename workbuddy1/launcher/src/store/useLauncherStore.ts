import { create } from "zustand";
import { api } from "@/api/client";
import type {
  Account,
  DownloadTask,
  Instance,
  LauncherSettings,
  MemoryInfo,
  MemoryOptimizeResult,
  SystemInfo,
} from "@/api/types";

interface LauncherState {
  instances: Instance[];
  accounts: Account[];
  tasks: DownloadTask[];
  settings: LauncherSettings | null;
  systemInfo: SystemInfo | null;
  memoryInfo: MemoryInfo | null;
  optimizing: boolean;
  lastOptimize: MemoryOptimizeResult | null;
  optimizeError: string | null;
  selectedAccountId: string | null;
  loading: boolean;
  error: string | null;

  refreshAll: () => Promise<void>;
  setSelectedAccount: (id: string) => void;
  addOfflineAccount: (username: string) => Promise<void>;
  removeAccount: (id: string) => Promise<void>;
  createInstance: (input: { name: string; gameVersion: string; loader: Instance["loader"] }) => Promise<void>;
  deleteInstance: (id: string) => Promise<void>;
  launchInstance: (id: string) => Promise<void>;
  updateSettings: (patch: Partial<LauncherSettings>) => Promise<void>;
  upsertTask: (task: DownloadTask) => void;
  loadMemoryInfo: () => Promise<void>;
  optimizeMemory: (perProcess?: boolean) => Promise<MemoryOptimizeResult | null>;
}

export const useLauncherStore = create<LauncherState>((set, get) => ({
  instances: [],
  accounts: [],
  tasks: [],
  settings: null,
  systemInfo: null,
  memoryInfo: null,
  optimizing: false,
  lastOptimize: null,
  optimizeError: null,
  selectedAccountId: null,
  loading: false,
  error: null,

  refreshAll: async () => {
    set({ loading: true, error: null });
    try {
      const [instances, accounts, tasks, settings, systemInfo, memoryInfo] = await Promise.all([
        api.listInstances(),
        api.listAccounts(),
        api.listTasks(),
        api.getSettings(),
        api.getSystemInfo(),
        api.getMemoryInfo(),
      ]);
      set({
        instances,
        accounts,
        tasks,
        settings,
        systemInfo,
        memoryInfo,
        selectedAccountId: get().selectedAccountId ?? accounts[0]?.id ?? null,
        loading: false,
      });
    } catch (e) {
      set({ error: e instanceof Error ? e.message : String(e), loading: false });
    }
  },

  setSelectedAccount: (id) => set({ selectedAccountId: id }),

  addOfflineAccount: async (username) => {
    const acc = await api.addOfflineAccount(username);
    set((s) => ({ accounts: [...s.accounts, acc] }));
  },

  removeAccount: async (id) => {
    await api.removeAccount(id);
    set((s) => ({ accounts: s.accounts.filter((a) => a.id !== id) }));
  },

  createInstance: async (input) => {
    await api.createInstance(input);
    set({ instances: await api.listInstances() });
  },

  deleteInstance: async (id) => {
    await api.deleteInstance(id);
    set((s) => ({ instances: s.instances.filter((i) => i.id !== id) }));
  },

  launchInstance: async (id) => {
    const accountId = get().selectedAccountId;
    if (!accountId) throw new Error("请先选择一个账户");
    await api.launchInstance({ instanceId: id, accountId });
  },

  updateSettings: async (patch) => {
    await api.setSettings(patch);
    set((s) => ({ settings: s.settings ? { ...s.settings, ...patch } : null }));
    // 内存相关设置会影响压力判定，刷新一次
    if (patch.autoMemory !== undefined || patch.maxMemoryMB !== undefined || patch.memoryReserveMB !== undefined) {
      await get().loadMemoryInfo();
    }
  },

  loadMemoryInfo: async () => {
    try {
      set({ memoryInfo: await api.getMemoryInfo() });
    } catch (e) {
      set({ error: e instanceof Error ? e.message : String(e) });
    }
  },

  optimizeMemory: async (perProcess = true) => {
    set({ optimizing: true, optimizeError: null });
    try {
      const res = await api.optimizeMemory({ perProcess });
      set({ lastOptimize: res });
      await get().loadMemoryInfo();
      return res;
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      set({ optimizeError: msg });
      return null;
    } finally {
      set({ optimizing: false });
    }
  },

  upsertTask: (task) =>
    set((s) => {
      const i = s.tasks.findIndex((t) => t.id === task.id);
      if (i < 0) return { tasks: [...s.tasks, task] };
      const tasks = [...s.tasks];
      tasks[i] = task;
      return { tasks };
    }),
}));
