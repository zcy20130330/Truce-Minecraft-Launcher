# -*- coding: utf-8 -*-
"""内存优化（行为对标 Sysinternals RamMap 的 Empty Standby List / Empty Working Sets）。

说明：本模块**按公开的 Windows API 自行实现**，未参考或复制任何第三方源码
（RamMap 闭源、PCL/IPC 相关实现均未取用），仅追求行为一致：
把 Windows 占着当缓存的「待机列表」和「进程工作集」回收成自由内存，供 JVM 分配。

原理：Windows 会把暂时用不到的页留在「待机列表（Standby List）」和各进程的
「工作集（Working Set）」里当缓存，这部分对 JVM 而言是不可直接分配的——
RamMap 的 Empty Working Sets / Empty Standby List 就是把它们回收给自由内存。

实现的四个动作（均为未文档化的 Windows 内存管理接口，逐个独立容错）：
  1. MemoryEmptyWorkingSets  清空所有进程工作集
  2. MemoryFlushModifiedList 回写并释放修改页列表
  3. MemoryPurgeStandbyList  清除待机列表（收益最大的一步）
  4. MemoryPurgeLowPriorityStandbyList 清除低优先级待机列表
另加 SetProcessWorkingSetSizeEx 逐进程修剪，覆盖系统级命令够不到的进程。

**绝大多数动作需要管理员权限**（SeIncreaseQuotaPrivilege /
SeProfileSingleProcessPrivilege / SeDebugPrivilege）；非管理员时本模块会
明确报告「无权执行」而不会影响启动流程。
"""
import sys

from . import system

# NtSetSystemInformation 的信息类
SYSTEM_MEMORY_LIST_INFORMATION = 80

# SYSTEM_MEMORY_LIST_COMMAND
MEMORY_EMPTY_WORKING_SETS = 2
MEMORY_FLUSH_MODIFIED_LIST = 3
MEMORY_PURGE_STANDBY_LIST = 4
MEMORY_PURGE_LOW_PRIORITY_STANDBY_LIST = 5

NT_STATUS_MESSAGES = {
    0xC0000022: "权限不足（需要管理员运行）",
    0xC0000005: "系统拒绝（当前 Windows 版本限制该命令）",
    0xC0000001: "接口不支持（Windows 版本差异）",
    0xC000000D: "参数无效",
}

_STEP_NAMES = {
    MEMORY_EMPTY_WORKING_SETS: "清空进程工作集",
    MEMORY_FLUSH_MODIFIED_LIST: "回写修改页列表",
    MEMORY_PURGE_STANDBY_LIST: "清除待机列表",
    MEMORY_PURGE_LOW_PRIORITY_STANDBY_LIST: "清除低优先级待机列表",
}


# ---------- 环境判定 ----------

def supported():
    """仅 Windows 有这套接口。"""
    return sys.platform == "win32"


def is_admin():
    """当前进程是否具备管理员令牌。"""
    if not supported():
        return False
    try:
        import ctypes
        return bool(ctypes.windll.shell32.IsUserAnAdmin())
    except Exception:
        return False


# ---------- 权限提升 ----------

def _enable_privilege(name):
    """启用指定特权，返回 (ok, 错误描述)。失败不影响后续其它步骤。"""
    try:
        import ctypes
        from ctypes import wintypes

        class LUID(ctypes.Structure):
            _fields_ = [("LowPart", wintypes.DWORD), ("HighPart", wintypes.LONG)]

        class LUID_AND_ATTRIBUTES(ctypes.Structure):
            _fields_ = [("Luid", LUID), ("Attributes", wintypes.DWORD)]

        class TOKEN_PRIVILEGES(ctypes.Structure):
            _fields_ = [("PrivilegeCount", wintypes.DWORD),
                        ("Privileges", LUID_AND_ATTRIBUTES * 1)]

        advapi32 = ctypes.windll.advapi32
        token = wintypes.HANDLE()
        if not advapi32.OpenProcessToken(ctypes.windll.kernel32.GetCurrentProcess(),
                                         wintypes.DWORD(0x0028),  # TOKEN_ADJUST_PRIVILEGES | TOKEN_QUERY
                                         ctypes.byref(token)):
            return False, "OpenProcessToken 失败"
        try:
            luid = LUID()
            if not advapi32.LookupPrivilegeValueW(None, name, ctypes.byref(luid)):
                return False, "特权不存在"
            tp = TOKEN_PRIVILEGES()
            tp.PrivilegeCount = 1
            tp.Privileges[0].Luid = luid
            tp.Privileges[0].Attributes = 0x00000002  # SE_PRIVILEGE_ENABLED
            if not advapi32.AdjustTokenPrivileges(token, False, ctypes.byref(tp),
                                                  ctypes.sizeof(TOKEN_PRIVILEGES), None, None):
                return False, "AdjustTokenPrivileges 失败"
            return True, "已启用"
        finally:
            ctypes.windll.kernel32.CloseHandle(token)
    except Exception as e:
        return False, str(e)


def enable_privileges(log=None):
    """开启优化所需的特权，返回 [(特权名, ok, 说明)]。"""
    out = []
    for p in ("SeIncreaseQuotaPrivilege", "SeProfileSingleProcessPrivilege", "SeDebugPrivilege"):
        ok, msg = _enable_privilege(p)
        out.append((p, ok, msg))
        if log:
            log(f"[优化] 特权 {p}：{'已启用' if ok else '未启用（' + msg + '）'}")
    return out


# ---------- 核心动作 ----------

