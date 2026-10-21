import { Play, Trash2, Pencil, Package } from "lucide-react";
import type { Instance } from "@/api/types";
import { formatPlaytime, formatRelativeTime } from "@/utils/format";
import { useState } from "react";

const loaderLabels: Record<Instance["loader"], string> = {
  vanilla: "原版",
  fabric: "Fabric",
  forge: "Forge",
  neoforge: "NeoForge",
  quilt: "Quilt",
};

interface Props {
  instance: Instance;
  onLaunch: (id: string) => void;
  onDelete: (id: string) => void;
  onRename: (id: string, name: string) => void;
}

export function InstanceCard({ instance, onLaunch, onDelete, onRename }: Props) {
  const [renaming, setRenaming] = useState(false);
  const [draft, setDraft] = useState(instance.name);

  const commitRename = () => {
    setRenaming(false);
    const name = draft.trim();
    if (name && name !== instance.name) onRename(instance.id, name);
    else setDraft(instance.name);
  };

  return (
    <div className="card instance-card">
      <div className="instance-head">
        <div className="instance-icon">
          <Package size={20} color="var(--accent)" />
        </div>
        <div className="grow">
          {renaming ? (
            <input
              className="input"
              style={{ width: "100%" }}
              autoFocus
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              onBlur={commitRename}
              onKeyDown={(e) => e.key === "Enter" && commitRename()}
            />
          ) : (
            <div className="instance-name">{instance.name}</div>
          )}
          <div className="instance-meta">
            MC {instance.gameVersion} · {loaderLabels[instance.loader]}
            {instance.loaderVersion && ` ${instance.loaderVersion}`}
          </div>
        </div>
      </div>
      <div className="instance-stats">
        <span>🕐 {formatPlaytime(instance.playTimeSeconds)}</span>
        <span>📦 {instance.modCount} 个 Mod</span>
        <span>🎮 {formatRelativeTime(instance.lastPlayed)}</span>
      </div>
      <div className="instance-actions">
        <button className="btn btn-primary btn-sm grow" onClick={() => onLaunch(instance.id)}>
          <Play size={13} /> 启动
        </button>
        <button className="btn btn-sm btn-icon" title="重命名" onClick={() => setRenaming(true)}>
          <Pencil size={13} />
        </button>
        <button className="btn btn-sm btn-icon btn-danger" title="删除" onClick={() => onDelete(instance.id)}>
          <Trash2 size={13} />
        </button>
      </div>
    </div>
  );
}
