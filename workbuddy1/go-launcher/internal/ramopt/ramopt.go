// Package ramopt 内存优化（行为对标 Sysinternals RamMap 的 Empty Standby List / Empty Working Sets）。
//
// 说明：本模块按公开的 Windows API 自行实现，未参考或复制任何第三方源码（RamMap 闭源、
// PCL/IPC 相关实现均未取用），仅追求行为一致：把 Windows 占着当缓存的「待机列表」和
// 「进程工作集」回收成自由内存，供 JVM 分配。
package ramopt

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"

	"truce-launcher/internal/system"
)

// NtSetSystemInformation 的信息类
const systemMemoryListInformation = 80

// SYSTEM_MEMORY_LIST_COMMAND
const (
	memoryEmptyWorkingSets             = 2
	memoryFlushModifiedList            = 3
	memoryPurgeStandbyList             = 4
	memoryPurgeLowPriorityStandbyList  = 5
)

var ntStatusMessages = map[uint32]string{
	0xC0000022: "权限不足（需要管理员运行）",
	0xC0000005: "系统拒绝（当前 Windows 版本限制该命令）",
	0xC0000001: "接口不支持（Windows 版本差异）",
	0xC000000D: "参数无效",
}

var stepNames = map[uint32]string{
	memoryEmptyWorkingSets:            "清空进程工作集",
	memoryFlushModifiedList:           "回写修改页列表",
	memoryPurgeStandbyList:            "清除待机列表",
	memoryPurgeLowPriorityStandbyList: "清除低优先级待机列表",
}

var (
	ntdll          = windows.NewLazySystemDLL("ntdll.dll")
	procNtSetSysMem = ntdll.NewProc("NtSetSystemInformation")
	shell32        = windows.NewLazySystemDLL("shell32.dll")
	procIsAdmin    = shell32.NewProc("IsUserAnAdmin")
	psapi          = windows.NewLazySystemDLL("psapi.dll")
	procEnumProc   = psapi.NewProc("EnumProcesses")
)

// Supported 仅 Windows 有这套接口。
func Supported() bool {
	return true
}

// IsAdmin 当前进程是否具备管理员令牌。
func IsAdmin() bool {
	r, _, _ := procIsAdmin.Call()
	return r != 0
}

func ntSetSystemMemoryList(command uint32) (status int32) {
	var cmd uint32 = command
	r, _, _ := procNtSetSysMem.Call(
		uintptr(systemMemoryListInformation),
		uintptr(unsafe.Pointer(&cmd)),
		uintptr(unsafe.Sizeof(cmd)),
	)
	return int32(r)
}

func enablePrivilege(name string) bool {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &token); err != nil {
		return false
	}
	defer token.Close()
	var luid windows.LUID
	if err := windows.LookupPrivilegeValue(nil, windows.StringToUTF16Ptr(name), &luid); err != nil {
		return false
	}
	var tp windows.Tokenprivileges
	tp.PrivilegeCount = 1
	tp.Privileges[0].Luid = luid
	tp.Privileges[0].Attributes = windows.SE_PRIVILEGE_ENABLED
	if err := windows.AdjustTokenPrivileges(token, false, &tp, 0, nil, nil); err != nil {
		return false
	}
	return true
}

func enablePrivileges(log func(string)) {
	for _, p := range []string{"SeIncreaseQuotaPrivilege", "SeProfileSingleProcessPrivilege", "SeDebugPrivilege"} {
		ok := enablePrivilege(p)
		if log != nil {
			if ok {
				log(fmt.Sprintf("[优化] 特权 %s：已启用", p))
			} else {
				log(fmt.Sprintf("[优化] 特权 %s：未启用", p))
			}
		}
	}
}

func trimProcessWorkingSets() (done, total int) {
	const max = 4096
	pids := make([]uint32, max)
	var needed uint32
	r, _, _ := procEnumProc.Call(
		uintptr(unsafe.Pointer(&pids[0])),
		uintptr(len(pids)*4),
		uintptr(unsafe.Pointer(&needed)),
	)
	if r == 0 {
		return 0, 0
	}
	count := int(needed) / 4
	if count > max {
		count = max
	}
	sizeMax := uintptr(^uintptr(0))
	self := uint32(windows.GetCurrentProcessId())
	for i := 0; i < count; i++ {
		pid := pids[i]
		if pid == 0 || pid == self {
			continue
		}
		total++
		h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
		if err != nil {
			continue
		}
		if windows.SetProcessWorkingSetSizeEx(h, sizeMax, sizeMax, 0) == nil {
			done++
		}
		windows.CloseHandle(h)
	}
	return done, total
}

