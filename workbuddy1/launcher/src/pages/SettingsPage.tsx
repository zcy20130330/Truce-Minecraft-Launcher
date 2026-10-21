import { useState } from "react";
import { UserPlus, Trash2, MemoryStick, Sparkles, Loader2 } from "lucide-react";
import { useLauncherStore } from "@/store/useLauncherStore";
import { MemoryBar } from "@/components/MemoryBar";
import { formatMemoryMB } from "@/utils/format";
import type { GameLanguage } from "@/api/types";

function Switch({ checked, onChange }: { checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <div className={`switch${checked ? " on" : ""}`} role="switch" aria-checked={checked} onClick={() => onChange(!checked)} />
  );
}

const GAME_LANGUAGES: { value: GameLanguage; label: string }[] = [
  { value: "system", label: "跟随系统" },
  { value: "zh_cn", label: "简体中文" },
  { value: "zh_tw", label: "繁體中文（台灣）" },
  { value: "en_us", label: "English" },
  { value: "ja_jp", label: "日本語" },
  { value: "ko_kr", label: "한국어" },
  { value: "none", label: "不修改（保持游戏内设置）" },
];

export function SettingsPage() {
  const {
    settings,
    systemInfo,
    memoryInfo,
    optimizing,
    lastOptimize,
    optimizeError,
    updateSettings,
    optimizeMemory,
    accounts,
    addOfflineAccount,
    removeAccount,
  } = useLauncherStore();
  const [newUser, setNewUser] = useState("");

  if (!settings) return <div className="page text-tertiary">加载中…</div>;

  const planMB = settings.autoMemory
    ? memoryInfo?.suggestedMB ?? systemInfo?.suggestedMemoryMB ?? settings.maxMemoryMB
    : settings.maxMemoryMB;
  const needMB = planMB + settings.memoryReserveMB;

  return (
    <div className="page col" style={{ gap: 8, maxWidth: 760 }}>
      {/* 账户 */}
      <div className="settings-section">
        <h3>账户</h3>
        <div className="card col" style={{ gap: 12 }}>
          {accounts.map((a) => (
            <div key={a.id} className="row">
              <div className="avatar">{a.username.slice(0, 1).toUpperCase()}</div>
              <div className="grow">
                <div style={{ fontSize: 13 }}>{a.username}</div>
                <div className="text-tertiary mono" style={{ fontSize: 11 }}>
                  {a.uuid} · {a.type === "microsoft" ? "微软正版" : a.type === "offline" ? "离线模式" : "第三方"}
                </div>
              </div>
              <button className="btn btn-sm btn-icon btn-danger" onClick={() => removeAccount(a.id)}>
                <Trash2 size={13} />
              </button>
            </div>
          ))}
          <div className="row">
            <input className="input grow" placeholder="离线账户名（如 Caeliana）" value={newUser} onChange={(e) => setNewUser(e.target.value)} />
            <button
              className="btn"
              disabled={!newUser.trim()}
              onClick={() => {
                addOfflineAccount(newUser.trim());
                setNewUser("");
              }}
            >
              <UserPlus size={13} /> 添加离线账户
            </button>
          </div>
          <div className="text-tertiary" style={{ fontSize: 12 }}>
            微软正版登录（设备码流）将在 Rust 核心接入后可用
          </div>
        </div>
      </div>

      {/* 内存与性能 */}
      <div className="settings-section">
        <h3>启动与性能</h3>

        {/* 内存状况 */}
        <div className="card" style={{ marginBottom: 12 }}>
          <div className="row" style={{ marginBottom: 10 }}>
            <MemoryStick size={16} color="var(--accent)" />
            <span style={{ fontSize: 13, fontWeight: 600 }}>内存状况</span>
            {memoryInfo?.pressure.needed ? (
              <span className="badge badge-warning">内存偏紧</span>
            ) : (
              <span className="badge badge-accent">充足</span>
            )}
            {lastOptimize && !lastOptimize.admin && (
              <span className="badge badge-danger">无管理员权限</span>
            )}
            <span className="grow" />
            <button className="btn btn-sm" disabled={optimizing || !memoryInfo} onClick={() => void optimizeMemory()}>
              {optimizing ? <Loader2 size={13} className="spin" /> : <Sparkles size={13} />}
              {optimizing ? "优化中…" : "立即优化"}
            </button>
          </div>

          {memoryInfo ? <MemoryBar info={memoryInfo} /> : <div className="text-tertiary" style={{ fontSize: 12 }}>读取中…</div>}

          <div className="text-secondary" style={{ fontSize: 12, marginTop: 10, lineHeight: 1.7 }}>
            本次启动计划：<b>{formatMemoryMB(planMB)}</b>（Xmx）
            {settings.autoMemory ? " · 自动分配" : " · 手动指定"}，连同系统预留共需约{" "}
            <b>{formatMemoryMB(needMB)}</b>
            <br />
            <span className="text-tertiary">{memoryInfo?.pressure.reason}</span>
          </div>

          {optimizeError && (
            <div className="text-tertiary" style={{ fontSize: 12, marginTop: 8, color: "var(--danger)" }}>
              优化失败：{optimizeError}
            </div>
          )}

          {lastOptimize && !optimizeError && (
            <div className="mem-result">
              <div style={{ fontSize: 12 }}>
                {lastOptimize.freedMB > 0 ? (
                  <>
                    已释放 <b style={{ color: "var(--accent)" }}>{formatMemoryMB(lastOptimize.freedMB)}</b>
                    （{formatMemoryMB(lastOptimize.beforeMB)} → {formatMemoryMB(lastOptimize.afterMB)} 可用）
                  </>
                ) : (
                  <>优化完成，未释放到可用内存（{formatMemoryMB(lastOptimize.beforeMB)} → {formatMemoryMB(lastOptimize.afterMB)}）</>
                )}
              </div>
              <div className="mem-steps">
                {lastOptimize.steps.map((s) => (
                  <div key={s.name} className="mem-step">
                    <span className={s.ok ? "ok" : "no"}>{s.ok ? "✓" : "✕"}</span>
                    <span className="grow">{s.name}</span>
                    <span className="text-tertiary">{s.detail}</span>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>

        {/* 内存分配 */}
        <div className="card">
          <div className="settings-row">
            <div>
              <div className="settings-label">自动分配内存</div>
              <div className="settings-desc">
                按系统内存自动选择上限，当前建议 {memoryInfo ? formatMemoryMB(memoryInfo.suggestedMB) : "—"}
              </div>
            </div>
            <Switch checked={settings.autoMemory} onChange={(v) => updateSettings({ autoMemory: v })} />
          </div>
          <div className="settings-row">
            <div>
              <div className="settings-label">最大内存（MB）</div>
              <div className="settings-desc">自动分配关闭时生效，即 JVM 的 -Xmx</div>
            </div>
            <input
              className="input"
              type="number"
              style={{ width: 110 }}
              value={settings.maxMemoryMB}
              disabled={settings.autoMemory}
              onChange={(e) => updateSettings({ maxMemoryMB: Math.max(512, Number(e.target.value)) })}
            />
          </div>
          <div className="settings-row">
            <div>
              <div className="settings-label">系统预留（MB）</div>
              <div className="settings-desc">除游戏外留给系统的余量，用于判断内存是否够用</div>
            </div>
            <input
              className="input"
              type="number"
              style={{ width: 110 }}
              value={settings.memoryReserveMB}
              onChange={(e) => updateSettings({ memoryReserveMB: Math.max(0, Number(e.target.value)) })}
            />
          </div>
          <div className="settings-row">
            <div>
              <div className="settings-label">内存不足时自动优化</div>
              <div className="settings-desc">
                启动前若可用内存低于 {formatMemoryMB(needMB)} 或不足总量的 20%，自动清理工作集与待机列表
              </div>
            </div>
            <Switch checked={settings.memoryOptimize} onChange={(v) => updateSettings({ memoryOptimize: v })} />
          </div>
          <div className="settings-row">
            <div>
              <div className="settings-label">启动后关闭启动器</div>
              <div className="settings-desc">游戏启动后最小化/退出主窗口</div>
            </div>
            <Switch checked={settings.closeOnLaunch} onChange={(v) => updateSettings({ closeOnLaunch: v })} />
          </div>
        </div>
      </div>

      {/* 游戏 */}
      <div className="settings-section">
        <h3>游戏</h3>
        <div className="card">
          <div className="settings-row">
            <div>
              <div className="settings-label">游戏语言</div>
              <div className="settings-desc">
                启动时写入实例 options.txt 的 lang 项，下次启动生效
              </div>
            </div>
            <select
              className="select"
              value={settings.gameLanguage}
              onChange={(e) => updateSettings({ gameLanguage: e.target.value as GameLanguage })}
            >
              {GAME_LANGUAGES.map((g) => (
                <option key={g.value} value={g.value}>
                  {g.label}
                </option>
              ))}
            </select>
          </div>
        </div>
      </div>

      {/* 下载 */}
      <div className="settings-section">
        <h3>下载</h3>
        <div className="card">
          <div className="settings-row">
            <div>
              <div className="settings-label">下载线程数</div>
              <div className="settings-desc">多线程分块下载，国内网络建议 8~16</div>
            </div>
            <input
              className="input"
              type="number"
              style={{ width: 110 }}
              value={settings.downloadThreads}
              onChange={(e) => updateSettings({ downloadThreads: Number(e.target.value) })}
            />
          </div>
        </div>
      </div>

      {/* 关于 */}
      <div className="settings-section">
        <h3>关于</h3>
        <div className="card text-secondary" style={{ fontSize: 13, lineHeight: 1.7 }}>
          MC Launcher v0.1.0（开发中）
          <br />
          前端：React 19 + Tauri 2 + Rust 核心 · 本页面所有设置将持久化到 Rust 侧配置文件
        </div>
      </div>
    </div>
  );
}
