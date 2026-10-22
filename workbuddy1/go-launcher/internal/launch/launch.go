// Package launch 启动链路核心：参数拼装 + natives 解压 + 进程管理 + 日志流式输出。
// 对应 prototype 的 launcher/launch.py。
package launch

import (
	"archive/zip"
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"truce-launcher/internal/auth"
	"truce-launcher/internal/config"
	"truce-launcher/internal/download"
	"truce-launcher/internal/java"
	"truce-launcher/internal/options"
	"truce-launcher/internal/ramopt"
	"truce-launcher/internal/system"
	"truce-launcher/internal/versions"
)

// ---------- 辅助：JSON map 访问 ----------

func asMap(v interface{}) (map[string]interface{}, bool) {
	m, ok := v.(map[string]interface{})
	return m, ok
}
func asSlice(v interface{}) ([]interface{}, bool) {
	s, ok := v.([]interface{})
	return s, ok
}
func asString(v interface{}) (string, bool) {
	s, ok := v.(string)
	return s, ok
}

// ---------- 路径 ----------

// NativesFolder 返回 natives 解压目录。
func NativesFolder(mcFolder, versionName string) string {
	return filepath.Join(mcFolder, "versions", versionName, versionName+"-natives")
}

// GameDirFor 游戏目录：版本隔离时指向版本文件夹，否则指向 .minecraft。
func GameDirFor(mcFolder string, v *versions.Version, cfg *config.Config) string {
	if cfg.VersionIsolate {
		return v.Path
	}
	return mcFolder
}

// ---------- natives 解压 ----------

// ExtractNatives 把 natives 库中的 dll/so/dylib 解压到 <版本>-natives 目录。
func ExtractNatives(mcFolder, versionName string, libs []versions.Library, log func(string)) []string {
	target := NativesFolder(mcFolder, versionName)
	os.MkdirAll(target, 0o755)
	extracted := []string{}
	for _, lib := range libs {
		if lib.Natives == "" {
			continue
		}
		if _, err := os.Stat(lib.Local); err != nil {
			log(fmt.Sprintf("[natives] 缺失：%s", lib.Name))
			continue
		}
		r, err := zip.OpenReader(lib.Local)
		if err != nil {
			log(fmt.Sprintf("[natives] 解压失败 %s: %v", lib.Name, err))
			continue
		}
		for _, entry := range r.File {
			fn := entry.Name
			if strings.Contains(fn, "META-INF") {
				continue
			}
			ext := strings.ToLower(filepath.Ext(fn))
			if ext != ".dll" && ext != ".so" && ext != ".dylib" {
				continue
			}
			out := filepath.Join(target, filepath.Base(fn))
			if fi, err := os.Stat(out); err == nil && fi.Size() == int64(entry.UncompressedSize64) {
				continue
			}
			rc, err := entry.Open()
			if err != nil {
				continue
			}
			data, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				continue
			}
			if err := os.WriteFile(out, data, 0o644); err == nil {
				extracted = append(extracted, out)
			}
		}
		r.Close()
	}
	log(fmt.Sprintf("[natives] 解压完成，共 %d 个文件 → %s", len(extracted), target))
	return extracted
}

// ---------- 参数拼装 ----------

func buildClasspath(mcFolder, versionName string, libs []versions.Library) string {
	var parts []string
	for _, lib := range libs {
		if lib.Natives != "" {
			continue
		}
		parts = append(parts, lib.Local)
	}
	versionJar := filepath.Join(mcFolder, "versions", versionName, versionName+".jar")
	if _, err := os.Stat(versionJar); err == nil {
		parts = append(parts, versionJar)
	}
	return strings.Join(parts, ";")
}

func buildJVMArgs(mcFolder, versionName string, libs []versions.Library, ramMB int, nativesDir, customJVM string) []string {
	args := []string{fmt.Sprintf("-Xmx%dm", ramMB)}
	args = append(args, "-Djava.library.path="+nativesDir)
	args = append(args, "-cp", buildClasspath(mcFolder, versionName, libs))
	args = append(args, "-Dlog4j2.formatMsgNoLookups=true")
	if customJVM != "" {
		args = append(args, strings.Fields(customJVM)...)
	}
	return args
}

func flattenArgValue(val interface{}) []string {
	switch t := val.(type) {
	case []interface{}:
		out := make([]string, 0, len(t))
		for _, v := range t {
			out = append(out, fmt.Sprint(v))
		}
		return out
	case string:
		return []string{t}
	default:
		return []string{fmt.Sprint(val)}
	}
}