def _nt_set_system_memory_list(command):
    """调用 NtSetSystemInformation(SystemMemoryListInformation, command)。"""
    import ctypes

    ntdll = ctypes.windll.ntdll
    cmd = ctypes.c_ulong(command)
    status = ntdll.NtSetSystemInformation(
        ctypes.c_int(SYSTEM_MEMORY_LIST_INFORMATION),
        ctypes.byref(cmd),
        ctypes.c_ulong(ctypes.sizeof(cmd)),
    )
    return status


def _trim_process_working_sets():
    """逐进程修剪工作集（补充手段），返回 (成功进程数, 进程总数)。"""
    import ctypes
    from ctypes import wintypes

    psapi = ctypes.windll.psapi
    kernel32 = ctypes.windll.kernel32

    arr = (wintypes.DWORD * 4096)()
    needed = wintypes.DWORD()
    if not psapi.EnumProcesses(ctypes.byref(arr), ctypes.sizeof(arr), ctypes.byref(needed)):
        return 0, 0
    count = min(needed.value // ctypes.sizeof(wintypes.DWORD), 4096)

    size_t_max = ctypes.c_size_t(-1).value
    ok = 0
    total = 0
    for i in range(count):
        pid = arr[i]
        if not pid or pid == kernel32.GetCurrentProcessId():
            continue
        total += 1
        handle = None
        try:
            handle = kernel32.OpenProcess(0x0100 | 0x1000,  # SET_QUOTA | QUERY_LIMITED_INFORMATION
                                          False, wintypes.DWORD(pid))
            if not handle:
                continue
            if kernel32.SetProcessWorkingSetSizeEx(handle, size_t_max, size_t_max, 0):
                ok += 1
        except Exception:
            continue
        finally:
            if handle:
                kernel32.CloseHandle(handle)
    return ok, total


def optimize(log=None, per_process=True):
    """执行一次完整优化。返回 {before_mb, after_mb, freed_mb, steps, admin, ok}。"""
    result = {
        "supported": supported(),
        "admin": is_admin(),
        "before_mb": None,
        "after_mb": None,
        "freed_mb": 0,
        "steps": [],
        "ok": False,
    }
    if not supported():
        result["error"] = "内存优化仅支持 Windows"
        return result

    if log:
        log("[优化] 开始内存优化（RamMap 式清理）")

    enable_privileges(log)

    before = system.available_memory_mb() or 0
    result["before_mb"] = before

    # 系统级命令（收益主要来自待机列表清理）
    for cmd in (MEMORY_EMPTY_WORKING_SETS,
                MEMORY_FLUSH_MODIFIED_LIST,
                MEMORY_PURGE_STANDBY_LIST,
                MEMORY_PURGE_LOW_PRIORITY_STANDBY_LIST):
        name = _STEP_NAMES[cmd]
        try:
            status = _nt_set_system_memory_list(cmd)
            ok = status >= 0
            mid = system.available_memory_mb() or before
            result["steps"].append({
                "name": name,
                "ok": ok,
                "status": f"0x{status & 0xFFFFFFFF:08X}",
                "detail": "成功" if ok else NT_STATUS_MESSAGES.get(status, "失败"),
                "freed_mb": max(0, mid - before),
            })
            if log:
                log(f"[优化] {name}：{'成功' if ok else NT_STATUS_MESSAGES.get(status, '失败')}")
        except Exception as e:
            result["steps"].append({"name": name, "ok": False, "detail": str(e), "freed_mb": 0})
            if log:
                log(f"[优化] {name}：异常 {e}")

    # 逐进程修剪工作集
    if per_process:
        try:
            done, total = _trim_process_working_sets()
            result["steps"].append({
                "name": "逐进程修剪工作集",
                "ok": done > 0,
                "detail": f"{done}/{total} 个进程",
                "freed_mb": 0,
            })
            if log:
                log(f"[优化] 逐进程修剪工作集：{done}/{total} 个进程")
        except Exception as e:
            result["steps"].append({"name": "逐进程修剪工作集", "ok": False, "detail": str(e), "freed_mb": 0})

    after = system.available_memory_mb() or 0
    result["after_mb"] = after
    result["freed_mb"] = max(0, after - before)
    result["ok"] = any(s["ok"] for s in result["steps"])
    if log:
        log(f"[优化] 完成：可用内存 {before} MB → {after} MB"
            f"（释放 {result['freed_mb']} MB）")
    return result


# ---------- 触发判定 ----------

def check_pressure(need_mb, reserve_mb=1024, ratio_threshold=0.2):
    """判断是否需要优化。

    need_mb: 本次启动要申请的 -Xmx
    reserve_mb: 除游戏外留给系统的余量（默认 1GB）
    ratio_threshold: 可用内存占总量的最低比例（默认 20%）

    返回 {"needed", "reason", "available_mb", "total_mb", "required_mb"}
    """
    st = system.memory_status()
    if not st:
        return {"needed": False, "reason": "内存信息读取失败，跳过优化",
                "available_mb": None, "total_mb": None, "required_mb": need_mb}
    avail, total = st["available_mb"], st["total_mb"]
    required = int(need_mb) + int(reserve_mb)

    if avail < required:
        return {"needed": True, "available_mb": avail, "total_mb": total,
                "required_mb": required,
                "reason": f"可用 {avail} MB < 需要 {required} MB（Xmx {need_mb} + 系统预留 {reserve_mb}）"}
    if total and avail < total * ratio_threshold:
        return {"needed": True, "available_mb": avail, "total_mb": total,
                "required_mb": required,
                "reason": f"可用比例过低：{avail}/{total} MB（低于 {int(ratio_threshold * 100)}%）"}
    return {"needed": False, "available_mb": avail, "total_mb": total,
            "required_mb": required,
            "reason": f"可用 {avail}/{total} MB，充足"}
