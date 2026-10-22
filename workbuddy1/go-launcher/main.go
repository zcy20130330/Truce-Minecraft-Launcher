// Truce Launcher（Go 版）命令行入口：scan / build / launch。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"syscall"

	"truce-launcher/internal/auth"
	"truce-launcher/internal/config"
	"truce-launcher/internal/gui"
	"truce-launcher/internal/java"
	"truce-launcher/internal/launch"
	"truce-launcher/internal/system"
	"truce-launcher/internal/versions"
)

func main() {
	// 无子命令时启动原生桌面界面；带 scan/build/launch 时走命令行（验证用）。
	if len(os.Args) < 2 {
		gui.Run()
		return
	}
	// windowsgui 子系统的程序默认没有控制台，命令行模式先补一个，
	// 否则 scan/build/launch 的输出会丢失。
	ensureConsole()
	cmd := os.Args[1]
	switch cmd {
	case "scan":
		runScan(os.Args[2:])
	case "build":
		runBuild(os.Args[2:])
	case "launch":
		runLaunch(os.Args[2:])
	default:
		fmt.Printf("未知命令：%s\n", cmd)
		os.Exit(1)
	}
}

// ensureConsole 让以命令行子命令方式运行的 windowsgui 程序也能正常输出。
// 优先附加到父进程控制台（cmd / PowerShell），失败则新建一个控制台，
// 随后重建 os.Stdout / Stderr / Stdin 句柄。
func ensureConsole() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	attach := kernel32.NewProc("AttachConsole")
	r, _, _ := attach.Call(^uintptr(0)) // ATTACH_PARENT_PROCESS = -1
	if r == 0 {
		alloc := kernel32.NewProc("AllocConsole")
		alloc.Call()
	}
	if h, e := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE); e == nil && h != 0 {
		os.Stdout = os.NewFile(uintptr(h), "stdout")
	}
	if h, e := syscall.GetStdHandle(syscall.STD_ERROR_HANDLE); e == nil && h != 0 {
		os.Stderr = os.NewFile(uintptr(h), "stderr")
	}
	if h, e := syscall.GetStdHandle(syscall.STD_INPUT_HANDLE); e == nil && h != 0 {
		os.Stdin = os.NewFile(uintptr(h), "stdin")
	}
}

func loadCfg(configPath string) *config.Config {
	return config.Load(configPath)
}

func runScan(args []string) {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	configPath := fs.String("config", "config.json", "配置文件路径")
	mcFolder := fs.String("mc", "", ".minecraft 目录")
	_ = fs.Parse(args)
	cfg := loadCfg(*configPath)
	if *mcFolder == "" {
		*mcFolder = cfg.McFolder
	}
	list := versions.ScanVersions(*mcFolder)
	b, _ := json.MarshalIndent(list, "", "  ")
	fmt.Println(string(b))
}

func runBuild(args []string) {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	configPath := fs.String("config", "config.json", "配置文件路径")
	mcFolder := fs.String("mc", "", ".minecraft 目录")
	versionName := fs.String("version", "", "版本名（必填）")
	_ = fs.Parse(args)
	cfg := loadCfg(*configPath)
	if *mcFolder == "" {
		*mcFolder = cfg.McFolder
	}
	if *versionName == "" {
		fmt.Println("错误：build 需要 --version")
		os.Exit(1)
	}

	v := versions.GetVersion(*mcFolder, *versionName)
	if v.Error != "" {
		fmt.Println("版本解析错误:", v.Error)
		os.Exit(1)
	}
	fmt.Printf("=== 版本 %s ===\n", v.Name)
	fmt.Printf("主类: %s\n", v.MainClass())
	fmt.Printf("类型: %s  加载器: %s  Java需求: %d\n", v.Type(), strings.Join(v.Loaders(), ","), v.JavaRequirement())
	fmt.Printf("资源索引: %s  主版本号: %d\n", v.AssetsID(), v.Major())

	// Java 选择
	javaPath := cfg.JavaPath
	if javaPath == "" {
		if chosen := java.SelectJava(java.DetectJava(), v.JavaRequirement()); chosen != nil {
			javaPath = chosen["path"].(string)
		}
	}
	fmt.Printf("Java: %s\n", javaPath)

	// 内存
	ramMB, src := system.ResolveRAMMB(cfg.AutoMemory, cfg.RamGB, 0)
	fmt.Printf("内存: -Xmx%dm (%s)\n", ramMB, src)

	libs := v.GetLibraries()
	nativeCount := 0
	for _, l := range libs {
		if l.Natives != "" {
			nativeCount++
		}
	}
	fmt.Printf("库数量: %d (其中 natives: %d)\n", len(libs), nativeCount)

	login := auth.OfflineLogin(cfg.Username).ToMap()
	cmd, gameDir := launch.BuildCommand(*mcFolder, v, cfg, login, javaPath, ramMB)
	fmt.Printf("游戏目录: %s\n", gameDir)
	fmt.Printf("启动命令 (%d 段):\n", len(cmd))
	for i, a := range cmd {
		fmt.Printf("  [%d] %s\n", i, a)
	}
}

func runLaunch(args []string) {
	fs := flag.NewFlagSet("launch", flag.ContinueOnError)
	configPath := fs.String("config", "config.json", "配置文件路径")
	mcFolder := fs.String("mc", "", ".minecraft 目录")
	versionName := fs.String("version", "", "版本名（必填）")
	maxSeconds := fs.Int("max-seconds", 0, "超时强制结束（0=不限制，仅用于验证）")
	out := fs.String("out", "", "把游戏日志写入该文件（可选）")
	_ = fs.Parse(args)
	cfg := loadCfg(*configPath)
	if *mcFolder == "" {
		*mcFolder = cfg.McFolder
	}
	if *versionName == "" {
		fmt.Println("错误：launch 需要 --version")
		os.Exit(1)
	}

	var logFn func(string)
	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			fmt.Println("无法创建日志文件:", err)
			os.Exit(1)
		}
		defer f.Close()
		logFn = func(s string) {
			ts := fmt.Sprintf("%s\n", s)
			fmt.Print(ts)
			f.WriteString(ts)
		}
	} else {
		logFn = func(s string) { fmt.Println(s) }
	}

	ok, msg := launch.Launch(*mcFolder, *versionName, cfg, logFn, *maxSeconds)
	fmt.Printf("\n=== 结果: ok=%v msg=%s ===\n", ok, msg)
}