func matchOSRule(rule interface{}, quickplayServer bool) bool {
	r, ok := rule.(map[string]interface{})
	if !ok {
		return true
	}
	action := "allow"
	if a, ok := asString(r["action"]); ok {
		action = a
	}
	if osRule, ok := asMap(r["os"]); ok {
		if n, ok := asString(osRule["name"]); ok && n != "unknown" && n != "windows" {
			return action != "allow"
		}
	}
	if features, ok := asMap(r["features"]); ok {
		for key, wantRaw := range features {
			want, _ := wantRaw.(bool)
			if key == "is_demo_user" {
				if want {
					return action != "allow"
				}
			} else if key == "has_custom_resolution" {
				continue
			} else if key == "has_quick_plays_support" || strings.HasPrefix(key, "is_quick_play") {
				enabled := false
				if key != "is_quick_play_singleplayer" && key != "is_quick_play_realms" {
					enabled = quickplayServer
				}
				if (action == "allow") != enabled {
					return false
				}
			} else {
				return action != "allow"
			}
		}
	}
	return action == "allow"
}

func buildGameArgs(v *versions.Version, gameDir, assetsDir, assetIndex string, login map[string]string,
	width, height int, fullscreen bool, customGame, server string) []string {
	args := []string{}
	jsonObj := v.JSON
	hasNew := false
	if argObj, ok := asMap(jsonObj["arguments"]); ok {
		if ga, ok := asSlice(argObj["game"]); ok {
			hasNew = true
			for _, item := range ga {
				switch t := item.(type) {
				case string:
					args = append(args, t)
				case map[string]interface{}:
					rules, _ := asSlice(t["rules"])
					pass := true
					for _, r := range rules {
						if !matchOSRule(r, server != "") {
							pass = false
							break
						}
					}
					if pass {
						args = append(args, flattenArgValue(t["value"])...)
					}
				}
			}
		}
	}
	if !hasNew {
		if ma, ok := asString(jsonObj["minecraftArguments"]); ok {
			argString := ma + fmt.Sprintf(" --width %d --height %d", width, height)
			args = strings.Fields(argString)
		}
	}

	tokens := map[string]string{
		"${auth_player_name}":           login["name"],
		"${version_name}":               v.Name,
		"${game_directory}":             gameDir,
		"${assets_root}":                assetsDir,
		"${assets_index_name}":          valueOr(assetIndex, "legacy"),
		"${auth_uuid}":                  login["uuid"],
		"${auth_access_token}":          login["access_token"],
		"${auth_xuid}":                  "0",
		"${access_token}":               login["access_token"],
		"${user_type}":                  login["user_type"],
		"${version_type}":               valueOr(v.Type(), "release"),
		"${resolution_width}":           strconv.Itoa(width),
		"${resolution_height}":          strconv.Itoa(height),
		"${launcher_name}":             "Truce-Launcher",
		"${launcher_version}":           "0.1",
		"${user_properties}":           "{}",
		"${clientid}":                   login["uuid"],
		"${xuid}":                       "0",
		"${demo_user}":                  "false",
		"${assets_version_name}":        valueOr(assetIndex, "legacy"),
		"${quickPlayPath}":              filepath.Join(gameDir, "quickPlay.json"),
		"${quickPlaySingleplayer}":      "",
		"${quickPlayMultiplayer}":       server,
		"${quickPlayRealms}":            "",
	}
	for i, a := range args {
		if repl, ok := tokens[a]; ok {
			args[i] = repl
		}
	}

	if fullscreen {
		args = append(args, "--fullscreen")
	}
	if customGame != "" {
		args = append(args, strings.Fields(customGame)...)
	}
	if server != "" {
		if v.Major() >= 20 || v.ReleaseTime() >= "2023-04-05" {
			if !containsStr(args, "--quickPlayMultiplayer") {
				args = append(args, "--quickPlayMultiplayer", server)
			}
		} else {
			host := server
			port := "25565"
			if idx := strings.Index(server, ":"); idx >= 0 {
				host = server[:idx]
				port = server[idx+1:]
			}
			args = append(args, "--server", host, "--port", port)
		}
	}
	return args
}

func valueOr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func containsStr(slice []string, s string) bool {
	for _, x := range slice {
		if x == s {
			return true
		}
	}
	return false
}

