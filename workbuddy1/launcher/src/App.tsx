import { useEffect } from "react";
import { HashRouter, Navigate, Route, Routes, useLocation } from "react-router-dom";
import { Sidebar, TopBar } from "@/components/Layout";
import { HomePage } from "@/pages/HomePage";
import { InstancesPage } from "@/pages/InstancesPage";
import { DownloadPage } from "@/pages/DownloadPage";
import { SettingsPage } from "@/pages/SettingsPage";
import { useLauncherStore } from "@/store/useLauncherStore";
import { api } from "@/api/client";

const titles: Record<string, string> = {
  "/": "首页",
  "/instances": "实例管理",
  "/download": "下载管理",
  "/settings": "设置",
};

function Shell() {
  const location = useLocation();
  const { accounts, selectedAccountId, setSelectedAccount, refreshAll } = useLauncherStore();

  useEffect(() => {
    void refreshAll();
    // 订阅启动状态事件
    let stop: (() => void) | undefined;
    api.onLaunchState((s) => console.log("[launch-state]", s)).then((fn) => {
      stop = fn;
    });
    return () => stop?.();
  }, [refreshAll]);

  return (
    <div className="app-shell">
      <Sidebar />
      <div className="main">
        <TopBar
          title={titles[location.pathname] ?? "MC Launcher"}
          accounts={accounts}
          selectedAccountId={selectedAccountId}
          onSelectAccount={setSelectedAccount}
        />
        <Routes>
          <Route path="/" element={<HomePage />} />
          <Route path="/instances" element={<InstancesPage />} />
          <Route path="/download" element={<DownloadPage />} />
          <Route path="/settings" element={<SettingsPage />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </div>
    </div>
  );
}

export function App() {
  return (
    <HashRouter>
      <Shell />
    </HashRouter>
  );
}
