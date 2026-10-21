import { useEffect } from "react";
import { Pause, Play, RotateCw, X, Download as DownloadIcon } from "lucide-react";
import { useLauncherStore } from "@/store/useLauncherStore";
import { api } from "@/api/client";
import { formatBytes, formatPercent, formatSpeed } from "@/utils/format";
import type { DownloadTask } from "@/api/types";

const statusLabels: Record<DownloadTask["status"], { text: string; cls: string }> = {
  pending: { text: "等待中", cls: "" },
  downloading: { text: "下载中", cls: "badge-info" },
  paused: { text: "已暂停", cls: "badge-warning" },
  completed: { text: "已完成", cls: "badge-accent" },
  failed: { text: "失败", cls: "badge-danger" },
  cancelled: { text: "已取消", cls: "" },
};

export function DownloadPage() {
  const { tasks, upsertTask } = useLauncherStore();

  useEffect(() => {
    // 订阅 Rust 侧 task-progress 事件（mock 模式由 ticker 驱动）
    let stop: (() => void) | undefined;
    api.onTaskProgress((task) => upsertTask(task)).then((fn) => {
      stop = fn;
    });
    return () => stop?.();
  }, [upsertTask]);

  const act = async (fn: () => Promise<void>) => {
    try {
      await fn();
    } catch (e) {
      alert(e instanceof Error ? e.message : String(e));
    }
  };

  return (
    <div className="page col" style={{ gap: 16 }}>
      <div className="row">
        <div className="grow" style={{ fontSize: 13 }}>
          共 <b>{tasks.length}</b> 个任务，{tasks.filter((t) => t.status === "downloading").length} 个进行中
        </div>
        <button className="btn btn-sm" onClick={() => useLauncherStore.getState().refreshAll()}>
          <RotateCw size={12} /> 刷新
        </button>
      </div>

      {tasks.length === 0 ? (
        <div className="empty-state">
          <DownloadIcon size={40} />
          <div>没有下载任务</div>
          <div style={{ fontSize: 12 }}>新建实例或安装 Mod 时会自动产生任务</div>
        </div>
      ) : (
        <div className="card" style={{ padding: 0, overflow: "hidden" }}>
          {tasks.map((t) => {
            const st = statusLabels[t.status];
            return (
              <div key={t.id} className="task-row">
                <div className="task-info">
                  <div className="task-name">{t.name}</div>
                  <div className="task-sub">
                    {formatBytes(t.downloadedBytes)} / {formatBytes(t.totalBytes)}
                    {t.status === "downloading" && ` · ${formatSpeed(t.speedBytesPerSec)}`}
                    {t.error && ` · ${t.error}`}
                  </div>
                  <div className="progress-track" style={{ marginTop: 8 }}>
                    <div
                      className={`progress-fill ${t.status === "paused" ? "paused" : t.status === "failed" ? "failed" : ""}`}
                      style={{ width: formatPercent(t.progress) }}
                    />
                  </div>
                </div>
                <span className={`badge ${st.cls}`}>{st.text}</span>
                <span className="mono" style={{ width: 44, textAlign: "right" }}>{formatPercent(t.progress)}</span>
                <div className="row" style={{ gap: 4 }}>
                  {t.status === "downloading" ? (
                    <button className="btn btn-sm btn-icon" title="暂停" onClick={() => act(() => api.pauseTask(t.id))}>
                      <Pause size={12} />
                    </button>
                  ) : t.status !== "completed" ? (
                    <button className="btn btn-sm btn-icon" title="继续/重试" onClick={() => act(() => api.retryTask(t.id))}>
                      <Play size={12} />
                    </button>
                  ) : null}
                  <button className="btn btn-sm btn-icon btn-danger" title="取消" onClick={() => act(() => api.cancelTask(t.id))}>
                    <X size={12} />
                  </button>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
