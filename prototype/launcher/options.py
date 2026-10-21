# -*- coding: utf-8 -*-
"""游戏内 options.txt 维护（当前用于自动设置语言）。

MC 的语言存在 <游戏目录>/options.txt 的 lang 项里，如 `lang:zh_cn`。
启动前写入即可生效——注意必须在进程启动**之前**写，游戏退出时会自行覆写该文件。
"""
import os

OPTION_KEY = "lang"
_ENCODINGS = ("utf-8", "gbk", "latin-1")


def _detect(raw):
    """按候选编码尝试解码，返回 (text, encoding)。"""
    for enc in _ENCODINGS:
        try:
            return raw.decode(enc), enc
        except UnicodeDecodeError:
            continue
    return None, None


def read_option(game_dir, key=OPTION_KEY):
    """读取某一项的值，文件不存在或没有该项返回 None。"""
    path = os.path.join(game_dir, "options.txt")
    if not os.path.isfile(path):
        return None
    try:
        with open(path, "rb") as f:
            text, _ = _detect(f.read())
    except Exception:
        return None
    if text is None:
        return None
    for line in text.splitlines():
        if line.startswith(key + ":"):
            return line[len(key) + 1:].strip()
    return None


def write_option(game_dir, key, value):
    """幂等写入：已有该项则替换，没有则追加。返回 (ok, 说明)。"""
    os.makedirs(game_dir, exist_ok=True)
    path = os.path.join(game_dir, "options.txt")

    lines = []
    encoding = "utf-8"
    if os.path.isfile(path):
        try:
            with open(path, "rb") as f:
                raw = f.read()
            text, encoding = _detect(raw)
            if text is None:
                return False, f"无法识别 options.txt 的编码：{path}"
            lines = text.splitlines()
        except Exception as e:
            return False, f"读取 options.txt 失败：{e}"

    target = f"{key}:{value}"
    hit = False
    out = []
    for line in lines:
        if line.startswith(key + ":"):
            if hit:          # 重复项（异常文件）：丢弃多余的行
                continue
            out.append(target)
            hit = True
        else:
            out.append(line)
    if not hit:
        out.append(target)

    try:
        with open(path, "w", encoding=encoding, newline="\n") as f:
            f.write("\n".join(out) + "\n")
    except Exception as e:
        return False, f"写入 options.txt 失败：{e}"
    return True, f"{target}"


def apply_language(game_dir, language):
    """设置游戏语言。language 为 "none" 时不干预游戏内现有设置。"""
    if not language or language == "none":
        current = read_option(game_dir)
        if current:
            return False, f"保持游戏现有语言 lang:{current}（可在设置里改成跟随系统）"
        return False, "未设置语言（保持游戏内现有设置）"
    old = read_option(game_dir)
    ok, msg = write_option(game_dir, OPTION_KEY, language)
    if not ok:
        return False, msg
    if old == language:
        return True, f"语言已是 {msg}"
    return True, f"已设置游戏语言 {msg}" + (f"（原 {old}）" if old else "")
