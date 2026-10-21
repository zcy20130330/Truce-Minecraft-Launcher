# -*- coding: utf-8 -*-
"""系统环境探测：物理内存 / 自动内存推荐 / 系统 UI 语言（纯标准库，跨平台）。

对标 PCL 的自动内存分配（ModLaunch.vb 内存推算）与语言跟随。
"""
import os
import sys

# ---------- 物理内存 ----------


def memory_status():
    """返回 {"total_mb", "available_mb", "load"}，失败返回 None。

    Windows 走 GlobalMemoryStatusEx（ullAvailPhys = 立即可用的物理内存）。
    """
    if sys.platform == "win32":
        try:
            import ctypes

            class MEMORYSTATUSEX(ctypes.Structure):
                _fields_ = [
                    ("dwLength", ctypes.c_ulong),
                    ("dwMemoryLoad", ctypes.c_ulong),
                    ("ullTotalPhys", ctypes.c_ulonglong),
                    ("ullAvailPhys", ctypes.c_ulonglong),
                    ("ullTotalPageFile", ctypes.c_ulonglong),
                    ("ullAvailPageFile", ctypes.c_ulonglong),
                    ("ullTotalVirtual", ctypes.c_ulonglong),
                    ("ullAvailVirtual", ctypes.c_ulonglong),
                    ("ullAvailExtendedVirtual", ctypes.c_ulonglong),
                ]

            stat = MEMORYSTATUSEX()
            stat.dwLength = ctypes.sizeof(MEMORYSTATUSEX)
            if ctypes.windll.kernel32.GlobalMemoryStatusEx(ctypes.byref(stat)):
                return {
                    "total_mb": int(stat.ullTotalPhys / (1024 * 1024)),
                    "available_mb": int(stat.ullAvailPhys / (1024 * 1024)),
                    "load": int(stat.dwMemoryLoad),
                }
        except Exception:
            pass
        return None

    # Linux：MemTotal / MemAvailable
    total = avail = None
    try:
        with open("/proc/meminfo", "r") as f:
            for line in f:
                if line.startswith("MemTotal:"):
                    total = int(line.split()[1]) // 1024
                elif line.startswith("MemAvailable:"):
                    avail = int(line.split()[1]) // 1024
        if total:
            return {"total_mb": total, "available_mb": avail if avail else total,
                    "load": int((total - (avail or total)) * 100 / total)}
    except Exception:
        pass
    total = total_memory_mb()
    if total:
        return {"total_mb": total, "available_mb": total, "load": 100}
    return None


def total_memory_mb():
    """系统物理内存总量（MB）。失败返回 None。"""
    st = memory_status()
    if st:
        return st["total_mb"]
    try:
        return int(os.sysconf("SC_PAGE_SIZE") * os.sysconf("SC_PHYS_PAGES") / (1024 * 1024))
    except Exception:
        return None


def available_memory_mb():
    """立即可用的物理内存（MB）。失败返回 None。"""
    st = memory_status()
    return st["available_mb"] if st else None


def suggest_memory_mb(total_mb=None):
    """根据物理内存推荐 -Xmx（MB）。

    策略：按整机容量分档，并保留系统开销——
    - 上限不超过物理内存的 45%，避免挤压系统/其它程序造成卡顿
    - 下限 512MB（低于此 MC 基本无法加载）
    """
    if total_mb is None:
        total_mb = total_memory_mb()
    if not total_mb:
        return 4096  # 探测失败时的保守值

    if total_mb >= 65536:        # >= 64G
        mb = 8192
    elif total_mb >= 32768:      # >= 32G
        mb = 6144
    elif total_mb >= 16384:      # >= 16G
        mb = 4096
    elif total_mb >= 8192:       # >= 8G
        mb = 3072
    elif total_mb >= 6144:       # >= 6G
        mb = 2560
    elif total_mb >= 4096:       # >= 4G
        mb = 2048
    elif total_mb >= 2048:       # >= 2G
        mb = 1024
    else:
        mb = 768

    mb = min(mb, max(512, int(total_mb * 0.45)))
    return max(512, mb)


def resolve_ram_mb(cfg, total_mb=None):
    """按配置决定本次启动的内存：自动时用推荐值，否则用手动 ram_gb。

    返回 (mb, source)，source 为 "auto" / "manual"。
    """
    if cfg.get("auto_memory", True):
        return suggest_memory_mb(total_mb), "auto"
    try:
        return int(float(cfg.get("ram_gb", 4.0)) * 1024), "manual"
    except Exception:
        return 4096, "manual"


# ---------- 系统语言 ----------

# Windows LANGID -> MC 语言资源名
_WIN_LANG_MAP = {
    0x0804: "zh_cn", 0x0004: "zh_cn", 0x1004: "zh_cn",              # 简体中文（中国大陆 / 新加坡）
    0x0404: "zh_tw", 0x0C04: "zh_tw", 0x1404: "zh_tw", 0x7C04: "zh_tw",  # 繁体中文（台湾 / 香港 / 澳门）
    0x0411: "ja_jp",
    0x0412: "ko_kr",
    0x0409: "en_us",
}


def system_language():
    """探测系统 UI 语言，返回 MC 语言资源名（如 zh_cn / en_us）。"""
    if sys.platform == "win32":
        try:
            import ctypes
            langid = ctypes.windll.kernel32.GetUserDefaultUILanguage()
            if langid:
                return _WIN_LANG_MAP.get(langid & 0xFFFF, "en_us")
        except Exception:
            pass
    for var in ("LANGUAGE", "LC_ALL", "LC_MESSAGES", "LANG"):
        v = os.environ.get(var)
        if not v:
            continue
        v = v.lower()
        if v.startswith(("zh_cn", "zh_hans")):
            return "zh_cn"
        if v.startswith(("zh_tw", "zh_hk", "zh_hant")):
            return "zh_tw"
        if v.startswith("ja"):
            return "ja_jp"
        if v.startswith("ko"):
            return "ko_kr"
        if v.startswith("en"):
            return "en_us"
    return "en_us"


def resolve_language(setting):
    """把配置里的语言设置解析为具体资源名；"none" 表示不干预。"""
    setting = (setting or "system").strip()
    if setting == "none":
        return "none"
    if setting == "system":
        return system_language()
    return setting
