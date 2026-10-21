import { useEffect } from "react";
import { Play, Plus, RefreshCw, MemoryStick, Sparkles, Loader2 } from "lucide-react";
import { useLauncherStore } from "@/store/useLauncherStore";
import { MemoryBar } from "@/components/MemoryBar";
import { formatRelativeTime, formatPlaytime, formatMemoryMB } from "@/utils/format";
import { useNavigate } from "react-router-dom";

export function HomePage() {
  const {
    instances,
    accounts,
    selectedAccountId,
    launchInstance,
    refreshAll,
    loading,
    memoryInfo,
    optimizing,
    optimizeMemory,
    lastOptimize,
  } = useLauncherStore();
  const navigate = useNavigate();
  const recent = [...instances].sort(
    (a, b) => new Date(b.lastPlayed ?? b.createdTime).getTime() - new Date(a.lastPlayed ?? a.createdTime).getTime(),
  )[0];

  useEffect(() => {
    void refreshAll();
  }, []);

  const launch = async (id: string) => {
    try {
      await launchInstance(id);
    } catch (e) {
      alert(e instanceof Error ? e.message : String(e));
    }
  };

  return (
    <div className="page col" style={{ gap: 16 }}>
      {/* 主启动区 */}
      <div className="card" style={{ display: "flex", alignItems: "center", gap: 18 }}>
        <div className="instance-icon" style={{ width: 56, height: 56 }}>
          <Play size={26} color="var(--accent)" />
        </div>
        <div className="grow">
          <div style={{ fontSize: 17, fontWeight: 700 }}>{recent ? recent.name : "还没有实例"}</div>
          <div className="text-tertiary" style={{ fontSize: 12, marginTop: 4 }}>
            {recent
              ? `MC ${recent.gameVersion} · ${recent.loader} · 上次游玩 ${formatRelativeTime(recent.lastPlayed)} · 累计 ${formatPlaytime(recent.playTimeSeconds)}`
              : "去「实例」页创建第一个实例吧"}
          </div>
        </div>
        <button
          className="btn btn-primary"
          style={{ padding: "10px 26px", fontSize: 14 }}
          disabled={!recent || !selectedAccountId}
          onClick={() => recent && launch(recent.id)}
        >
          <Play size={15} /> 开始游戏
        </button>
      </div>

      {/* 内存状况 */}
      {memoryInfo && (
        <div className="card" style={{ display: "flex", alignItems: "center", gap: 18 }}>
          <div className="mem-bar grow">
            <div className="row" style={{ marginBottom: 2 }}>
              <MemoryStick size={15} color={memoryInfo.pressure.needed ? "var(--warning)" : "var(--accent)"} />
              <span style={{ fontSize: 13, fontWeight: 600 }}>
                可用内存 {formatMemoryMB(memoryInfo.availableMB)}
              </span>
              {memoryInfo.pressure.needed ? (
                <span className="badge badge-warning">建议优化后启动</span>
              ) : (
                <span className="badge badge-accent">充足</span>
              )}
              <span className="grow" />
              <button className="btn btn-sm" disabled={optimizing} onClick={() => void optimizeMemory()}>
                {optimizing ? <Loader2 size={13} className="spin" /> : <Sparkles size={13} />}
                {optimizing ? "优化中…" : "一键优化"}
              </button>
            </div>
            <MemoryBar info={memoryInfo} height={6} />
          </div>
        </div>
      )}

      {lastOptimize && lastOptimize.freedMB > 0 && !optimizing && (
        <div className="text-tertiary" style={{ fontSize: 12, marginTop: -8 }}>
          上次优化释放 {formatMemoryMB(lastOptimize.freedMB)}
        </div>
      )}

      {/* 快捷入口 */}
      <div style={{ display: "flex", gap: 12 }}>
        <button className="btn" onClick={() => navigate("/instances")}>
          <Plus size={14} /> 新建实例
        </button>
        <button className="btn" onClick={() => navigate("/download")}>
          <RefreshCw size={14} /> 下载管理
        </button>
      </div>

      {/* 最近实例 */}
      <div>
        <div style={{ fontSize: 14, fontWeight: 600, marginBottom: 10 }}>最近游玩</div>
        {loading ? (
          <div className="text-tertiary">加载中…</div>
        ) : (
          <div className="instance-grid">
            {instances.slice(0, 4).map((i) => (
              <div key={i.id} className="card instance-card">
                <div className="instance-name">{i.name}</div>
                <div className="instance-meta">
                  MC {i.gameVersion} · {i.loader} · {formatRelativeTime(i.lastPlayed)}
                </div>
                <button className="btn btn-sm" style={{ marginTop: 8 }} onClick={() => launch(i.id)}>
                  <Play size={12} /> 启动
                </button>
              </div>
            ))}
          </div>
        )}
      </div>

      {!selectedAccountId && accounts.length === 0 && (
        <div className="card" style={{ borderColor: "var(--warning)" }}>
          <span className="badge badge-warning">提示</span>
          <span style={{ marginLeft: 10, fontSize: 13 }}>还没有账户，去设置页添加离线账户或登录微软账户</span>
        </div>
      )}
    </div>
  );
}
