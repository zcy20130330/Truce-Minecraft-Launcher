# -*- coding: utf-8 -*-
"""Java 检测与版本解析（对标 PCL ModJava.vb 简化版）。"""
import glob
import os
import re
import subprocess

CANDIDATE_ROOTS = [
    r"C:\Program Files\Java",
    r"C:\Program Files\Eclipse Adoptium",
    r"C:\Program Files\Microsoft",
    r"C:\Program Files (x86)\Java",
    r"C:\Program Files\Zulu",
    r"C:\Program Files\Amazon Corretto",
    r"C:\Program Files\BellSoft",
    r"C:\Program Files\RedHat",
    os.path.expandvars(r"%LOCALAPPDATA%\Programs\Eclipse Adoptium"),
    os.path.expandvars(r"%LOCALAPPDATA%\Programs\Microsoft"),
    os.path.expandvars(r"%USERPROFILE%\.jdks"),
]


def _parse_java_version(text):
    """解析 `java -version` 输出，返回 Version 对象（major/minor/patch）。"""
    m = re.search(r'version "([0-9._]+)"', text) or re.search(r'version "([0-9.]+)"', text)
    if not m:
        return None
    raw = m.group(1)
    if raw.startswith("1."):
        # 1.8.0_xxx -> 8
        parts = raw.split(".")
        return (int(parts[1]), 0, 0)
    parts = raw.split(".")
    major = int(parts[0])
    minor = int(parts[1]) if len(parts) > 1 else 0
    patch = int(parts[2]) if len(parts) > 2 else 0
    return (major, minor, patch)


def get_java_version(java_path):
    try:
        p = subprocess.run([java_path, "-version"], capture_output=True, timeout=15,
                           creationflags=getattr(subprocess, "CREATE_NO_WINDOW", 0))
        text = (p.stdout + p.stderr).decode("utf-8", errors="replace")
        v = _parse_java_version(text)
        if v is None:
            return None
        return {"path": java_path, "version": v, "major": v[0]}
    except Exception:
        return None


def _collect_candidates():
    """收集所有可能的 java.exe 路径。"""
    found = set()
    # 1) 常见安装根目录递归
    for root in CANDIDATE_ROOTS:
        if os.path.isdir(root):
            for j in glob.glob(os.path.join(root, "**", "bin", "java.exe"), recursive=True):
                found.add(j)
    # 2) JAVA_HOME
    jh = os.environ.get("JAVA_HOME")
    if jh:
        p = os.path.join(jh, "bin", "java.exe")
        if os.path.isfile(p):
            found.add(p)
    # 3) PATH 中的 java
    for d in os.environ.get("PATH", "").split(";"):
        d = d.strip().strip('"')
        if d:
            p = os.path.join(d, "java.exe")
            if os.path.isfile(p):
                found.add(p)
    return sorted(found)


def detect_java():
    """检测系统 Java，返回 [{path, version, major}]，按版本号从高到低。"""
    result = []
    seen = set()
    for path in _collect_candidates():
        if path.lower() in seen:
            continue
        seen.add(path.lower())
        info = get_java_version(path)
        if info:
            result.append(info)
    result.sort(key=lambda x: x["version"], reverse=True)
    return result


def select_java(java_list, required_major):
    """按需求选择 Java：优先 exact major，其次 >= required（<=21 限制放宽）。"""
    if not java_list:
        return None
    for j in java_list:
        if j["major"] == required_major:
            return j
    # 找不到精确版本，取 >= required 的最低版本（避免用太新的）
    eligible = [j for j in java_list if j["major"] >= required_major]
    if eligible:
        return eligible[-1]  # 已按版本降序，取最后一个即最低
    # 全部低于要求：取最高版本（老版本 MC 可降级用）
    return java_list[0]
