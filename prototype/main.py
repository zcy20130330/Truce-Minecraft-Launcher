# -*- coding: utf-8 -*-
"""PCL-Prototype 启动器原型 — 本地 Web 服务入口。

用法：
    python main.py
浏览器自动打开 http://127.0.0.1:8766

纯标准库实现（Python 3.9+），零第三方依赖。
"""
import json
import os
import sys
import threading
import webbrowser
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from launcher import config, launch, ramopt, system, versions, java  # noqa: E402

HOST = "127.0.0.1"
PORT = 8766  # 避开 AIQQBot Dashboard 的 8765
WEB_DIR = os.path.join(os.path.dirname(os.path.abspath(__file__)), "web")
MIME = {
    ".html": "text/html; charset=utf-8",
    ".css": "text/css; charset=utf-8",
    ".js": "application/javascript; charset=utf-8",
    ".json": "application/json; charset=utf-8",
    ".png": "image/png",
    ".ico": "image/x-icon",
    ".svg": "image/svg+xml",
}


def send_json(handler, obj, code=200):
    body = json.dumps(obj, ensure_ascii=False).encode("utf-8")
    handler.send_response(code)
    handler.send_header("Content-Type", "application/json; charset=utf-8")
    handler.send_header("Content-Length", str(len(body)))
    handler.send_header("Cache-Control", "no-store")
    handler.end_headers()
    handler.wfile.write(body)


def send_file(handler, path):
    if not os.path.isfile(path):
        handler.send_response(404)
        handler.end_headers()
        return
    ext = os.path.splitext(path)[1].lower()
    body = open(path, "rb").read()
    handler.send_response(200)
    handler.send_header("Content-Type", MIME.get(ext, "application/octet-stream"))
    handler.send_header("Content-Length", str(len(body)))
    handler.end_headers()
    handler.wfile.write(body)


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *a):
        pass  # 静默访问日志

    # ---- 静态资源 ----
    def do_GET(self):
        path = self.path.split("?", 1)[0]
        try:
            if path == "/" or path == "/index.html":
                return send_file(self, os.path.join(WEB_DIR, "index.html"))
            if path == "/style.css":
                return send_file(self, os.path.join(WEB_DIR, "style.css"))
            if path == "/app.js":
                return send_file(self, os.path.join(WEB_DIR, "app.js"))
            if path == "/api/config":
                return send_json(self, config.load())
            if path == "/api/versions":
                mc_folder = self._query_param("folder") or config.get("mc_folder")
                return send_json(self, versions.scan_versions(mc_folder))
            if path == "/api/java":
                return send_json(self, java.detect_java())
            if path == "/api/system":
                # 物理内存 / 推荐内存 / 系统语言，供设置面板展示
                return send_json(self, self._system_info())
            if path == "/api/ramopt":
                # 内存状态 + 是否建议优化
                return send_json(self, self._ramopt_info())
            if path == "/api/state":
                return send_json(self, launch.current_state())
            if path == "/api/logs":
                since = int(self._query_param("since") or 0)
                lines, last = launch.LOG.snapshot(since)
                return send_json(self, {"lines": lines, "last": last})
            if path == "/api/stop":
                ok = launch.stop_game()
                return send_json(self, {"ok": ok})
            if path == "/api/ramopt_optimize":
                # 手动触发一次内存优化
                return send_json(self, ramopt.optimize(log=None))
            if path == "/api/scan_folder":
                # 扫描常见 MC 文件夹，帮助用户选择
                return send_json(self, self._scan_common_folders())
            self.send_response(404)
            self.end_headers()
        except Exception as e:
            send_json(self, {"error": str(e)}, 500)

    # ---- 写操作 ----
    def do_POST(self):
        path = self.path.split("?", 1)[0]
        try:
            if path == "/api/config":
                body = self._read_body()
                cfg = config.save(body)
                return send_json(self, {"ok": True, "config": cfg})
            if path == "/api/launch":
                body = self._read_body()
                mc_folder = body.get("mc_folder") or config.get("mc_folder")
                version_name = body.get("version")
                if not version_name:
                    return send_json(self, {"ok": False, "error": "未指定版本"}, 400)
                # 合并前端传入的覆盖设置
                cfg = dict(config.load())
                cfg.update({k: v for k, v in body.items() if k != "version" and k != "mc_folder"})
                config.save(cfg)

                def worker():
                    try:
                        launch.launch(mc_folder, version_name, cfg)
                    except Exception as e:
                        launch.log("[错误] 启动异常：" + str(e))
                        launch.STATE.update(status="error", error=str(e))

                t = threading.Thread(target=worker, daemon=True)
                t.start()
                return send_json(self, {"ok": True, "started": True})
            self.send_response(404)
            self.end_headers()
        except Exception as e:
            send_json(self, {"error": str(e)}, 500)

    # ---- 工具 ----
    def _read_body(self):
        length = int(self.headers.get("Content-Length") or 0)
        if length == 0:
            return {}
        raw = self.rfile.read(length)
        try:
            return json.loads(raw.decode("utf-8"))
        except Exception:
            return {}

    def _query_param(self, key):
        from urllib.parse import parse_qs, urlparse
        qs = parse_qs(urlparse(self.path).query)
        vals = qs.get(key)
        return vals[0] if vals else None

    def _ramopt_info(self):
        cfg = config.load()
        ram_mb = system.resolve_ram_mb(cfg)[0]
        info = ramopt.check_pressure(ram_mb, int(cfg.get("memory_reserve_mb", 1024)))
        info.update({
            "supported": ramopt.supported(),
            "admin": ramopt.is_admin(),
            "enabled": cfg.get("memory_optimize", True),
        })
        return info

    def _system_info(self):
        total = system.total_memory_mb()
        return {
            "total_memory_mb": total,
            "suggested_mb": system.suggest_memory_mb(total),
            "system_language": system.system_language(),
            "auto_memory": config.get("auto_memory", True),
            "language": config.get("language", "system"),
        }

    def _scan_common_folders(self):
        import glob
        candidates = []
        ap = os.environ.get("APPDATA", "")
        if ap:
            candidates.append(os.path.join(ap, ".minecraft"))
        candidates += [
            os.path.join(os.path.expanduser("~"), ".minecraft"),
            r"D:\Minecraft",
            r"D:\MC",
            r"E:\Minecraft",
            r"F:\Minecraft",
        ]
        found = []
        for c in candidates:
            if os.path.isdir(os.path.join(c, "versions")):
                found.append(c)
        return {"folders": sorted(set(found))}


def main():
    server = ThreadingHTTPServer((HOST, PORT), Handler)
    url = f"http://{HOST}:{PORT}"
    print("=" * 50)
    print("  PCL-Prototype 启动器原型")
    print(f"  服务地址：{url}")
    print("  关闭窗口或 Ctrl+C 退出")
    print("=" * 50)
    threading.Timer(0.8, lambda: webbrowser.open(url)).start()
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        print("\n已退出")


if __name__ == "__main__":
    main()
