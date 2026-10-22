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
	"truce-launcher/internal/ramopt"
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
	case "probe":
		runProbe(os.Args[2:])
	case "scan":
		runScan(os.Args[2:])
	case "build":
		runBuild(os.Args[2:])
	case "launch":
		runLaunch(os.Args[2:])
	case "optimize":
		runOptimize(os.Args[2:])
	case "javacheck":
		runJavaCheck(os.Args[2:])
	case "javafind":
		runJavaFind(os.Args[2:])
	case "javainstall":
		runJavaInstall(os.Args[2:])
	default:
		fmt.Printf("未知命令：%s\n", cmd)
		os.Exit(1)
	}
}

// ensureConsole 让以命令行子命令方式运行的 windowsgui 程序也能正常输出。
// 优先附加到父进程控制台（cmd / PowerShell），失败则新建一个控制台，
// 随后重建 os.Stdout / Stderr / Stdin 句柄。
//
// 注意：当本程序被 GUI 前端以“重定向标准输出”的方式调用时，stdout 已经是
// 一个管道。此时绝不能 AttachConsole/AllocConsole——那会弹出一个黑框，并把
// os.Stdout 指向新控制台，导致前端读不到任何输出。所以先检测句柄类型。
func ensureConsole() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	if h, err := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE); err == nil &&
		h != 0 && h != syscall.InvalidHandle && h != ^syscall.Handle(0) {
		// stdout 已经接好了（控制台 / 文件重定向 / 管道），不要再动句柄：
		// 一旦 AttachConsole 或 AllocConsole 成功，标准句柄会被换成控制台句柄，
		// 原本重定向到文件的输出会全部跑到不可见的控制台里（静默丢日志）。
		return
	}
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

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

