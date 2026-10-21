import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { InstanceCard } from "@/components/InstanceCard";
import type { Instance } from "@/api/types";

const instance: Instance = {
  id: "t1",
  name: "生存主世界",
  gameVersion: "26.2",
  loader: "fabric",
  loaderVersion: "0.18.3",
  createdTime: "2026-10-01T10:00:00Z",
  playTimeSeconds: 375_000,
  modCount: 42,
  lastPlayed: new Date(Date.now() - 3600_000).toISOString(),
};

function setup(overrides: Partial<Parameters<typeof InstanceCard>[0]> = {}) {
  const props = {
    instance,
    onLaunch: vi.fn(),
    onDelete: vi.fn(),
    onRename: vi.fn(),
    ...overrides,
  };
  render(
    <MemoryRouter>
      <InstanceCard {...props} />
    </MemoryRouter>,
  );
  return props;
}

describe("InstanceCard", () => {
  it("渲染实例名称、版本与加载器", () => {
    setup();
    expect(screen.getByText("生存主世界")).toBeInTheDocument();
    expect(screen.getByText(/MC 26\.2 · Fabric 0\.18\.3/)).toBeInTheDocument();
  });

  it("渲染累计时长与 Mod 数", () => {
    setup();
    expect(screen.getByText(/104 小时/)).toBeInTheDocument();
    expect(screen.getByText(/42 个 Mod/)).toBeInTheDocument();
  });

  it("点击启动调用 onLaunch 并传入实例 id", async () => {
    const props = setup();
    await userEvent.click(screen.getByRole("button", { name: /启动/ }));
    expect(props.onLaunch).toHaveBeenCalledWith("t1");
  });

  it("点击删除调用 onDelete", async () => {
    const props = setup();
    await userEvent.click(screen.getByTitle("删除"));
    expect(props.onDelete).toHaveBeenCalledWith("t1");
  });

  it("双击重命名进入编辑态，回车提交", async () => {
    const props = setup();
    await userEvent.click(screen.getByTitle("重命名"));
    const input = screen.getByDisplayValue("生存主世界");
    await userEvent.clear(input);
    await userEvent.type(input, "新世界{Enter}");
    expect(props.onRename).toHaveBeenCalledWith("t1", "新世界");
  });
});
