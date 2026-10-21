# -*- coding: utf-8 -*-
"""库文件 / 资源文件下载补全（对标 PCL DlClientFix / DlSource* 简化版）。

镜像规则（同 PCL）：
  - libraries: libraries.minecraft.net / piston-data / piston-meta -> bmclapi2.bangbang93.com/maven、/libraries
  - assets:    resources.download.minecraft.net -> bmclapi2.bangbang93.com/assets
  - meta:      piston-meta / launcher.mojang / launchermeta.mojang -> bmclapi2.bangbang93.com
"""
import hashlib
import os
import ssl
import urllib.request

USER_AGENT = "PCL-Prototype/0.1 (Minecraft Launcher Prototype)"


def make_opener():
    ctx = ssl.create_default_context()
    return urllib.request.build_opener(urllib.request.HTTPSHandler(context=ctx))


OPENER = make_opener()


def http_get(url, timeout=30):
    req = urllib.request.Request(url, headers={"User-Agent": USER_AGENT})
    with OPENER.open(req, timeout=timeout) as r:
        return r.read()


def http_download(url, dest, timeout=60):
    req = urllib.request.Request(url, headers={"User-Agent": USER_AGENT})
    tmp = dest + ".part"
    os.makedirs(os.path.dirname(dest), exist_ok=True)
    with OPENER.open(req, timeout=timeout) as r:
        with open(tmp, "wb") as f:
            while True:
                chunk = r.read(65536)
                if not chunk:
                    break
                f.write(chunk)
    os.replace(tmp, dest)


def sha1_of(path):
    h = hashlib.sha1()
    with open(path, "rb") as f:
        while True:
            chunk = f.read(65536)
            if not chunk:
                break
            h.update(chunk)
    return h.hexdigest()


def file_ok(path, sha1=None, size=None):
    """文件是否通过校验：存在、大小匹配（若有）、SHA1 匹配（若有）。"""
    if not os.path.isfile(path):
        return False
    if size:
        try:
            if os.path.getsize(path) != int(size):
                return False
        except Exception:
            pass
    if sha1:
        try:
            return sha1_of(path).lower() == sha1.lower()
        except Exception:
            return False
    return True


# ---------- 下载源 ----------

def _mirror(original, kind):
    if kind == "library":
        cands = [
            original.replace("https://piston-data.mojang.com", "https://bmclapi2.bangbang93.com/maven")
                    .replace("https://piston-meta.mojang.com", "https://bmclapi2.bangbang93.com/maven")
                    .replace("https://libraries.minecraft.net", "https://bmclapi2.bangbang93.com/maven"),
            original.replace("https://piston-data.mojang.com", "https://bmclapi2.bangbang93.com/libraries")
                    .replace("https://piston-meta.mojang.com", "https://bmclapi2.bangbang93.com/libraries")
                    .replace("https://libraries.minecraft.net", "https://bmclapi2.bangbang93.com/libraries"),
        ]
        return cands
    if kind == "asset":
        return [original.replace("https://resources.download.minecraft.net", "https://bmclapi2.bangbang93.com/assets")]
    # meta
    return [original.replace("https://piston-data.mojang.com", "https://bmclapi2.bangbang93.com")
                    .replace("https://piston-meta.mojang.com", "https://bmclapi2.bangbang93.com")
                    .replace("https://launcher.mojang.com", "https://bmclapi2.bangbang93.com")
                    .replace("https://launchermeta.mojang.com", "https://bmclapi2.bangbang93.com")]


def url_order(original, kind, prefer_mirror=True):
    mirrors = _mirror(original, kind)
    if prefer_mirror:
        return mirrors + [original]
    return [original] + mirrors


def download_file(original, dest, kind="library", sha1=None, size=None, prefer_mirror=True, progress=None):
    """按顺序尝试各下载源，下载并校验。成功返回 True，全部失败返回 False。"""
    for url in url_order(original, kind, prefer_mirror):
        try:
            http_download(url, dest)
            if file_ok(dest, sha1, size):
                return True
            # 校验失败：删掉重试下一个源
            try:
                os.remove(dest)
            except Exception:
                pass
        except Exception as e:
            print(f"[download] 失败 {url}: {e}")
            continue
    return False


# ---------- 缺失文件列表 ----------

def missing_libraries(mc_folder, libs, check_files=True):
    """返回需要下载的库文件列表 [{urls, local, sha1, size, name}]。"""
    missing = []
    for lib in libs:
        local = lib["local"]
        if not check_files and os.path.isfile(local):
            continue
        if file_ok(local, lib.get("sha1"), lib.get("size")):
            continue
        # 构造官方 URL
        if lib.get("url"):
            official = lib["url"]
        else:
            # 按本地路径拼 libraries.minecraft.net
            rel = os.path.relpath(local, os.path.join(mc_folder, "libraries")).replace(os.sep, "/")
            official = "https://libraries.minecraft.net/" + rel
        missing.append({
            "name": lib["name"],
            "local": local,
            "url": official,
            "sha1": lib.get("sha1"),
            "size": lib.get("size"),
        })
    return missing


def missing_assets(mc_folder, index_json, check_files=True):
    """根据资源索引返回缺失的资源文件列表。index_json 为 assets/indexes/<id>.json 内容。"""
    missing = []
    objects = (index_json or {}).get("objects", {})
    virtual = bool((index_json or {}).get("virtual"))
    for key, obj in objects.items():
        h = obj.get("hash", "")
        size = obj.get("size", 0)
        if virtual:
            local = os.path.join(mc_folder, "assets", "virtual", "legacy", key.replace("/", os.sep))
        else:
            local = os.path.join(mc_folder, "assets", "objects", h[:2], h)
        if not check_files and os.path.isfile(local):
            continue
        if file_ok(local, h, size):
            continue
        official = f"https://resources.download.minecraft.net/{h[:2]}/{h}"
        missing.append({"key": key, "local": local, "url": official, "sha1": h, "size": size})
    return missing


def download_many(items, kind="library", prefer_mirror=True, progress_cb=None, thread_count=1):
    """批量下载。progress_cb(done, total, name)。返回 (ok_count, fail_count)。"""
    total = len(items)
    ok = fail = 0
    if total == 0:
        return 0, 0
    if thread_count <= 1:
        for it in items:
            if download_file(it["url"], it["local"], kind, it.get("sha1"), it.get("size"), prefer_mirror):
                ok += 1
            else:
                fail += 1
            if progress_cb:
                progress_cb(ok + fail, total, it.get("name") or it.get("key"))
        return ok, fail

    from concurrent.futures import ThreadPoolExecutor
    done = [0]

    def work(it):
        nonlocal ok, fail
        if download_file(it["url"], it["local"], kind, it.get("sha1"), it.get("size"), prefer_mirror):
            ok += 1
        else:
            fail += 1
        done[0] += 1
        if progress_cb:
            progress_cb(done[0], total, it.get("name") or it.get("key"))

    with ThreadPoolExecutor(max_workers=thread_count) as ex:
        ex.map(work, items)
    if progress_cb:
        progress_cb(total, total, "完成")
    return ok, fail
