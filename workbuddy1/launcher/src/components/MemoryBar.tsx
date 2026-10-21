import type { MemoryInfo } from "@/api/types";
import { formatMemoryMB, formatPercent } from "@/utils/format";

/**
 * 内存占用条：显示已用/可用，内存紧张（pressure.needed）时变为警告配色。
 * 设置页与首页共用。
 */
export function MemoryBar({ info, height = 8 }: { info: MemoryInfo; height?: number }) {
  const usedMB = Math.max(0, info.totalMB - info.availableMB);
  const usedRatio = info.totalMB > 0 ? Math.min(1, usedMB / info.totalMB) : 0;

  return (
    <div className="mem-bar">
      <div className="progress-track" style={{ height }}>
        <div
          className={`mem-fill${info.pressure.needed ? " tight" : ""}`}
          style={{ width: `${Math.max(2, usedRatio * 100)}%` }}
        />
      </div>
      <div className="mem-bar-legend">
        <span>
          已用 {formatMemoryMB(usedMB)}（{formatPercent(usedRatio)}）
        </span>
        <span>
          可用 {formatMemoryMB(info.availableMB)} / {formatMemoryMB(info.totalMB)}
        </span>
      </div>
    </div>
  );
}
