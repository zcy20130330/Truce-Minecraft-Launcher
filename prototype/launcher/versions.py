# -*- coding: utf-8 -*-
"""版本扫描与 JSON 解析（对标 PCL ModMinecraft.vb 的 McInstance/McLibEntry/McJsonRuleCheck 简化版）。"""
import json
import os
import re

ARCH = "64"  # 原型只处理 64 位


# ---------- 基础工具 ----------

def load_json(path):
    with open(path, "r", encoding="utf-8-sig") as f:
        return json.load(f)


def version_folder(mc_folder, name):
    return os.path.join(mc_folder, "versions", name)


def version_json_path(mc_folder, name):
    return os.path.join(version_folder(mc_folder, name), name + ".json")


def _match_rule(rule, os_name):
    """rules 过滤：简化实现 os.name / os.arch（默认 64 位 x86）。返回 True=通过。"""
    action = rule.get("action", "allow")
    ok = True
    os_rule = rule.get("os")
    if os_rule:
        if "name" in os_rule and os_rule["name"] != "unknown" and os_rule["name"] != os_name:
            ok = False
        if "arch" in os_rule:
            a = os_rule["arch"]
            if a == "x86" and ARCH != "x86":
                ok = False
            elif "arm" in a and "x86" not in a:
                ok = False
    features = rule.get("features")
    if features:
        # is_demo_user=true 的库仅 demo 模式加载；非 demo 玩家过滤
        if features.get("is_demo_user") is True:
            ok = False
    return (action == "allow") == ok


def filter_rules(entries, os_name):
    out = []
    for e in entries or []:
        rules = e.get("rules")
        if rules and not all(_match_rule(r, os_name) for r in rules):
            continue
        out.append(e)
    return out


# ---------- 版本解析 ----------

