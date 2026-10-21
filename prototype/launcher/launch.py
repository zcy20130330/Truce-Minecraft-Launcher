# -*- coding: utf-8 -*-
"""启动链路核心：参数拼装 + natives 解压 + 进程管理 + 实时日志（对标 PCL ModLaunch.vb 简化版）。"""
import os
import queue
import re
import subprocess
import threading
import time
import zipfile
from collections import deque

from . import auth, options, ramopt, system

CRASH_KEYWORDS = [
    "Crash report saved to",
    "This crash report has been saved to",
    "Could not save crash report to",
    "An exception was thrown, the game will display an error screen and halt",
    "Process crashed with exitcode",
]


# ---------- 日志缓冲（线程安全，供 HTTP 轮询） ----------

class LogBuffer:
    def __init__(self, capacity=2000):
        self._lines = deque(maxlen=capacity)
        self._seq = 0
        self._lock = threading.Lock()

    def push(self, text):
        with self._lock:
            self._seq += 1
            self._lines.append((self._seq, text))

    def snapshot(self, since=0, limit=500):
        with self._lock:
            out = [t for s, t in self._lines if s > since]
            last = self._seq
        return out[-limit:], last

    def clear(self):
        with self._lock:
            self._lines.clear()
            self._seq = 0


LOG = LogBuffer()
STATE = {"status": "idle", "version": None, "pid": None, "error": None}
_PROC = None
_PROC_LOCK = threading.Lock()


def log(text):
    ts = time.strftime("%H:%M:%S")
    LOG.push(f"[{ts}] {text}")
    print(text)


# ---------- natives 解压 ----------

def natives_folder(mc_folder, version_name):
    return os.path.join(mc_folder, "versions", version_name, version_name + "-natives")


def extract_natives(mc_folder, version_name, libs):
    """把 natives 库中的 dll/so 解压到 <版本>-natives 目录，返回 (ok, msg)。"""
    target = natives_folder(mc_folder, version_name)
    os.makedirs(target, exist_ok=True)
    extracted = []
    for lib in libs:
        if not lib.get("natives"):
            continue
        local = lib["local"]
        if not os.path.isfile(local):
            log(f"[natives] 缺失：{lib['name']}")
            continue
        try:
            with zipfile.ZipFile(local) as z:
                for entry in z.infolist():
                    fn = entry.filename
                    if "META-INF" in fn:
                        continue
                    # 只提取平台相关文件：dll（Windows）；保留其他文件（mac/linux 通用 .so/.dylib 也放行）
                    ext = os.path.splitext(fn)[1].lower()
                    if ext not in (".dll", ".so", ".dylib"):
                        continue
                    out = os.path.join(target, os.path.basename(fn))
                    # 跳过已存在且大小一致
                    if os.path.isfile(out) and os.path.getsize(out) == entry.file_size:
                        continue
                    with z.open(entry) as src, open(out, "wb") as dst:
                        dst.write(src.read())
                    extracted.append(out)
        except Exception as e:
            log(f"[natives] 解压失败 {lib['name']}: {e}")
    log(f"[natives] 解压完成，共 {len(extracted)} 个文件 → {target}")
    return extracted


# ---------- 参数拼装 ----------

def build_classpath(mc_folder, version_name, libs):
    parts = []
    for lib in libs:
        if lib.get("natives"):
            continue
        parts.append(lib["local"])
    # 版本主 jar（游戏客户端本体）必须进 classpath，否则 JVM 报 ClassNotFoundException
    version_jar = os.path.join(mc_folder, "versions", version_name, version_name + ".jar")
    if os.path.isfile(version_jar):
        parts.append(version_jar)
    return ";".join(parts)


def _flatten_arg_value(value):
    """版本 JSON 参数 value：字符串或数组。"""
    if isinstance(value, list):
        return [str(v) for v in value]
    return [str(value)]


def build_jvm_args(mc_folder, version_name, libs, ram_mb, natives_dir, custom_jvm):
    args = ["-Xmx%dm" % int(ram_mb)]
    args.append("-Djava.library.path=" + natives_dir)
    args.append("-cp")
    args.append(build_classpath(mc_folder, version_name, libs))
    args.append("-Dlog4j2.formatMsgNoLookups=true")  # Log4j 漏洞防御（同 PCL）
    if custom_jvm:
        args.extend(custom_jvm.split())
    return args


