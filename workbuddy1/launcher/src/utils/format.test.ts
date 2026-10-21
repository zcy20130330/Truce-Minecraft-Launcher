import { describe, it, expect } from "vitest";
import { formatBytes, formatSpeed, formatPlaytime, formatRelativeTime, formatPercent, formatMemoryMB } from "./format";

describe("formatBytes", () => {
  it("处理 0 和负数", () => {
    expect(formatBytes(0)).toBe("0 B");
    expect(formatBytes(-100)).toBe("0 B");
  });
  it("按 1024 进制换算", () => {
    expect(formatBytes(1024)).toBe("1.0 KB");
    expect(formatBytes(1024 * 1024 * 3.5)).toBe("3.5 MB");
    expect(formatBytes(1024 ** 3 * 2.25)).toBe("2.3 GB");
  });
  it("字节级不带小数", () => {
    expect(formatBytes(512)).toBe("512 B");
  });
});

describe("formatSpeed", () => {
  it("追加 /s 后缀", () => {
    expect(formatSpeed(3_200_000)).toBe("3.1 MB/s");
  });
});

describe("formatPlaytime", () => {
  it("未玩过", () => {
    expect(formatPlaytime(0)).toBe("未玩过");
  });
  it("分钟与小时", () => {
    expect(formatPlaytime(1800)).toBe("30 分钟");
    expect(formatPlaytime(375_000)).toBe("104 小时 10 分");
  });
});

describe("formatRelativeTime", () => {
  it("空值返回从未", () => {
    expect(formatRelativeTime(undefined)).toBe("从未");
  });
  it("刚刚", () => {
    expect(formatRelativeTime(new Date().toISOString())).toBe("刚刚");
  });
  it("小时前", () => {
    const twoHoursAgo = new Date(Date.now() - 2 * 3600_000).toISOString();
    expect(formatRelativeTime(twoHoursAgo)).toBe("2 小时前");
  });
});

describe("formatPercent", () => {
  it("钳制到 0~100", () => {
    expect(formatPercent(0.624)).toBe("62%");
    expect(formatPercent(1.5)).toBe("100%");
    expect(formatPercent(-0.2)).toBe("0%");
  });
});

describe("formatMemoryMB", () => {
  it("小于 1GB 用 MB 整数", () => {
    expect(formatMemoryMB(0)).toBe("0 MB");
    expect(formatMemoryMB(-5)).toBe("0 MB");
    expect(formatMemoryMB(768)).toBe("768 MB");
  });
  it("大于等于 1GB 用 GB 一位小数", () => {
    expect(formatMemoryMB(1024)).toBe("1.0 GB");
    expect(formatMemoryMB(6144)).toBe("6.0 GB");
    expect(formatMemoryMB(23911)).toBe("23.4 GB");
  });
  it("边界：1023MB 仍按 MB 显示", () => {
    expect(formatMemoryMB(1023)).toBe("1023 MB");
  });
});