class McVersion:
    def __init__(self, mc_folder, name):
        self.mc_folder = mc_folder
        self.name = name
        self.path = version_folder(mc_folder, name)
        self.json_path = version_json_path(mc_folder, name)
        self.json = None
        self.error = None
        self._load()

    def _load(self):
        try:
            self.json = load_json(self.json_path)
        except Exception as e:
            self.error = f"无法读取版本 JSON：{e}"
            return
        # 递归继承：沿 inheritsFrom 链合并（libraries / arguments / mainClass / assets / javaVersion）
        self.base_id = self.name
        seen = {self.name}
        inherit = self.json.get("inheritsFrom")
        chain = []
        while inherit and inherit != self.name and inherit not in seen:
            seen.add(inherit)
            try:
                parent = McVersion(self.mc_folder, inherit)
                if parent.json:
                    chain.append(parent)
                    inherit = parent.json.get("inheritsFrom")
                    if inherit:
                        self.base_id = inherit
                else:
                    break
            except Exception as e:
                self.error = f"继承版本 {inherit} 解析失败：{e}"
                break
        if chain:
            merged = dict(chain[-1].json)  # 从最底层开始
            for parent in reversed(chain[:-1]):
                merged.update(parent.json)
            merged.update(self.json)
            # libraries 合并（子在前，父在后，去重）
            libs = list(self.json.get("libraries") or [])
            for parent in chain:
                for l in (parent.json.get("libraries") or []):
                    if l not in libs:
                        libs.append(l)
            merged["libraries"] = libs
            merged["inheritsFrom"] = chain[-1].name
            self.json = merged
            self.parent = chain[0]

    @property
    def main_class(self):
        return self.json.get("mainClass") if self.json else None

    @property
    def release_time(self):
        try:
            return self.json.get("releaseTime", "")
        except Exception:
            return ""

    @property
    def type(self):
        return self.json.get("type", "unknown") if self.json else "error"

    @property
    def assets_id(self):
        if not self.json:
            return None
        a = self.json.get("assets")
        if a:
            return a
        # 1.5.2 及更早：legacy
        try:
            major = self.major
            if major <= 5:
                return "legacy"
            if major == 6:
                return "pre-1.6"
        except Exception:
            pass
        return None

    @property
    def major(self):
        """从（继承链最底层的）版本 id 提取主版本号，如 1.21.1 -> 21。失败返回 0。"""
        m = re.match(r"(\d+)\.(\d+)", getattr(self, "base_id", None) or self.name)
        if m:
            return int(m.group(2))
        return 0

    @property
    def java_requirement(self):
        """Java 版本需求（优先版本 JSON 的 javaVersion，否则按 MC 版本推断）。"""
        if self.json:
            jv = self.json.get("javaVersion")
            if isinstance(jv, dict):
                major = jv.get("majorVersion") or jv.get("major")
                if major:
                    return int(major)
            elif jv:
                return int(jv)
        if self.major >= 21:
            return 21
        if self.major >= 18:
            return 17
        if self.major >= 16:
            return 16
        return 8

    @property
    def loaders(self):
        """识别加载器标记：Forge/Fabric/NeoForge/Quilt/OptiFine/LiteLoader。"""
        if not self.json:
            return []
        tags = []
        libs = " ".join(l.get("name", "") for l in self.json.get("libraries") or [])
        lower_name = self.name.lower()
        if "forge" in lower_name or "forge" in libs.lower():
            tags.append("Forge")
        if "neoforge" in lower_name or "neoforge" in libs.lower():
            tags.append("NeoForge")
        if "fabric" in lower_name or "fabric" in libs.lower():
            tags.append("Fabric")
        if "quilt" in lower_name or "quilt" in libs.lower():
            tags.append("Quilt")
        if "optifine" in lower_name or "optifine" in libs.lower():
            tags.append("OptiFine")
        if "liteloader" in lower_name or "liteloader" in libs.lower():
            tags.append("LiteLoader")
        return tags

    @property
    def is_old(self):
        """旧版格式：使用 minecraftArguments（1.12 及更早）。"""
        return bool(self.json and self.json.get("minecraftArguments"))

    def get_libraries(self):
        """返回合并后的库列表（已过滤 rules，含 natives 标记）。"""
        if not self.json:
            return []
        os_name = "windows"
        result = []
        for lib in filter_rules(self.json.get("libraries") or [], os_name):
            name = lib.get("name", "")
            downloads = lib.get("downloads") or {}
            artifact = downloads.get("artifact") or {}
            classifiers = downloads.get("classifiers") or {}
            # --- natives 识别 ---
            # 1) 现代格式：独立 natives 条目，name 带第 4 段 classifier（如 org.lwjgl:lwjgl:3.3.3:natives-windows）
            # 2) 旧格式：libraries.natives.windows = "natives-windows"（可能含 ${arch}）
            parts = name.split(":")
            classifier = parts[3] if len(parts) >= 4 else None
            native = None
            if classifier and classifier.startswith("natives-"):
                native = classifier
            else:
                native_obj = lib.get("natives") or {}
                native = native_obj.get("windows")
                if native:
                    native = native.replace("${arch}", ARCH)
            # 目标平台之外的分支（arm64/x86）不纳入（rules 已过滤，此处兜底）
            if native and native not in ("natives-windows",):
                if native.endswith("-arm64") or native.endswith("-x86"):
                    continue
            path = None
            url = None
            sha1 = None
            size = None
            if native:
                # natives 库：优先 classifiers/artifact 下载信息
                entry = classifiers.get(native)
                if not entry and classifier:
                    entry = artifact
                if entry:
                    path = entry.get("path")
                    url = entry.get("url")
                    sha1 = entry.get("sha1")
                    size = entry.get("size")
                if not path and name:
                    path = maven_path(name, native)
            else:
                if artifact.get("path"):
                    path = artifact["path"]
                elif name:
                    path = maven_path(name)
                url = artifact.get("url")
                sha1 = artifact.get("sha1")
                size = artifact.get("size")
            if not path:
                continue
            result.append({
                "name": name,
                "path": path,
                "local": os.path.join(self.mc_folder, "libraries", path.replace("/", os.sep)),
                "url": url,
                "sha1": sha1,
                "size": size,
                "natives": native,
            })
        return result


def maven_path(name, classifier=None):
    """com.mojang:authlib:1.5.21 -> com/mojang/authlib/1.5.21/authlib-1.5.21.jar
    org.lwjgl:lwjgl:3.3.3:natives-windows -> .../lwjgl-3.3.3-natives-windows.jar"""
    parts = name.split(":")
    if len(parts) < 3:
        return None
    group, artifact, version = parts[0], parts[1], parts[2]
    if len(parts) >= 4 and not classifier:
        classifier = parts[3]
    base = f"{group.replace('.', '/')}/{artifact}/{version}/{artifact}-{version}"
    if classifier:
        return f"{base}-{classifier}.jar"
    return base + ".jar"


# ---------- 扫描 ----------

def scan_versions(mc_folder):
    """扫描 versions 目录，返回版本信息列表（含错误信息）。"""
    result = []
    versions_dir = os.path.join(mc_folder, "versions")
    if not os.path.isdir(versions_dir):
        return result
    for entry in sorted(os.listdir(versions_dir), key=str.lower, reverse=True):
        name = entry
        json_path = version_json_path(mc_folder, name)
        if not os.path.isfile(json_path):
            continue
        try:
            v = McVersion(mc_folder, name)
            if v.error:
                result.append({"name": name, "state": "error", "error": v.error})
                continue
            result.append({
                "name": v.name,
                "type": v.type,
                "release_time": v.release_time,
                "major": v.major,
                "loaders": v.loaders,
                "java": v.java_requirement,
                "state": "ok",
            })
        except Exception as e:
            result.append({"name": name, "state": "error", "error": str(e)})
    return result


def get_version(mc_folder, name):
    v = McVersion(mc_folder, name)
    return v