def build_game_args(version, game_dir, assets_dir, asset_index, login, width, height,
                    fullscreen, custom_game, server):
    """新版/旧版游戏参数统一生成。version 为 McVersion 对象。"""
    args = []
    json_obj = version.json
    arg_string = ""

    if json_obj and json_obj.get("arguments", {}).get("game") is not None:
        # 新版：arguments.game 数组（含 rules 过滤的对象）
        for item in json_obj["arguments"]["game"]:
            if isinstance(item, str):
                args.append(item)
            elif isinstance(item, dict):
                rules = item.get("rules")
                if rules and not all(_match_os_rule(r, server or None) for r in rules):
                    continue
                args.extend(_flatten_arg_value(item.get("value", [])))
    elif json_obj and json_obj.get("minecraftArguments"):
        # 旧版：minecraftArguments 字符串
        arg_string = json_obj["minecraftArguments"] + " --width %d --height %d" % (width, height)
        args = arg_string.split()

    tokens = {
        "${auth_player_name}": login["name"],
        "${version_name}": version.name,
        "${game_directory}": game_dir,
        "${assets_root}": assets_dir,
        "${assets_index_name}": asset_index or "legacy",
        "${auth_uuid}": login["uuid"],
        "${auth_access_token}": login["access_token"],
        "${auth_xuid}": "0",
        "${access_token}": login["access_token"],
        "${user_type}": login["user_type"],
        "${version_type}": version.type or "release",
        "${resolution_width}": str(width),
        "${resolution_height}": str(height),
        "${launcher_name}": "PCL-Prototype",
        "${launcher_version}": "0.1",
        "${user_properties}": "{}",
        "${clientid}": login["uuid"],
        "${xuid}": "0",
        "${demo_user}": "false",
        "${assets_version_name}": asset_index or "legacy",
        "${quickPlayPath}": os.path.join(game_dir, "quickPlay.json"),
        "${quickPlaySingleplayer}": "",
        "${quickPlayMultiplayer}": server or "",
        "${quickPlayRealms}": "",
    }
    args = [tokens.get(a, a) for a in args]

    if fullscreen:
        args.append("--fullscreen")
    if custom_game:
        args.extend(custom_game.split())
    if server:
        # 自动进服
        if version.major >= 20 or version.release_time >= "2023-04-05":
            # 新版 JSON 通常已内建 --quickPlayMultiplayer ${quickPlayMultiplayer}（上方已由 tokens 替换为服务器地址）；
            # 若该版本 JSON 没有此条目，才手动补上
            if "--quickPlayMultiplayer" not in args:
                args += ["--quickPlayMultiplayer", server]
        else:
            host, _, port = server.partition(":")
            args += ["--server", host, "--port", port or "25565"]
    return args


def _match_os_rule(rule, quickplay_server=None):
    """判断单条 rule 是否通过。

    - os.name 不匹配 → 不通过
    - features 条件按实际启动场景判断：
      is_demo_user      → 非 demo 启动，不放行
      has_custom_resolution → 始终自定义窗口大小，条件恒满足
      quickPlay 系列    → 仅多人自动进服（填了服务器地址）时放行 multiplayer 相关
    """
    action = rule.get("action", "allow")
    os_rule = rule.get("os") or {}
    name = os_rule.get("name")
    if name and name != "unknown" and name != "windows":
        return action != "allow"
    features = rule.get("features")
    if features:
        for key, want in features.items():
            if key == "is_demo_user":
                if want:
                    return action != "allow"
            elif key == "has_custom_resolution":
                continue  # 总是自定义宽高
            elif key == "has_quick_plays_support" or key.startswith("is_quick_play"):
                if key in ("is_quick_play_singleplayer", "is_quick_play_realms"):
                    enabled = False  # 原型不实现单机存档/Realms 快速进入
                else:
                    enabled = bool(quickplay_server)
                if (action == "allow") != enabled:
                    return False
            else:
                # 未知 features：保守处理，allow 规则一律不通过
                return action != "allow"
    return action == "allow"


def game_dir_for(mc_folder, version, cfg):
    """游戏目录（gameDir）：版本隔离时指向版本文件夹，否则指向 .minecraft。"""
    if isinstance(version, str):
        return os.path.join(mc_folder, "versions", version)
    return version.path if cfg.get("version_isolate", False) else mc_folder