// BuildCommand 生成完整启动命令列表。ramMB 由调用方解析好后传入。
func BuildCommand(mcFolder string, v *versions.Version, cfg *config.Config, login map[string]string, javaPath string, ramMB int) ([]string, string) {
	libs := v.GetLibraries()
	gameDir := GameDirFor(mcFolder, v, cfg)
	natives := NativesFolder(mcFolder, v.Name)
	assetsDir := filepath.Join(mcFolder, "assets")
	assetIndex := v.AssetsID()
	width := cfg.WindowWidth
	if width == 0 {
		width = 854
	}
	height := cfg.WindowHeight
	if height == 0 {
		height = 480
	}
	fullscreen := cfg.Fullscreen
	customJVM := cfg.JvmArgs
	customGame := cfg.GameArgs
	server := cfg.Server

	jvm := buildJVMArgs(mcFolder, v.Name, libs, ramMB, natives, customJVM)
	mainClass := v.MainClass()
	if mainClass == "" {
		mainClass = "net.minecraft.client.main.Main"
	}
	game := buildGameArgs(v, gameDir, assetsDir, assetIndex, login, width, height, fullscreen, customGame, server)
	return append(jvm, append([]string{mainClass}, game...)...), gameDir
}

// ---------- 启动 ----------

// scanLines 流式读取子进程输出。
func scanLines(r io.Reader, log func(string)) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 4*1024*1024)
	for scanner.Scan() {
		log(scanner.Text())
	}
}

