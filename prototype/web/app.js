/* PCL-Prototype 前端逻辑：版本列表 / 设置 / 启动 / 日志轮询 */
"use strict";

const $ = (sel) => document.querySelector(sel);

let cfg = {};
let sysInfo = null;
let ramInfo = null;
let versions = [];
let selected = null;
let logSince = 0;
let pollTimer = null;

/* ---------- API ---------- */
async function api(path, opts) {
  const res = await fetch(path, opts);
  return res.json();
}
const post = (path, body) => api(path, {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify(body || {}),
});

/* ---------- 版本列表 ---------- */
async function refreshVersions() {
  const listEl = $("#version-list");
  listEl.innerHTML = '<div class="empty">正在扫描版本…</div>';
  try {
    const data = await api("/api/versions?folder=" + encodeURIComponent(cfg.mc_folder || ""));
    versions = data;
    $("#ver-count").textContent = versions.length ? `(${versions.length})` : "";
    if (!versions.length) {
      listEl.innerHTML = '<div class="empty">未找到版本<br>请检查 MC 文件夹设置</div>';
      return;
    }
    listEl.innerHTML = "";
    for (const v of versions) {
      const card = document.createElement("div");
      card.className = "version-card" + (v.state === "error" ? " error" : "");
      card.dataset.name = v.name;
      const tags = (v.loaders || []).map(t => `<span class="tag ${t}">${t}</span>`).join("");
      card.innerHTML = `
        <div class="vc-name">${escapeHtml(v.name)}${v.state === "error" ? `<span class="err">${escapeHtml(v.error || "错误")}</span>` : ""}</div>
        <div class="vc-meta">${escapeHtml(v.type || "")}${v.release_time ? " · " + escapeHtml(v.release_time.slice(0, 10)) : ""}${v.java ? " · Java " + v.java : ""}</div>
        ${tags ? `<div class="vc-tags">${tags}</div>` : ""}`;
      card.addEventListener("click", () => selectVersion(v.name));
      listEl.appendChild(card);
    }
    if (selected && versions.some(v => v.name === selected)) {
      selectVersion(selected, true);
    }
  } catch (e) {
    listEl.innerHTML = `<div class="empty">扫描失败：${escapeHtml(String(e))}</div>`;
  }
}

function selectVersion(name, silent) {
  selected = name;
  document.querySelectorAll(".version-card").forEach(c => {
    c.classList.toggle("active", c.dataset.name === name);
  });
  const v = versions.find(x => x.name === name);
  const info = $("#selected-info");
  if (!v || v.state === "error") {
    info.innerHTML = `<div class="si-name">${escapeHtml(name)}</div><div class="si-error">${escapeHtml(v ? (v.error || "版本损坏") : "未知")}</div>`;
    $("#btn-launch").disabled = true;
    return;
  }
  const tags = (v.loaders || []).map(t => `<span class="tag ${t}">${t}</span>`).join("");
  info.innerHTML = `
    <div class="si-name">${escapeHtml(v.name)}</div>
    <div class="si-meta">
      类型：${escapeHtml(v.type)}<br>
      发布：${escapeHtml(v.release_time || "未知")}<br>
      Java 需求：${v.java}（${javaHint(v.java)}）<br>
      ${v.major ? `MC 主版本：1.${v.major}<br>` : ""}
    </div>
    ${tags ? `<div class="si-loaders">${tags}</div>` : ""}`;
  $("#btn-launch").disabled = false;
}

function javaHint(major) {
  if (major >= 21) return "Java 21+";
  if (major === 17) return "Java 17+";
  if (major === 16) return "Java 16+";
  if (major === 8) return "Java 8";
  return "Java " + major;
}

/* ---------- 设置 ---------- */
function openSettings() {
  $("#settings-overlay").classList.remove("hidden");
  $("#cfg-mcfolder").value = cfg.mc_folder || "";
  $("#cfg-username").value = cfg.username || "";
  $("#cfg-ram").value = cfg.ram_gb || 4;
  $("#ram-range").value = cfg.ram_gb || 4;
  $("#ram-label").textContent = (cfg.ram_gb || 4) + " GB";
  $("#cfg-automem").checked = cfg.auto_memory !== false;
  $("#mem-hint").textContent = memoryHint();
  $("#cfg-language").value = cfg.language || "system";
  $("#cfg-memopt").checked = cfg.memory_optimize !== false;
  updateMemUI();
  loadRamoptInfo();
  $("#cfg-java-mode").value = cfg.java_path ? "manual" : "auto";
  $("#row-java-path").classList.toggle("hidden", !cfg.java_path);
  $("#cfg-javapath").value = cfg.java_path || "";
  $("#cfg-width").value = cfg.window_width || 854;
  $("#cfg-height").value = cfg.window_height || 480;
  $("#cfg-fullscreen").checked = !!cfg.fullscreen;
  $("#cfg-jvmargs").value = cfg.jvm_args || "";
  $("#cfg-gameargs").value = cfg.game_args || "";
  $("#cfg-server").value = cfg.server || "";
  $("#cfg-dlsource").value = String(cfg.download_source == null ? 0 : cfg.download_source);
  $("#cfg-checkfiles").checked = cfg.check_files !== false;
  loadJavaList();
}

