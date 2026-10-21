import { useState } from "react";
import { Plus, PackageOpen } from "lucide-react";
import { useLauncherStore } from "@/store/useLauncherStore";
import { InstanceCard } from "@/components/InstanceCard";
import type { Instance } from "@/api/types";

const loaders: Instance["loader"][] = ["vanilla", "fabric", "forge", "neoforge", "quilt"];

export function InstancesPage() {
  const { instances, launchInstance, deleteInstance, createInstance } = useLauncherStore();
  const [creating, setCreating] = useState(false);
  const [name, setName] = useState("");
  const [version, setVersion] = useState("26.2");
  const [loader, setLoader] = useState<Instance["loader"]>("fabric");

  const submit = async () => {
    if (!name.trim()) return;
    await createInstance({ name: name.trim(), gameVersion: version, loader });
    setName("");
    setCreating(false);
  };

  return (
    <div className="page col" style={{ gap: 16 }}>
      <div className="row">
        <div className="grow" style={{ fontSize: 13 }}>
          共 <b>{instances.length}</b> 个实例
        </div>
        <button className="btn btn-primary" onClick={() => setCreating((v) => !v)}>
          <Plus size={14} /> 新建实例
        </button>
      </div>

      {creating && (
        <div className="card col" style={{ gap: 12 }}>
          <div className="row">
            <input
              className="input grow"
              placeholder="实例名称，如：生存主世界"
              value={name}
              onChange={(e) => setName(e.target.value)}
              autoFocus
            />
            <select className="select" value={version} onChange={(e) => setVersion(e.target.value)}>
              <option value="26.2">MC 26.2</option>
              <option value="26.3">MC 26.3</option>
              <option value="1.21.11">MC 1.21.11</option>
              <option value="1.21.4">MC 1.21.4</option>
              <option value="1.20.1">MC 1.20.1</option>
            </select>
            <select className="select" value={loader} onChange={(e) => setLoader(e.target.value as Instance["loader"])}>
              {loaders.map((l) => (
                <option key={l} value={l}>
                  {l === "vanilla" ? "原版" : l === "neoforge" ? "NeoForge" : l[0].toUpperCase() + l.slice(1)}
                </option>
              ))}
            </select>
            <button className="btn btn-primary" onClick={submit} disabled={!name.trim()}>
              创建
            </button>
          </div>
          <div className="text-tertiary" style={{ fontSize: 12 }}>
            创建后会自动下载游戏核心与所选 Mod 加载器（Rust 核心接入后生效）
          </div>
        </div>
      )}

      {instances.length === 0 ? (
        <div className="empty-state">
          <PackageOpen size={40} />
          <div>还没有实例</div>
          <div style={{ fontSize: 12 }}>点右上角「新建实例」开始</div>
        </div>
      ) : (
        <div className="instance-grid">
          {instances.map((i) => (
            <InstanceCard
              key={i.id}
              instance={i}
              onLaunch={(id) => launchInstance(id).catch((e) => alert(e.message))}
              onDelete={(id) => {
                if (confirm(`确定删除实例「${i.name}」？此操作不可恢复！`)) deleteInstance(id);
              }}
              onRename={() => {}}
            />
          ))}
        </div>
      )}
    </div>
  );
}