def build_command(mc_folder, version, cfg, login, java_path, ram_mb=None):
    """生成完整启动命令列表。ram_mb 由调用方解析好后传入（避免重复探测）。"""
    libs = version.get_libraries()
    if ram_mb is None:
        ram_mb, ram_source = system.resolve_ram_mb(cfg)
    else:
        ram_source = "auto" if cfg.get("auto_memory", True) else "manual"
    if ram_source == "auto":
        log(f"[内存] 自动分配：-Xmx{ram_mb}m（物理内存 {system.total_memory_mb()} MB）")
    else:
        log(f"[内存] 手动分配：-Xmx{ram_mb}m")
    game_dir = game_dir_for(mc_folder, version, cfg)
    natives = natives_folder(mc_folder, version.name)
    assets_dir = os.path.join(mc_folder, "assets")
    asset_index = version.assets_id
    width = int(cfg.get("window_width", 854))
    height = int(cfg.get("window_height", 480))
    fullscreen = cfg.get("fullscreen", False)
    custom_jvm = cfg.get("jvm_args", "")
    custom_game = cfg.get("game_args", "")
    server = cfg.get("server", "")

    jvm = build_jvm_args(mc_folder, version.name, libs, ram_mb, natives, custom_jvm)
    main_class = version.main_class or "net.minecraft.client.main.Main"
    game = build_game_args(version, game_dir, assets_dir, asset_index, login,
                           width, height, fullscreen, custom_game, server)
    return jvm + [main_class] + game, game_dir


# ---------- 启动与监控 ----------

def _read_stream(stream, stop_event):
    """读取子进程输出（兼容 GBK/UTF-8 编码），写入日志缓冲。"""
    while not stop_event.is_set():
        try:
            line = stream.readline()
        except Exception:
            break
        if not line:
            break
        try:
            text = line.decode("utf-8", errors="replace").rstrip("\r\n")
        except Exception:
            text = line.decode("gbk", errors="replace").rstrip("\r\n")
        log(text)
        for kw in CRASH_KEYWORDS:
            if kw in text:
                log("[watcher] 检测到崩溃日志关键字：" + kw)