function collectSettings() {
  return {
    mc_folder: $("#cfg-mcfolder").value.trim(),
    username: $("#cfg-username").value.trim(),
    ram_gb: parseFloat($("#cfg-ram").value) || 4,
    auto_memory: $("#cfg-automem").checked,
    language: $("#cfg-language").value,
    memory_optimize: $("#cfg-memopt").checked,
    version_isolate: $("#cfg-isolate").checked,
    java_path: $("#cfg-java-mode").value === "manual" ? $("#cfg-javapath").value.trim() : "",
    window_width: parseInt($("#cfg-width").value) || 854,
    window_height: parseInt($("#cfg-height").value) || 480,
    fullscreen: $("#cfg-fullscreen").checked,
    jvm_args: $("#cfg-jvmargs").value.trim(),
    game_args: $("#cfg-gameargs").value.trim(),
    server: $("#cfg-server").value.trim(),
    download_source: parseInt($("#cfg-dlsource").value) || 0,
    check_files: $("#cfg-checkfiles").checked,
  };
}

async function saveSettings() {
  const body = collectSettings();
  const res = await post("/api/config", body);
  if (res.ok) {
    cfg = res.config;
    closeSettings();
    refreshVersions();
  }
}

/* ---------- 自动内存 / 语言 ---------- */
async function loadSystemInfo() {
  try {
    sysInfo = await api("/api/system");
  } catch (e) {
    sysInfo = null;
  }
  $("#mem-hint").textContent = memoryHint();
  updateMemUI();
}

function memoryHint() {
  if (!sysInfo || !sysInfo.total_memory_mb) return "未能读取物理内存";
  const total = (sysInfo.total_memory_mb / 1024).toFixed(1);
  const sug = (sysInfo.suggested_mb / 1024).toFixed(1);
  return `物理内存 ${total} GB · 自动建议 ${sug} GB · 当前系统语言 ${sysInfo.system_language}`;
}

function updateMemUI() {
  const auto = $("#cfg-automem").checked;
  $("#cfg-ram").disabled = auto;
  $("#ram-range").disabled = auto;
  if (auto && sysInfo) {
    $("#ram-range").value = Math.min(16, sysInfo.suggested_mb / 1024);
    $("#ram-label").textContent = (sysInfo.suggested_mb / 1024).toFixed(1) + " GB（自动）";
  } else {
    $("#ram-label").textContent = (parseFloat($("#cfg-ram").value) || 4) + " GB";
  }
}

async function loadRamoptInfo() {
  try {
    ramInfo = await api("/api/ramopt");
  } catch (e) {
    ramInfo = null;
  }
  $("#memopt-hint").textContent = ramoptHint();
}

function ramoptHint() {
  if (!ramInfo) return "读取内存状态失败";
  if (!ramInfo.supported) return "内存优化仅支持 Windows";
  const a = ramInfo.available_mb, t = ramInfo.total_mb;
  const admin = ramInfo.admin ? "" : " · 非管理员，建议用管理员身份运行才有效果";
  const advice = ramInfo.needed ? `当前内存不足，建议优化（${ramInfo.reason}）` : "当前内存充足";
  return `可用 ${a}/${t} MB · ${advice}${admin}`;
}

async function runMemOpt() {
  const el = $("#memopt-hint");
  el.textContent = "正在优化…";
  try {
    const res = await post("/api/ramopt_optimize");
    if (res.freed_mb > 0) {
      el.textContent = `已释放 ${res.freed_mb} MB（${res.before_mb} → ${res.after_mb} MB）`;
    } else {
      el.textContent = `优化完成，未释放到内存（${res.before_mb} → ${res.after_mb} MB）`
        + (res.admin ? "" : " · 建议用管理员身份运行");
    }
  } catch (e) {
    el.textContent = "优化失败：" + e;
  }
}

