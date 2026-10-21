# -*- coding: utf-8 -*-
"""配置管理：读写 config.json（原型版，纯标准库）。"""
import json
import os

CONFIG_PATH = os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "config.json")

DEFAULT_CONFIG = {
    "mc_folder": os.path.join(os.environ.get("APPDATA", os.path.expanduser("~")), ".minecraft"),
    "java_path": "",              # 空 = 自动选择
    "ram_gb": 4.0,                # -Xmx 内存（auto_memory 关闭时用）
    "auto_memory": True,          # 自动分配内存：按物理内存推荐 Xmx
    "memory_optimize": True,      # 内存不足时自动优化（RamMap 式清理）
    "memory_reserve_mb": 1024,    # 除游戏外留给系统的余量
    "language": "system",         # 游戏语言：system/zh_cn/zh_tw/en_us/ja_jp/ko_kr/none
    "username": "Player",         # 离线登录名
    "window_width": 854,
    "window_height": 480,
    "fullscreen": False,
    "version_isolate": False,     # 版本隔离（gameDir 指向版本文件夹）
    "jvm_args": "",               # 自定义 JVM 参数
    "game_args": "",              # 自定义游戏参数
    "server": "",                 # 自动进服 ip:port
    "download_source": 0,         # 0=镜像优先 1=官方优先
    "check_files": True,          # 启动前补全文件
}

_config_cache = None


def load() -> dict:
    global _config_cache
    if _config_cache is not None:
        return _config_cache
    cfg = dict(DEFAULT_CONFIG)
    try:
        if os.path.exists(CONFIG_PATH):
            with open(CONFIG_PATH, "r", encoding="utf-8") as f:
                cfg.update(json.load(f))
    except Exception as e:
        print(f"[config] 读取配置失败，使用默认值：{e}")
    _config_cache = cfg
    return cfg


def save(cfg: dict) -> dict:
    global _config_cache
    merged = dict(DEFAULT_CONFIG)
    merged.update(cfg)
    _config_cache = merged
    try:
        with open(CONFIG_PATH, "w", encoding="utf-8") as f:
            json.dump(merged, f, ensure_ascii=False, indent=2)
    except Exception as e:
        print(f"[config] 保存配置失败：{e}")
    return merged


def get(key: str, default=None):
    return load().get(key, default)