def launch(mc_folder, version_name, cfg):
    """启动入口：在工作线程调用。返回 (ok, msg)。"""
    global _PROC
    from .versions import get_version
    from .java import select_java, detect_java

    LOG.clear()
    log("=" * 56)
    log(f"启动版本：{version_name}")

    # 1) 版本解析
    version = get_version(mc_folder, version_name)
    if version.error:
        STATE.update(status="error", error=version.error)
        log("[错误] " + version.error)
        return False, version.error
    log(f"主类：{version.main_class}  加载器：{','.join(version.loaders) or '原版'}  Java需求：{version.java_requirement}")

    # 2) Java 选择
    java_path = cfg.get("java_path") or None
    if java_path and os.path.isfile(java_path):
        log("使用指定 Java：" + java_path)
    else:
        from .java import get_java_version
        java_list = detect_java()
        chosen = select_java(java_list, version.java_requirement)
        if not chosen:
            STATE.update(status="error", error="未找到合适的 Java")
            log("[错误] 未检测到 Java，请安装或手动指定 java.exe 路径")
            return False, "未找到合适的 Java"
        java_path = chosen["path"]
        log(f"自动选择 Java：{java_path}（版本 {'.'.join(map(str, chosen['version']))}）")
    if not java_path or not os.path.isfile(java_path):
        STATE.update(status="error", error=f"Java 不存在：{java_path}")
        return False, f"Java 不存在：{java_path}"

    # 2.5) 版本主 jar 检查（游戏客户端本体，缺了 classpath 再全也没用）
    version_jar = os.path.join(mc_folder, "versions", version_name, version_name + ".jar")
    if not os.path.isfile(version_jar):
        msg = (f"版本主文件缺失：{version_jar}\n"
               f"原型不支持安装新版本，请先用其它启动器装好 {version_name} 再启动")
        STATE.update(status="error", error=msg)
        log("[错误] " + msg)
        return False, msg

    # 2.6) 本次启动的内存 + 低内存自动优化
    ram_mb, ram_source = system.resolve_ram_mb(cfg)
    if cfg.get("memory_optimize", True) and ramopt.supported():
        pressure = ramopt.check_pressure(ram_mb, int(cfg.get("memory_reserve_mb", 1024)))
        if pressure["needed"]:
            log(f"[优化] 检测到内存不足：{pressure['reason']}")
            res = ramopt.optimize(log=log)
            if not res["ok"]:
                log("[优化] 优化未生效" + ("" if res["admin"] else "（建议用管理员身份运行本启动器）"))
        else:
            log(f"[优化] 无需优化：{pressure['reason']}")

    # 3) 补全文件（库 + 资源）
    libs = version.get_libraries()
    from . import download
    prefer_mirror = cfg.get("download_source", 0) == 0
    check_files = cfg.get("check_files", True)
    missing_libs = download.missing_libraries(mc_folder, libs, check_files)
    if missing_libs:
        log(f"[下载] 缺失 {len(missing_libs)} 个支持库文件，开始下载…")
        ok, fail = download.download_many(missing_libs, "library", prefer_mirror,
                                          progress_cb=lambda d, t, n: log(f"[下载] 库 {d}/{t} {n}"))
        if fail:
            log(f"[警告] {fail} 个库文件下载失败，可能无法启动")
    else:
        log("[下载] 支持库文件完整，无需下载")

    # assets 索引 + 资源
    asset_index = version.assets_id
    index_path = os.path.join(mc_folder, "assets", "indexes", asset_index + ".json")
    index_json = None
    if os.path.isfile(index_path):
        try:
            from .versions import load_json
            index_json = load_json(index_path)
        except Exception as e:
            log(f"[警告] 资源索引解析失败：{e}")
    else:
        log(f"[下载] 资源索引缺失：{asset_index}.json（暂不自动下载，不影响核心启动）")
    if index_json:
        missing_assets = download.missing_assets(mc_folder, index_json, check_files)
        if missing_assets:
            log(f"[下载] 缺失 {len(missing_assets)} 个资源文件，开始下载…")
            ok, fail = download.download_many(missing_assets, "asset", prefer_mirror,
                                              progress_cb=lambda d, t, n: log(f"[下载] 资源 {d}/{t}"))
            if fail:
                log(f"[警告] {fail} 个资源文件下载失败")
        else:
            log("[下载] 资源文件完整")

    # 4) 解压 natives
    extract_natives(mc_folder, version_name, libs)

    # 5) 登录（离线）
    login = auth.offline_login(cfg.get("username") or "Player")
    log(f"离线登录：{login['name']}  UUID={login['uuid']}")

    # 5.5) 游戏语言：写 options.txt 的 lang 项（必须在拉进程之前，游戏退出时会覆写该文件）
    lang = system.resolve_language(cfg.get("language", "system"))
    ok_lang, msg_lang = options.apply_language(game_dir_for(mc_folder, version, cfg), lang)
    log("[语言] " + msg_lang)

    # 6) 拼参数
    cmd, game_dir = build_command(mc_folder, version, cfg, login, java_path, ram_mb=ram_mb)
    os.makedirs(game_dir, exist_ok=True)
    log("启动命令：")
    for a in cmd:
        log("  " + a)

    # 7) 启动进程
    env = dict(os.environ)
    env["APPDATA"] = mc_folder  # 同 PCL：让游戏把 .minecraft 指到所选目录
    try:
        proc = subprocess.Popen(
            [java_path] + cmd,
            cwd=game_dir,
            env=env,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            creationflags=getattr(subprocess, "CREATE_NO_WINDOW", 0),
        )
    except Exception as e:
        STATE.update(status="error", error=str(e))
        log("[错误] 启动进程失败：" + str(e))
        return False, str(e)

    with _PROC_LOCK:
        _PROC = proc
    STATE.update(status="running", version=version_name, pid=proc.pid, error=None)
    log(f"游戏进程已启动 PID={proc.pid}")

    # 8) 后台读取日志（不阻塞）
    stop = threading.Event()
    t = threading.Thread(target=_read_stream, args=(proc.stdout, stop), daemon=True)
    t.start()

    # 9) 等待退出
    code = proc.wait()
    stop.set()
    with _PROC_LOCK:
        _PROC = None
    log(f"游戏进程已退出，退出码：{code}")
    if code != 0:
        log("[watcher] 退出码非 0，可能发生崩溃（可查看上方日志）")
        STATE.update(status="exited", pid=None, error=f"退出码 {code}")
        return True, f"进程退出（代码 {code}）"
    STATE.update(status="exited", pid=None, error=None)
    return True, "游戏已退出"


def stop_game():
    global _PROC
    with _PROC_LOCK:
        p = _PROC
    if p and p.poll() is None:
        p.kill()
        log("已强制结束游戏进程")
        return True
    return False


def current_state():
    s = dict(STATE)
    if s["status"] == "running":
        with _PROC_LOCK:
            if _PROC and _PROC.poll() is not None:
                s["status"] = "exited"
    return s