// Launch 启动入口。maxSeconds>0 时到时强制结束子进程。返回 (ok, msg)。
func Launch(mcFolder, versionName string, cfg *config.Config, logFn func(string), maxSeconds int) (bool, string) {
	log := logFn
	if log == nil {
		log = func(string) {}
	}
	log("========================================================")
	log("启动版本：" + versionName)

	v := versions.GetVersion(mcFolder, versionName)
	if v.Error != "" {
		log("[错误] " + v.Error)
		return false, v.Error
	}
	log(fmt.Sprintf("主类：%s  加载器：%s  Java需求：%d", v.MainClass(), strings.Join(v.Loaders(), ","), v.JavaRequirement()))

	// 2) Java 选择
	javaPath := cfg.JavaPath
	if javaPath != "" {
		if _, err := os.Stat(javaPath); err != nil {
			javaPath = ""
		}
	}
	if javaPath == "" {
		list := java.DetectJava()
		chosen := java.SelectJava(list, v.JavaRequirement())
		if chosen == nil {
			log("[错误] 未检测到 Java，请安装或手动指定 java.exe 路径")
			return false, "未找到合适的 Java"
		}
		javaPath = chosen["path"].(string)
		log(fmt.Sprintf("自动选择 Java：%s（版本 %v）", javaPath, chosen["version"]))
	}
	if _, err := os.Stat(javaPath); err != nil {
		msg := fmt.Sprintf("Java 不存在：%s", javaPath)
		log("[错误] " + msg)
		return false, msg
	}

	// 2.5) 版本主 jar 检查
	versionJar := filepath.Join(mcFolder, "versions", versionName, versionName+".jar")
	if _, err := os.Stat(versionJar); err != nil {
		msg := fmt.Sprintf("版本主文件缺失：%s\n请先用其它启动器装好 %s 再启动", versionJar, versionName)
		log("[错误] " + msg)
		return false, msg
	}

	// 2.6) 内存 + 低内存自动优化
	ramMB, ramSource := system.ResolveRAMMB(cfg.AutoMemory, cfg.RamGB, 0)
	if ramSource == "auto" {
		log(fmt.Sprintf("[内存] 自动分配：-Xmx%dm（物理内存 %d MB）", ramMB, system.TotalMemoryMB()))
	} else {
		log(fmt.Sprintf("[内存] 手动分配：-Xmx%dm", ramMB))
	}
	if cfg.MemoryOptimize {
		pressure := ramopt.CheckPressure(ramMB, cfg.MemoryReserveMB, 0.2)
		if pressure.Needed {
			log("[优化] 检测到内存不足：" + pressure.Reason)
			res := ramopt.Optimize(log, true)
			if !res.OK {
				log("[优化] 优化未生效（建议用管理员身份运行本启动器）")
			}
		} else {
			log("[优化] 无需优化：" + pressure.Reason)
		}
	}

	// 3) 补全文件（库 + 资源）
	libs := v.GetLibraries()
	preferMirror := cfg.DownloadSource == 0
	checkFiles := cfg.CheckFiles
	missingLibs := download.MissingLibraries(mcFolder, libs, checkFiles)
	if len(missingLibs) > 0 {
		log(fmt.Sprintf("[下载] 缺失 %d 个支持库文件，开始下载…", len(missingLibs)))
		ok, fail := download.DownloadMany(missingLibs, "library", preferMirror, func(d, t int, n string) {
			log(fmt.Sprintf("[下载] 库 %d/%d %s", d, t, n))
		})
		if fail > 0 {
			log(fmt.Sprintf("[警告] %d 个库文件下载失败，可能无法启动", fail))
		}
		_ = ok
	} else {
		log("[下载] 支持库文件完整，无需下载")
	}

	assetIndex := v.AssetsID()
	var indexJSON map[string]interface{}
	if assetIndex != "" {
		indexPath := filepath.Join(mcFolder, "assets", "indexes", assetIndex+".json")
		if data, err := os.ReadFile(indexPath); err == nil {
			// ignore parse error; treated as no index
			_ = json.Unmarshal(data, &indexJSON)
		} else {
			log(fmt.Sprintf("[下载] 资源索引缺失：%s.json（暂不自动下载，不影响核心启动）", assetIndex))
		}
	}
	if indexJSON != nil {
		missingAssets := download.MissingAssets(mcFolder, indexJSON, checkFiles)
		if len(missingAssets) > 0 {
			log(fmt.Sprintf("[下载] 缺失 %d 个资源文件，开始下载…", len(missingAssets)))
			ok, fail := download.DownloadMany(missingAssets, "asset", preferMirror, func(d, t int, n string) {
				log(fmt.Sprintf("[下载] 资源 %d/%d", d, t))
			})
			if fail > 0 {
				log(fmt.Sprintf("[警告] %d 个资源文件下载失败", fail))
			}
			_ = ok
		} else {
			log("[下载] 资源文件完整")
		}
	}

	// 4) 解压 natives
	ExtractNatives(mcFolder, versionName, libs, log)

	// 5) 登录（离线）
	login := auth.OfflineLogin(cfg.Username).ToMap()
	log(fmt.Sprintf("离线登录：%s  UUID=%s", login["name"], login["uuid"]))

	// 5.5) 游戏语言
	lang := system.ResolveLanguage(cfg.Language)
	msgLang := options.ApplyLanguage(GameDirFor(mcFolder, v, cfg), lang)
	log("[语言] " + msgLang)

	// 6) 拼参数
	cmd, gameDir := BuildCommand(mcFolder, v, cfg, login, javaPath, ramMB)
	if err := os.MkdirAll(gameDir, 0o755); err != nil {
		log("[错误] 创建游戏目录失败：" + err.Error())
		return false, err.Error()
	}
	log("启动命令：")
	for _, a := range cmd {
		log("  " + a)
	}

	// 7) 启动进程
	env := os.Environ()
	env = append(env, "APPDATA="+mcFolder)
	proc := exec.Command(javaPath)
	proc.Args = append([]string{javaPath}, cmd...)
	proc.Dir = gameDir
	proc.Env = env

	stdoutPipe, err := proc.StdoutPipe()
	if err != nil {
		log("[错误] 创建输出管道失败：" + err.Error())
		return false, err.Error()
	}
	stderrPipe, err := proc.StderrPipe()
	if err != nil {
		log("[错误] 创建错误管道失败：" + err.Error())
		return false, err.Error()
	}

	if err := proc.Start(); err != nil {
		log("[错误] 启动进程失败：" + err.Error())
		return false, err.Error()
	}
	log(fmt.Sprintf("游戏进程已启动 PID=%d", proc.Process.Pid))

	go scanLines(stdoutPipe, log)
	go scanLines(stderrPipe, log)

	// 超时强制结束
	done := make(chan error, 1)
	go func() { done <- proc.Wait() }()

	var waitErr error
	if maxSeconds > 0 {
		select {
		case waitErr = <-done:
		case <-time.After(time.Duration(maxSeconds) * time.Second):
			log(fmt.Sprintf("[超时] 达到 %d 秒上限，强制结束游戏进程（用于验证）", maxSeconds))
			_ = proc.Process.Kill()
			waitErr = <-done
		}
	} else {
		waitErr = <-done
	}

	log("游戏进程已退出")
	if waitErr != nil {
		log("[watcher] 进程异常退出：" + waitErr.Error())
		return true, "进程退出（" + waitErr.Error() + "）"
	}
	return true, "游戏已退出"
}
