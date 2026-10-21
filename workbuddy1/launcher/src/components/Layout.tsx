import { NavLink } from "react-router-dom";
import { Boxes, Download, House, Settings, Cpu } from "lucide-react";
import { usingMock } from "@/api/client";

const navItems = [
  { to: "/", label: "首页", icon: House },
  { to: "/instances", label: "实例", icon: Boxes },
  { to: "/download", label: "下载", icon: Download },
  { to: "/settings", label: "设置", icon: Settings },
];

export function Sidebar() {
  return (
    <aside className="sidebar">
      <div className="sidebar-logo">
        <span className="logo-icon">
          <Cpu size={17} strokeWidth={2.5} />
        </span>
        <span>MC Launcher</span>
      </div>
      <nav>
        {navItems.map(({ to, label, icon: Icon }) => (
          <NavLink key={to} to={to} className={({ isActive }) => `nav-item${isActive ? " active" : ""}`} end={to === "/"}>
            <Icon size={16} />
            <span>{label}</span>
          </NavLink>
        ))}
      </nav>
      <div className="sidebar-footer">
        {usingMock && (
          <div style={{ fontSize: 11, color: "var(--warning)", padding: "4px 6px" }}>
            ⚠️ Mock 模式（未接入 Tauri 后端）
          </div>
        )}
        <div style={{ fontSize: 11, color: "var(--text-tertiary)", padding: "4px 6px" }}>
          v0.1.0 · Rust 核心 {usingMock ? "未连接" : "已连接"}
        </div>
      </div>
    </aside>
  );
}

/** 顶栏：当前账户选择 */
export function TopBar({
  title,
  accounts,
  selectedAccountId,
  onSelectAccount,
}: {
  title: string;
  accounts: { id: string; username: string }[];
  selectedAccountId: string | null;
  onSelectAccount: (id: string) => void;
}) {
  const current = accounts.find((a) => a.id === selectedAccountId);
  return (
    <header className="topbar">
      <span className="topbar-title">{title}</span>
      <div className="topbar-spacer" />
      {current ? (
        <select
          className="select"
          value={selectedAccountId ?? ""}
          onChange={(e) => onSelectAccount(e.target.value)}
          title="切换账户"
        >
          {accounts.map((a) => (
            <option key={a.id} value={a.id}>
              {a.username}
            </option>
          ))}
        </select>
      ) : (
        <span className="text-tertiary" style={{ fontSize: 12 }}>无账户</span>
      )}
    </header>
  );
}
