// Package system 负责系统环境探测：物理内存 / 自动内存推荐 / 系统 UI 语言。
// 使用标准库 syscall 调用公开的 Win32 API（GlobalMemoryStatusEx / GetUserDefaultUILanguage）。
package system

import (
	"strings"
	"syscall"
	"unsafe"
)

// memoryStatusEx 与 Windows MEMORYSTATUSEX 布局一致（uint32 后跟 uint64 天然对齐）。
type memoryStatusEx struct {
	dwLength                uint32
	dwMemoryLoad            uint32
	ullTotalPhys            uint64
	ullAvailPhys            uint64
	ullTotalPageFile        uint64
	ullAvailPageFile        uint64
	ullTotalVirtual         uint64
	ullAvailVirtual         uint64
	ullAvailExtendedVirtual uint64
}

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	procGlobalMemoryStatusEx = kernel32.NewProc("GlobalMemoryStatusEx")
	procGetUserDefaultUILanguage = kernel32.NewProc("GetUserDefaultUILanguage")
)

// MemoryStatus 返回 {total_mb, available_mb, load}，失败返回 nil。
func MemoryStatus() map[string]int {
	var stat memoryStatusEx
	stat.dwLength = uint32(unsafe.Sizeof(stat))
	r, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&stat)))
	if r == 0 {
		return nil
	}
	return map[string]int{
		"total_mb":     int(stat.ullTotalPhys / (1024 * 1024)),
		"available_mb": int(stat.ullAvailPhys / (1024 * 1024)),
		"load":         int(stat.dwMemoryLoad),
	}
}

// TotalMemoryMB 系统物理内存总量（MB）。
func TotalMemoryMB() int {
	if st := MemoryStatus(); st != nil {
		return st["total_mb"]
	}
	return 0
}

// AvailableMemoryMB 立即可用的物理内存（MB）。
func AvailableMemoryMB() int {
	if st := MemoryStatus(); st != nil {
		return st["available_mb"]
	}
	return 0
}

// SuggestMemoryMB 根据物理内存推荐 -Xmx（MB）。
func SuggestMemoryMB(totalMB int) int {
	if totalMB == 0 {
		totalMB = TotalMemoryMB()
	}
	if totalMB == 0 {
		return 4096
	}
	var mb int
	switch {
	case totalMB >= 65536:
		mb = 8192
	case totalMB >= 32768:
		mb = 6144
	case totalMB >= 16384:
		mb = 4096
	case totalMB >= 8192:
		mb = 3072
	case totalMB >= 6144:
		mb = 2560
	case totalMB >= 4096:
		mb = 2048
	case totalMB >= 2048:
		mb = 1024
	default:
		mb = 768
	}
	if mb > int(float64(totalMB)*0.45) {
		mb = int(float64(totalMB) * 0.45)
	}
	if mb < 512 {
		mb = 512
	}
	return mb
}

// ResolveRAMMB 按配置决定本次启动内存；返回 (mb, source)。source 为 "auto"/"manual"。
func ResolveRAMMB(autoMemory bool, ramGB float64, totalMB int) (int, string) {
	if autoMemory {
		return SuggestMemoryMB(totalMB), "auto"
	}
	return int(ramGB * 1024), "manual"
}

// ---------- 系统语言 ----------

// Windows LANGID -> MC 语言资源名
var winLangMap = map[uint32]string{
	0x0804: "zh_cn", 0x0004: "zh_cn", 0x1004: "zh_cn",
	0x0404: "zh_tw", 0x0C04: "zh_tw", 0x1404: "zh_tw", 0x7C04: "zh_tw",
	0x0411: "ja_jp",
	0x0412: "ko_kr",
	0x0409: "en_us",
}

// SystemLanguage 探测系统 UI 语言，返回 MC 语言资源名（如 zh_cn / en_us）。
func SystemLanguage() string {
	r, _, _ := procGetUserDefaultUILanguage.Call()
	langid := uint32(r)
	if v, ok := winLangMap[langid]; ok {
		return v
	}
	return "en_us"
}

// ResolveLanguage 把配置里的语言设置解析为具体资源名；"none" 表示不干预。
func ResolveLanguage(setting string) string {
	setting = strings.TrimSpace(setting)
	if setting == "" {
		setting = "system"
	}
	if setting == "none" {
		return "none"
	}
	if setting == "system" {
		return SystemLanguage()
	}
	return setting
}