// runProbe 一次性返回桌面界面初始化所需的全部信息（单行 JSON）：
// 配置、已安装 Java、物理内存、系统语言、版本列表。
// UI 启动时只需一次进程调用即可完成首屏渲染。
func runProbe(args []string) {
	fs := flag.NewFlagSet("probe", flag.ContinueOnError)
	configPath := fs.String("config", "config.json", "配置文件路径")
	mcFolder := fs.String("mc", "", ".minecraft 目录")
	_ = fs.Parse(args)

	cfg := loadCfg(*configPath)
	if *mcFolder == "" {
		*mcFolder = cfg.McFolder
	}

	javaDir := java.DefaultJavaDir()
	javaList := java.DetectJava(javaDir)

	// 选中逻辑：配置里指定的优先；否则按首个已装版本的需求自动匹配。
	var selected map[string]interface{}
	if cfg.JavaPath != "" {
		if info, ok := java.GetJavaVersion(cfg.JavaPath); ok {
			selected = info
		}
	}
	firstRequired := 0
	allVersions := versions.ScanVersions(*mcFolder)
	for _, v := range allVersions {
		if n, _ := v["java"].(int); n > 0 {
			firstRequired = n
			break
		}
	}
	if selected == nil {
		selected = java.Recommend(javaList, firstRequired).Best
	}

	total := system.TotalMemoryMB()
	out := map[string]interface{}{
		"config_path":      *configPath,
		"config":            cfg,
		"mc_folder":         *mcFolder,
		"mc_folder_exists":  dirExists(*mcFolder),
		"java":              javaList,
		"java_dir":          javaDir,
		"java_required":     firstRequired,
		"selected_java":     selected,
		"system_language":   system.SystemLanguage(),
		"versions":          allVersions,
		"memory": map[string]int{
			"total_mb":     total,
			"available_mb": system.AvailableMemoryMB(),
			"suggest_mb":   system.SuggestMemoryMB(total),
		},
	}
	b, err := json.Marshal(out)
	if err != nil {
		fmt.Println(`{"error":"probe 序列化失败"}`)
		return
	}
	fmt.Println(string(b))
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
		if chosen := java.SelectJava(java.DetectJava(java.DefaultJavaDir()), v.JavaRequirement()); chosen != nil {
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

// runOptimize 执行一次内存优化。过程日志实时打印到 stdout（供 UI 流式显示），
// 最后追加一行 "###RESULT###{json}" 供前端解析结果摘要。
func runOptimize(args []string) {
	fs := flag.NewFlagSet("optimize", flag.ContinueOnError)
	configPath := fs.String("config", "config.json", "配置文件路径")
	perProcess := fs.Bool("per-process", false, "额外逐进程修剪工作集")
	_ = fs.Parse(args)
	_ = *configPath // 预留：后续可读配置决定是否逐进程修剪

	var logs []string
	res := ramopt.Optimize(func(s string) {
		logs = append(logs, s)
		fmt.Println(s)
	}, *perProcess)

	summary, _ := json.Marshal(map[string]interface{}{
		"ok":        res.OK,
		"admin":     res.Admin,
		"before_mb": res.BeforeMB,
		"after_mb":  res.AfterMB,
		"freed_mb":  res.FreedMB,
		"steps":     len(res.Steps),
	})
	fmt.Println("###RESULT###" + string(summary))
}

// runJavaCheck 校验单个 java.exe 并返回其版本（供 UI 手动指定 Java 时使用）。
func runJavaCheck(args []string) {
	fs := flag.NewFlagSet("javacheck", flag.ContinueOnError)
	path := fs.String("path", "", "java.exe 路径（必填）")
	_ = fs.Parse(args)

	if *path == "" {
		fmt.Println(`{"ok":false,"error":"缺少 --path"}`)
		return
	}
	info, ok := java.GetJavaVersion(*path)
	if !ok {
		b, _ := json.Marshal(map[string]interface{}{"ok": false, "path": *path})
		fmt.Println(string(b))
		return
	}
	info["ok"] = true
	b, _ := json.Marshal(info)
	fmt.Println(string(b))
}

// runJavaFind 针对某个已安装版本给出 Java 选择结论：
// 该版本要求哪个 Java、本机哪个最合适、是否建议自动安装。
func runJavaFind(args []string) {
	fs := flag.NewFlagSet("javafind", flag.ContinueOnError)
	configPath := fs.String("config", "config.json", "配置文件路径")
	mcFolder := fs.String("mc", "", ".minecraft 目录")
	versionName := fs.String("version", "", "版本名")
	_ = fs.Parse(args)

	cfg := loadCfg(*configPath)
	if *mcFolder == "" {
		*mcFolder = cfg.McFolder
	}

	required := 0
	if *versionName != "" {
		v := versions.GetVersion(*mcFolder, *versionName)
		if v != nil && v.Error == "" {
			required = v.JavaRequirement()
		}
	}

	javaDir := java.DefaultJavaDir()
	list := java.DetectJava(javaDir)
	rec := java.Recommend(list, required)

	b, _ := json.Marshal(map[string]interface{}{
		"version":       *versionName,
		"java_dir":      javaDir,
		"available":     list,
		"required":      rec.Required,
		"best":          rec.Best,
		"need_install":  rec.NeedInstall,
		"install_major": rec.InstallMajor,
		"reason":        rec.Reason,
	})
	fmt.Println(string(b))
}

// runJavaInstall 自动下载并安装指定主版本的 JRE。
// 进度与结果按行输出，供前端流式解析：
//
//	###PROGRESS###{"stage":"download","percent":42,"message":"..."}
//	###RESULT###{"ok":true,"java_path":"...","major":21}
func runJavaInstall(args []string) {
	fs := flag.NewFlagSet("javainstall", flag.ContinueOnError)
	major := fs.Int("major", 21, "要安装的 Java 主版本")
	javaDir := fs.String("dir", "", "安装目录（默认 %LOCALAPPDATA%\\TruceLauncher\\java）")
	_ = fs.Parse(args)

	emit := func(line string) { fmt.Println(line) }
	report := func(p java.InstallProgress) {
		b, _ := json.Marshal(map[string]interface{}{
			"stage":   p.Stage,
			"percent": p.Percent,
			"message": p.Message,
		})
		emit("###PROGRESS###" + string(b))
	}

	path, err := java.InstallJRE(*major, *javaDir, report)
	if err != nil {
		b, _ := json.Marshal(map[string]interface{}{
			"ok": false, "major": *major, "error": err.Error(),
		})
		emit("###RESULT###" + string(b))
		os.Exit(1)
	}
	b, _ := json.Marshal(map[string]interface{}{
		"ok": true, "major": *major, "java_path": path,
	})
	emit("###RESULT###" + string(b))
}