async function loadJavaList() {
  try {
    const list = await api("/api/java");
    const el = $("#java-list");
    if (!list.length) {
      el.innerHTML = '<span class="hint">未检测到 Java（可手动指定 java.exe）</span>';
      return;
    }
    el.innerHTML = list.map(j => `• ${escapeHtml(j.path)} <span class="hint">(Java ${j.major})</span>`).join("<br>");
  } catch (e) {
    $("#java-list").innerHTML = '<span class="hint">检测失败</span>';
  }
}

/* ---------- 启动 ---------- */
async function launch() {
  if (!selected) return;
  $("#btn-launch").disabled = true;
  $("#btn-stop").disabled = false;
  logSince = 0;
  $("#log-view").textContent = "";
  const body = collectSettings();
  body.version = selected;
  const res = await post("/api/launch", body);
  if (!res.ok) {
    setBadge("error", res.error || "启动请求失败");
    $("#btn-launch").disabled = false;
    $("#btn-stop").disabled = true;
  }
}

async function stopGame() {
  await post("/api/stop");
}

/* ---------- 日志轮询 ---------- */
async function pollLogs() {
  try {
    const data = await api("/api/logs?since=" + logSince);
    if (data.lines && data.lines.length) {
      const view = $("#log-view");
      view.textContent += data.lines.join("\n") + "\n";
      view.scrollTop = view.scrollHeight;
      logSince = data.last;
    }
    const st = await api("/api/state");
    updateState(st);
  } catch (e) { /* 服务暂不可用 */ }
}

function updateState(st) {
  const map = {
    idle: ["idle", "空闲"],
    running: ["running", "运行中"],
    exited: ["exited", "已退出"],
    error: ["error", "出错"],
  };
  const [cls, label] = map[st.status] || ["idle", "空闲"];
  const badge = $("#state-badge");
  badge.className = "badge " + cls;
  badge.textContent = st.pid ? `${label} (PID ${st.pid})` : label;
  if (st.status === "running") {
    $("#btn-launch").disabled = true;
    $("#btn-stop").disabled = false;
  } else if (st.status === "exited" || st.status === "error" || st.status === "idle") {
    $("#btn-stop").disabled = true;
    if (!selected || st.status === "idle") $("#btn-launch").disabled = true;
    else $("#btn-launch").disabled = false;
  }
}

/* ---------- 工具 ---------- */
function escapeHtml(s) {
  return String(s).replace(/[&<>"']/g, c => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
  }[c]));
}
function setBadge(cls, text) {
  const b = $("#state-badge");
  b.className = "badge " + cls;
  b.textContent = text;
}

/* ---------- 事件绑定 ---------- */
$("#btn-refresh").addEventListener("click", refreshVersions);
$("#btn-settings").addEventListener("click", openSettings);
$("#btn-close-settings").addEventListener("click", closeSettings);
$("#btn-save-settings").addEventListener("click", saveSettings);
$("#btn-launch").addEventListener("click", launch);
$("#btn-stop").addEventListener("click", stopGame);
$("#btn-clear-log").addEventListener("click", () => { $("#log-view").textContent = ""; });
$("#cfg-java-mode").addEventListener("change", (e) => {
  $("#row-java-path").classList.toggle("hidden", e.target.value !== "manual");
});
$("#ram-range").addEventListener("input", (e) => {
  const v = parseFloat(e.target.value);
  $("#ram-label").textContent = v + " GB";
  $("#cfg-ram").value = v;
});
$("#cfg-automem").addEventListener("change", updateMemUI);
$("#cfg-ram").addEventListener("input", updateMemUI);
$("#btn-memopt").addEventListener("click", runMemOpt);
$("#btn-scan-folder").addEventListener("click", async () => {
  try {
    const data = await api("/api/scan_folder");
    if (data.folders && data.folders.length) {
      $("#cfg-mcfolder").value = data.folders[0];
    } else {
      alert("未在常见路径找到 .minecraft，请手动填写路径");
    }
  } catch (e) { alert("扫描失败：" + e); }
});

function closeSettings() {
  $("#settings-overlay").classList.add("hidden");
}

/* ---------- 初始化 ---------- */
(async function init() {
  try {
    cfg = await api("/api/config");
  } catch (e) {
    console.error("加载配置失败", e);
  }
  await loadSystemInfo();
  await refreshVersions();
  pollTimer = setInterval(pollLogs, 800);
})();
