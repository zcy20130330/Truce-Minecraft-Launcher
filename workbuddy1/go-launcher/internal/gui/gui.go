// Package gui 纯 Go 原生 Win32 桌面界面（基于 gonutz/w32，零 CGO、无 WebView）。
// 对应 prototype 的 Web UI / React 前端，但落地为真正的桌面应用程序窗口。
package gui

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"github.com/gonutz/w32"

	"truce-launcher/internal/config"
	"truce-launcher/internal/launch"
	"truce-launcher/internal/ramopt"
	"truce-launcher/internal/system"
	"truce-launcher/internal/versions"
)

// w32 v1.0.0 未导出的常量化名
const (
	emSetReadOnly = 0x00CF
	cbGetLBString = 0x0148 // CB_GETLBSTRING
)

// u16 把 Go 字符串转 *uint16（忽略错误，用于窗口/控件文本）。
func u16(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

// 控件 ID
const (
	idList      = 101
	idRefresh   = 102
	idLaunch    = 103
	idMcEdit    = 104
	idBrowse    = 105
	idMemEdit   = 106
	idAutoMem   = 107
	idLangCombo = 108
	idUserEdit  = 109
	idMemOptChk = 110
	idMemOptBtn = 111
	idSaveBtn   = 112
	idConsole   = 113
)

// appState 保存窗口与所有子控件句柄、配置与版本列表。
type appState struct {
	hMain      w32.HWND
	hList      w32.HWND
	hSelInfo   w32.HWND
	hMcEdit    w32.HWND
	hMemEdit   w32.HWND
	hAutoMem   w32.HWND
	hLangCombo w32.HWND
	hUserEdit  w32.HWND
	hMemOptChk w32.HWND
	hMemInfo   w32.HWND
	hConsole   w32.HWND
	hLaunchBtn w32.HWND

	cfg          *config.Config
	cfgPath      string
	versionNames []string
	consoleMu    sync.Mutex
}

// mainApp 是唯一的桌面主窗口状态指针。整个程序只有一个主窗口，
// 因此直接用包级全局变量持有 appState，避免在 wndProc 里通过
// GWLP_USERDATA 做 unsafe.Pointer 转换（否则 go vet 会误报）。
var mainApp *appState

// Run 启动桌面应用主窗口。
func Run() {
	exePath, err := os.Executable()
	exeDir := "."
	if err == nil {
		exeDir = filepath.Dir(exePath)
	}
	cfgPath := config.Path(exeDir)
	cfg := config.Load(cfgPath)

	a := &appState{cfg: cfg, cfgPath: cfgPath}

	instance := w32.GetModuleHandle("")
	cls := w32.WNDCLASSEX{
		Size:      uint32(unsafe.Sizeof(w32.WNDCLASSEX{})),
		WndProc:   syscall.NewCallback(wndProcGlobal),
		Instance:  instance,
		Cursor:    w32.LoadCursor(0, w32.MakeIntResource(w32.IDC_ARROW)),
		Background: w32.GetSysColorBrush(w32.COLOR_BTNFACE),
		ClassName: u16("TruceLauncherClass"),
	}
	atom := w32.RegisterClassEx(&cls)
	if atom == 0 {
		fmt.Fprintln(os.Stderr, "窗口类注册失败")
		os.Exit(1)
	}

	a.hMain = w32.CreateWindowEx(
		0, u16("TruceLauncherClass"),
		u16("Truce Minecraft Launcher"),
		w32.WS_OVERLAPPEDWINDOW,
		w32.CW_USEDEFAULT, w32.CW_USEDEFAULT, 916, 640,
		0, 0, instance, unsafe.Pointer(nil),
	)
	if a.hMain == 0 {
		fmt.Fprintln(os.Stderr, "主窗口创建失败")
		os.Exit(1)
	}
	mainApp = a

	a.createControls()
	a.initUI()

	w32.ShowWindow(a.hMain, w32.SW_SHOWNORMAL)
	w32.UpdateWindow(a.hMain)

	// 主线程跑消息循环
	runtime.LockOSThread()
	var msg w32.MSG
	for w32.GetMessage(&msg, 0, 0, 0) != 0 {
		w32.TranslateMessage(&msg)
		w32.DispatchMessage(&msg)
	}
}

// createControls 创建全部子控件。
func (a *appState) createControls() {
	instance := w32.GetModuleHandle("")

	make := func(class, text string, style, ex uint, x, y, w, h, id int) w32.HWND {
		return w32.CreateWindowEx(
			ex, u16(class), u16(text),
			style, x, y, w, h, a.hMain, w32.HMENU(id), instance, unsafe.Pointer(nil),
		)
	}

	label := func(text string, x, y, w, h int) {
		make("STATIC", text, w32.WS_CHILD|w32.WS_VISIBLE|w32.SS_LEFT, 0, x, y, w, h, 0)
	}

	// 左侧
	label("版本列表", 16, 14, 200, 20)
	a.hList = make("LISTBOX", "",
		w32.WS_CHILD|w32.WS_VISIBLE|w32.WS_BORDER|w32.WS_TABSTOP|w32.LBS_NOTIFY|w32.LBS_HASSTRINGS|w32.WS_VSCROLL,
		w32.WS_EX_CLIENTEDGE, 16, 40, 360, 300, idList)
	make("BUTTON", "刷新", w32.WS_CHILD|w32.WS_VISIBLE|w32.BS_PUSHBUTTON|w32.WS_TABSTOP, 0, 16, 350, 110, 28, idRefresh)
	a.hLaunchBtn = make("BUTTON", "启动游戏", w32.WS_CHILD|w32.WS_VISIBLE|w32.BS_PUSHBUTTON|w32.WS_TABSTOP, 0, 142, 350, 110, 28, idLaunch)
	a.hSelInfo = make("STATIC", "未选择版本", w32.WS_CHILD|w32.WS_VISIBLE|w32.SS_LEFT, 0, 16, 386, 360, 50, 0)

	// 右侧
	label("设置", 404, 14, 200, 20)
	label(".minecraft 目录：", 404, 40, 120, 20)
	a.hMcEdit = make("EDIT", a.cfg.McFolder,
		w32.WS_CHILD|w32.WS_VISIBLE|w32.WS_BORDER|w32.WS_TABSTOP|w32.ES_AUTOHSCROLL,
		w32.WS_EX_CLIENTEDGE, 404, 62, 320, 22, idMcEdit)
	make("BUTTON", "浏览…", w32.WS_CHILD|w32.WS_VISIBLE|w32.BS_PUSHBUTTON|w32.WS_TABSTOP, 0, 730, 60, 70, 24, idBrowse)
	label("Java：启动前按版本需求自动匹配（也可在 config.json 指定）", 404, 94, 470, 20)
	label("最大内存(MB)：", 404, 124, 100, 20)
	a.hMemEdit = make("EDIT", "",
		w32.WS_CHILD|w32.WS_VISIBLE|w32.WS_BORDER|w32.WS_TABSTOP|w32.ES_AUTOHSCROLL|w32.ES_NUMBER,
		w32.WS_EX_CLIENTEDGE, 510, 122, 90, 22, idMemEdit)
	a.hAutoMem = make("BUTTON", "自动分配", w32.WS_CHILD|w32.WS_VISIBLE|w32.BS_AUTOCHECKBOX|w32.WS_TABSTOP, 0, 612, 124, 110, 22, idAutoMem)
	label("游戏语言：", 404, 156, 100, 20)
	a.hLangCombo = make("COMBOBOX", "",
		w32.WS_CHILD|w32.WS_VISIBLE|w32.WS_BORDER|w32.WS_TABSTOP|w32.CBS_DROPDOWNLIST,
		w32.WS_EX_CLIENTEDGE, 510, 154, 220, 200, idLangCombo)
	label("离线账号名：", 404, 188, 100, 20)
	a.hUserEdit = make("EDIT", a.cfg.Username,
		w32.WS_CHILD|w32.WS_VISIBLE|w32.WS_BORDER|w32.WS_TABSTOP|w32.ES_AUTOHSCROLL,
		w32.WS_EX_CLIENTEDGE, 510, 186, 200, 22, idUserEdit)
	a.hMemOptChk = make("BUTTON", "内存过低时自动优化（RamMap 式）", w32.WS_CHILD|w32.WS_VISIBLE|w32.BS_AUTOCHECKBOX|w32.WS_TABSTOP, 0, 404, 220, 320, 22, idMemOptChk)
	make("BUTTON", "立即内存优化", w32.WS_CHILD|w32.WS_VISIBLE|w32.BS_PUSHBUTTON|w32.WS_TABSTOP, 0, 404, 250, 130, 28, idMemOptBtn)
	make("BUTTON", "保存设置", w32.WS_CHILD|w32.WS_VISIBLE|w32.BS_PUSHBUTTON|w32.WS_TABSTOP, 0, 544, 250, 130, 28, idSaveBtn)
	a.hMemInfo = make("STATIC", "", w32.WS_CHILD|w32.WS_VISIBLE|w32.SS_LEFT, 0, 404, 286, 470, 44, 0)

	// 底部控制台
	label("控制台", 16, 452, 200, 20)
	a.hConsole = make("EDIT", "",
		w32.WS_CHILD|w32.WS_VISIBLE|w32.WS_BORDER|w32.WS_TABSTOP|w32.ES_MULTILINE|w32.ES_AUTOVSCROLL|w32.ES_AUTOHSCROLL|w32.WS_VSCROLL,
		w32.WS_EX_CLIENTEDGE, 16, 476, 868, 110, idConsole)
	w32.SendMessage(a.hConsole, emSetReadOnly, 1, 0)
}

// initUI 设置初始控件值与语言下拉。
func (a *appState) initUI() {
	// 语言下拉
	langOptions := []string{
		"system（跟随系统）",
		"zh_cn（简体中文）",
		"zh_tw（繁体中文）",
		"en_us（英文）",
		"ja_jp（日文）",
		"ko_kr（韩文）",
		"none（不干预）",
	}
	for _, o := range langOptions {
		w32.SendMessage(a.hLangCombo, w32.CB_ADDSTRING, 0,
			uintptr(unsafe.Pointer(u16(o))))
	}
	w32.SendMessage(a.hLangCombo, w32.CB_SETCURSEL, uintptr(langToOption(a.cfg.Language)), 0)

	// 内存
	if a.cfg.AutoMemory {
		w32.SetWindowText(a.hMemEdit, strconv.Itoa(system.SuggestMemoryMB(0)))
		w32.SendMessage(a.hMemEdit, w32.EM_SETREADONLY, 1, 0)
	} else {
		w32.SetWindowText(a.hMemEdit, strconv.Itoa(int(a.cfg.RamGB*1024)))
		w32.SendMessage(a.hMemEdit, w32.EM_SETREADONLY, 0, 0)
	}
	// 勾选框初始状态
	w32.SendMessage(a.hAutoMem, w32.BM_SETCHECK, boolToCheck(a.cfg.AutoMemory), 0)
	w32.SendMessage(a.hMemOptChk, w32.BM_SETCHECK, boolToCheck(a.cfg.MemoryOptimize), 0)

	a.log("Truce Minecraft Launcher（原生桌面版）已启动")
	a.log("配置文件：" + a.cfgPath)
	a.refresh()
}

func boolToCheck(b bool) uintptr {
	if b {
		return w32.BST_CHECKED
	}
	return w32.BST_UNCHECKED
}

// ---------- 行为方法 ----------

func (a *appState) log(line string) {
	a.consoleMu.Lock()
	defer a.consoleMu.Unlock()
	cur := w32.GetWindowText(a.hConsole)
	if cur != "" {
		cur += "\r\n"
	}
	cur += line
	w32.SetWindowText(a.hConsole, cur)
	end := uintptr(len([]rune(cur)))
	w32.SendMessage(a.hConsole, w32.EM_SETSEL, end, end)
	w32.SendMessage(a.hConsole, w32.EM_SCROLLCARET, 0, 0)
}

func (a *appState) updateMemInfo() {
	total := system.TotalMemoryMB()
	avail := system.AvailableMemoryMB()
	w32.SetWindowText(a.hMemInfo, fmt.Sprintf(
		"物理内存：%d MB   可用：%d MB   建议分配：%d MB",
		total, avail, system.SuggestMemoryMB(total),
	))
}

func (a *appState) refresh() {
	w32.SendMessage(a.hList, w32.LB_RESETCONTENT, 0, 0)
	a.versionNames = a.versionNames[:0]
	for _, info := range versions.ScanVersions(a.cfg.McFolder) {
		name, _ := info["name"].(string)
		w32.SendMessage(a.hList, w32.LB_ADDSTRING, 0,
			uintptr(unsafe.Pointer(u16(name))))
		a.versionNames = append(a.versionNames, name)
	}
	a.log(fmt.Sprintf("扫描到 %d 个版本（目录：%s）", len(a.versionNames), a.cfg.McFolder))
	a.updateMemInfo()
}

func (a *appState) selectedVersion() string {
	idx := int(w32.SendMessage(a.hList, w32.LB_GETCURSEL, 0, 0))
	if idx < 0 || idx >= len(a.versionNames) {
		return ""
	}
	return a.versionNames[idx]
}

func (a *appState) applyUISettings() {
	a.cfg.McFolder = w32.GetWindowText(a.hMcEdit)
	if w32.SendMessage(a.hAutoMem, w32.BM_GETCHECK, 0, 0) == w32.BST_CHECKED {
		a.cfg.AutoMemory = true
	} else {
		a.cfg.AutoMemory = false
		if n, err := strconv.Atoi(w32.GetWindowText(a.hMemEdit)); err == nil {
			a.cfg.RamGB = float64(n) / 1024.0
		}
	}
	a.cfg.Language = optionToLang(int(w32.SendMessage(a.hLangCombo, w32.CB_GETCURSEL, 0, 0)))
	a.cfg.Username = w32.GetWindowText(a.hUserEdit)
	a.cfg.MemoryOptimize = w32.SendMessage(a.hMemOptChk, w32.BM_GETCHECK, 0, 0) == w32.BST_CHECKED
}

func (a *appState) onSelectVersion() {
	name := a.selectedVersion()
	if name == "" {
		w32.SetWindowText(a.hSelInfo, "未选择版本")
		return
	}
	v := versions.GetVersion(a.cfg.McFolder, name)
	loaders := strings.Join(v.Loaders(), "/")
	if loaders == "" {
		loaders = "原版"
	}
	w32.SetWindowText(a.hSelInfo, fmt.Sprintf(
		"版本：%s\n类型：%s   加载器：%s   Java需求：%d",
		name, v.Type(), loaders, v.JavaRequirement(),
	))
}

func (a *appState) onAutoMemToggle() {
	checked := w32.SendMessage(a.hAutoMem, w32.BM_GETCHECK, 0, 0) == w32.BST_CHECKED
	if checked {
		w32.SetWindowText(a.hMemEdit, strconv.Itoa(system.SuggestMemoryMB(0)))
		w32.SendMessage(a.hMemEdit, w32.EM_SETREADONLY, 1, 0)
	} else {
		w32.SendMessage(a.hMemEdit, w32.EM_SETREADONLY, 0, 0)
	}
}

func (a *appState) onBrowse() {
	if path := a.browseFolder("选择 .minecraft 目录"); path != "" {
		w32.SetWindowText(a.hMcEdit, path)
		a.cfg.McFolder = path
		a.refresh()
	}
}

func (a *appState) onSave() {
	a.applyUISettings()
	if err := config.Save(a.cfgPath, a.cfg); err != nil {
		w32.MessageBox(a.hMain, "保存失败："+err.Error(), "错误", w32.MB_OK|w32.MB_ICONERROR)
		return
	}
	w32.MessageBox(a.hMain, "配置已写入：\n"+a.cfgPath, "已保存", w32.MB_OK|w32.MB_ICONINFORMATION)
	a.updateMemInfo()
}

func (a *appState) onMemOpt() {
	go func() {
		a.log("--- 内存优化（RamMap 式）---")
		res := ramopt.Optimize(func(s string) { a.log(s) }, true)
		if res.OK {
			a.log(fmt.Sprintf("内存优化完成：释放约 %d MB（%d → %d）", res.FreedMB, res.BeforeMB, res.AfterMB))
		} else {
			a.log("内存优化未完全生效：部分步骤需管理员权限，请用管理员身份运行本启动器。")
		}
		a.updateMemInfo()
	}()
}

func (a *appState) onLaunch() {
	name := a.selectedVersion()
	if name == "" {
		w32.MessageBox(a.hMain, "请先在左侧列表中选择一个版本。", "提示", w32.MB_OK|w32.MB_ICONWARNING)
		return
	}
	a.applyUISettings()
	w32.EnableWindow(a.hLaunchBtn, false)
	go func() {
		defer w32.EnableWindow(a.hLaunchBtn, true)
		a.log("==================================================")
		_, msg := launch.Launch(a.cfg.McFolder, name, a.cfg, func(s string) {
			a.log(s)
		}, 0)
		a.log("【启动结束】" + msg)
		a.updateMemInfo()
	}()
}

func (a *appState) browseFolder(title string) string {
	var bi w32.BROWSEINFO
	bi.Title = u16(title)
	bi.Flags = w32.BIF_NEWDIALOGSTYLE | w32.BIF_RETURNONLYFSDIRS
	idl := w32.SHBrowseForFolder(&bi)
	if idl == 0 {
		return ""
	}
	return w32.SHGetPathFromIDList(idl)
}

// ---------- 消息分发 ----------

func wndProcGlobal(hwnd w32.HWND, msg uint32, wparam, lparam uintptr) uintptr {
	if msg == w32.WM_CREATE {
		return w32.DefWindowProc(hwnd, msg, wparam, lparam)
	}
	a := mainApp
	if a == nil {
		return w32.DefWindowProc(hwnd, msg, wparam, lparam)
	}

	switch msg {
	case w32.WM_COMMAND:
		id := int(wparam & 0xFFFF)
		code := int((wparam >> 16) & 0xFFFF)
		switch id {
		case idRefresh:
			if code == w32.BN_CLICKED {
				a.applyUISettings()
				a.refresh()
			}
		case idLaunch:
			if code == w32.BN_CLICKED {
				a.onLaunch()
			}
		case idBrowse:
			if code == w32.BN_CLICKED {
				a.onBrowse()
			}
		case idMemOptBtn:
			if code == w32.BN_CLICKED {
				a.onMemOpt()
			}
		case idSaveBtn:
			if code == w32.BN_CLICKED {
				a.onSave()
			}
		case idAutoMem:
			if code == w32.BN_CLICKED {
				a.onAutoMemToggle()
			}
		case idList:
			if code == w32.LBN_SELCHANGE {
				a.onSelectVersion()
			}
		}
		return w32.DefWindowProc(hwnd, msg, wparam, lparam)
	case w32.WM_DESTROY:
		w32.PostQuitMessage(0)
		return 0
	case w32.WM_CLOSE:
		w32.DestroyWindow(hwnd)
		return 0
	default:
		return w32.DefWindowProc(hwnd, msg, wparam, lparam)
	}
}

// langToOption 配置语言代码 → 下拉索引。
func langToOption(lang string) int {
	switch strings.TrimSpace(lang) {
	case "zh_cn":
		return 1
	case "zh_tw":
		return 2
	case "en_us":
		return 3
	case "ja_jp":
		return 4
	case "ko_kr":
		return 5
	case "none":
		return 6
	default:
		return 0
	}
}

// optionToLang 下拉索引 → 配置语言代码。
func optionToLang(idx int) string {
	switch idx {
	case 1:
		return "zh_cn"
	case 2:
		return "zh_tw"
	case 3:
		return "en_us"
	case 4:
		return "ja_jp"
	case 5:
		return "ko_kr"
	case 6:
		return "none"
	default:
		return "system"
	}
}
