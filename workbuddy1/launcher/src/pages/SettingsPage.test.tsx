import { describe, it, expect, beforeAll } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { SettingsPage } from "@/pages/SettingsPage";
import { useLauncherStore } from "@/store/useLauncherStore";

/** 每项测试前把 store 拉到「已加载」状态（走 Mock 后端） */
async function boot() {
  await useLauncherStore.getState().refreshAll();
  render(
    <MemoryRouter>
      <SettingsPage />
    </MemoryRouter>,
  );
  await screen.findByText("内存状况");
}

beforeAll(async () => {
  await useLauncherStore.getState().refreshAll();
});

describe("SettingsPage 内存与游戏语言", () => {
  it("渲染内存状况卡：计划内存与预留需求", async () => {
    await boot();
    expect(screen.getByText("内存状况")).toBeInTheDocument();
    // Mock：autoMemory=true → 建议 6144MB，预留默认 1024MB → 合计 7.0 GB
    expect(screen.getByText("6.0 GB")).toBeInTheDocument();
    expect(screen.getByText("7.0 GB")).toBeInTheDocument();
  });

  it("点击「立即优化」后出现释放结果", async () => {
    await boot();
    await userEvent.click(screen.getByRole("button", { name: /立即优化/ }));
    await waitFor(() => expect(screen.getByText(/已释放/)).toBeInTheDocument(), { timeout: 5000 });
    // 优化后不再显示原有的「可用余额不足」提示，而是展示本次释放量
    expect(screen.getByText(/清空进程工作集/)).toBeInTheDocument();
  });

  it("切换「内存不足时自动优化」会写回设置", async () => {
    await boot();
    const row = screen.getByText("内存不足时自动优化").closest(".settings-row");
    expect(row).toBeTruthy();
    const sw = row!.querySelector(".switch")!;
    await userEvent.click(sw);
    // updateSettings 走 Mock 后端（约 150ms 延迟），需等待回写
    await waitFor(() => expect(useLauncherStore.getState().settings?.memoryOptimize).toBe(false));
  });

  it("游戏语言下拉可切换到指定语言", async () => {
    await boot();
    const select = screen.getByDisplayValue("跟随系统") as HTMLSelectElement;
    await userEvent.selectOptions(select, "zh_cn");
    await waitFor(() => expect(useLauncherStore.getState().settings?.gameLanguage).toBe("zh_cn"));
  });

  it("关闭自动分配后可手动修改最大内存", async () => {
    await boot();
    const row = screen.getByText("自动分配内存").closest(".settings-row")!;
    await userEvent.click(row.querySelector(".switch")!);
    await waitFor(() => expect(useLauncherStore.getState().settings?.autoMemory).toBe(false));
    const maxInput = screen.getByDisplayValue("4096") as HTMLInputElement;
    expect(maxInput.disabled).toBe(false);
  });
});