// Step 是单个优化步骤的结果。
type Step struct {
	Name     string
	OK       bool
	Status   string
	Detail   string
	FreedMB  int
}

// Result 是一次完整优化的结果。
type Result struct {
	Supported bool
	Admin     bool
	BeforeMB  int
	AfterMB   int
	FreedMB   int
	Steps     []Step
	OK        bool
}

// Optimize 执行一次完整优化。perProcess 为 true 时额外逐进程修剪工作集。
func Optimize(log func(string), perProcess bool) Result {
	result := Result{
		Supported: Supported(),
		Admin:     IsAdmin(),
		BeforeMB:  system.AvailableMemoryMB(),
	}
	if !result.Supported {
		return result
	}
	if log != nil {
		log("[优化] 开始内存优化（RamMap 式清理）")
	}
	enablePrivileges(log)
	before := result.BeforeMB

	for _, cmd := range []uint32{
		memoryEmptyWorkingSets,
		memoryFlushModifiedList,
		memoryPurgeStandbyList,
		memoryPurgeLowPriorityStandbyList,
	} {
		name := stepNames[cmd]
		status := ntSetSystemMemoryList(cmd)
		ok := status >= 0
		mid := system.AvailableMemoryMB()
		if mid == 0 {
			mid = before
		}
		step := Step{
			Name:    name,
			OK:      ok,
			Status:  fmt.Sprintf("0x%08X", uint32(status)),
			Detail:  mapOr(uint32(status), ntStatusMessages, "失败"),
			FreedMB: maxInt(0, mid-before),
		}
		result.Steps = append(result.Steps, step)
		if log != nil {
			if ok {
				log(fmt.Sprintf("[优化] %s：成功", name))
			} else {
				log(fmt.Sprintf("[优化] %s：%s", name, step.Detail))
			}
		}
	}

	if perProcess {
		done, total := trimProcessWorkingSets()
		result.Steps = append(result.Steps, Step{
			Name:   "逐进程修剪工作集",
			OK:     done > 0,
			Detail: fmt.Sprintf("%d/%d 个进程", done, total),
		})
		if log != nil {
			log(fmt.Sprintf("[优化] 逐进程修剪工作集：%d/%d 个进程", done, total))
		}
	}

	result.AfterMB = system.AvailableMemoryMB()
	result.FreedMB = maxInt(0, result.AfterMB-before)
	result.OK = result.AfterMB > before || anyStepOK(result.Steps)
	if log != nil {
		log(fmt.Sprintf("[优化] 完成：可用内存 %d MB → %d MB（释放 %d MB）", before, result.AfterMB, result.FreedMB))
	}
	return result
}

// Pressure 是是否需要优化的判定结果。
type Pressure struct {
	Needed       bool
	Reason       string
	AvailableMB  int
	TotalMB      int
	RequiredMB   int
}

// CheckPressure 判断是否需要优化。
func CheckPressure(needMB, reserveMB int, ratioThreshold float64) Pressure {
	st := system.MemoryStatus()
	if st == nil {
		return Pressure{Needed: false, Reason: "内存信息读取失败，跳过优化", AvailableMB: 0, TotalMB: 0, RequiredMB: needMB}
	}
	avail, total := st["available_mb"], st["total_mb"]
	required := needMB + reserveMB
	if avail < required {
		return Pressure{
			Needed:      true,
			AvailableMB: avail, TotalMB: total, RequiredMB: required,
			Reason: fmt.Sprintf("可用 %d MB < 需要 %d MB（Xmx %d + 系统预留 %d）", avail, required, needMB, reserveMB),
		}
	}
	if total > 0 && float64(avail) < float64(total)*ratioThreshold {
		return Pressure{
			Needed:      true,
			AvailableMB: avail, TotalMB: total, RequiredMB: required,
			Reason: fmt.Sprintf("可用比例过低：%d/%d MB（低于 %d%%）", avail, total, int(ratioThreshold*100)),
		}
	}
	return Pressure{
		Needed:      false,
		AvailableMB: avail, TotalMB: total, RequiredMB: required,
		Reason: fmt.Sprintf("可用 %d/%d MB，充足", avail, total),
	}
}

func mapOr(key uint32, m map[uint32]string, def string) string {
	if v, ok := m[key]; ok {
		return v
	}
	return def
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func anyStepOK(steps []Step) bool {
	for _, s := range steps {
		if s.OK {
			return true
		}
	}
	return false
}
