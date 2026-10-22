package java

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// InstallProgress 是自动安装 Java 过程中的一次进度上报。
type InstallProgress struct {
	// Stage: resolve / download / extract / verify / done
	Stage   string `json:"stage"`
	Percent int    `json:"percent"`
	Message string `json:"message"`
}

// DefaultJavaDir 返回启动器自带的 Java 安装根目录。
func DefaultJavaDir() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		if d, err := os.UserCacheDir(); err == nil {
			base = d
		} else {
			base = os.TempDir()
		}
	}
	return filepath.Join(base, "TruceLauncher", "java")
}

var httpClient = &http.Client{Timeout: 45 * time.Minute}

const userAgent = "TruceLauncher/1.2 (+https://github.com/zcy20130330/Truce-Minecraft-Launcher)"

// mirror 是一个下载源：某主版本下 jre/jdk 的目录页地址。
type mirror struct {
	name string
	dir  string
	kind string // "jre" 或 "jdk"
}

// mirrorsFor 返回某主版本的候选下载源，按实测速度从快到慢排列
// （TUNA 在华语区约 3~5 MB/s；GitHub 直连常不可用，放最后并走加速前缀）。
// JRE 优先：启动器跑游戏只需要 JRE，体积约为 JDK 的四分之一。
func mirrorsFor(major int) []mirror {
	var out []mirror
	for _, k := range []string{"jre", "jdk"} {
		out = append(out, mirror{
			name: fmt.Sprintf("TUNA/%s", strings.ToUpper(k)),
			dir:  fmt.Sprintf("https://mirrors.tuna.tsinghua.edu.cn/Adoptium/%d/%s/x64/windows/", major, k),
			kind: k,
		})
	}
	return out
}

var zipHrefRe = regexp.MustCompile(`href="([^"]+\.zip)"`)

// pickLatestZip 从目录页 HTML 里挑出指定类型的最新 zip 文件名。
// 命名形如 OpenJDK21U-jre_x64_windows_hotspot_21.0.12.1_1.zip（现代版）
// 或 OpenJDK8U-jre_x64_windows_hotspot_8u504b01.zip（Java 8），
// 注意发行版名与类型之间是连字符，不能按 "_jre_" 匹配。
func pickLatestZip(html, kind string) string {
	marker := kind + "_x64_windows_hotspot"
	best := ""
	var bestVer []int
	for _, m := range zipHrefRe.FindAllStringSubmatch(html, -1) {
		name := m[1]
		if i := strings.LastIndex(name, "/"); i >= 0 {
			name = name[i+1:]
		}
		if !strings.Contains(name, marker) {
			continue
		}
		ver := verFromZip(name)
		if len(ver) == 0 {
			continue
		}
		if best == "" || lessVer(bestVer, ver) {
			best, bestVer = name, ver
		}
	}
	return best
}

// verFromZip 从包名里解析版本号数字序列（21.0.12.1_1 → 21,0,12,1,1；8u504b01 → 8,504,1）。
func verFromZip(name string) []int {
	base := strings.TrimSuffix(name, ".zip")
	if i := strings.Index(base, "hotspot_"); i >= 0 {
		base = base[i+len("hotspot_"):]
	}
	var nums []int
	cur := ""
	flush := func() {
		if cur != "" {
			if n, err := strconv.Atoi(cur); err == nil {
				nums = append(nums, n)
			}
			cur = ""
		}
	}
	for _, r := range base {
		if r >= '0' && r <= '9' {
			cur += string(r)
		} else {
			flush()
		}
	}
	flush()
	return nums
}

func lessVer(a, b []int) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return len(a) < len(b)
}

func fetchText(url string) (string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// adoptiumPackageLink 通过 Adoptium 官方 API 取某个主版本的 zip 直链（兜底用）。
func adoptiumPackageLink(major int) (string, error) {
	api := fmt.Sprintf(
		"https://api.adoptium.net/v3/assets/latest/%d/hotspot?architecture=x64&image_type=jre&os=windows&vendor=eclipse",
		major)
	text, err := fetchText(api)
	if err != nil {
		return "", err
	}
	var assets []struct {
		Binary struct {
			Package struct {
				Link string `json:"link"`
			} `json:"package"`
		} `json:"binary"`
	}
	if err := json.Unmarshal([]byte(text), &assets); err != nil {
		return "", err
	}
	for _, a := range assets {
		if a.Binary.Package.Link != "" {
			return a.Binary.Package.Link, nil
		}
	}
	return "", fmt.Errorf("Adoptium 未返回可用包")
}

// candidate 是一个可尝试的下载目标（镜像 + 具体包）。
type candidate struct {
	source string
	url    string
	name   string
}

// resolveCandidates 收集所有可用下载目标，按优先级排序：
// TUNA 的 JRE → TUNA 的 JDK → Adoptium 官方 API（GitHub 直连）→ GitHub 加速。
// 不在第一个镜像上死磕——断流时上层会换下一个源。
func resolveCandidates(major int, report func(InstallProgress)) ([]candidate, error) {
	report(InstallProgress{Stage: "resolve", Message: fmt.Sprintf("正在查找 Java %d 的下载地址…", major)})

	var out []candidate
	for _, m := range mirrorsFor(major) {
		html, err := fetchText(m.dir)
		if err != nil {
			report(InstallProgress{Stage: "resolve", Message: fmt.Sprintf("%s 不可用（%v）", m.name, err)})
			continue
		}
		name := pickLatestZip(html, m.kind)
		if name == "" {
			report(InstallProgress{Stage: "resolve", Message: fmt.Sprintf("%s 未找到 Java %d 的 %s 包", m.name, major, strings.ToUpper(m.kind))})
			continue
		}
		out = append(out, candidate{source: m.name, url: m.dir + name, name: name})
	}

	// 兜底：Adoptium 官方 API（包托管在 GitHub，国内多半需要加速前缀）
	if link, err := adoptiumPackageLink(major); err == nil {
		name := link
		if i := strings.LastIndex(link, "/"); i >= 0 {
			name = link[i+1:]
		}
		out = append(out,
			candidate{source: "Adoptium 官方", url: link, name: name},
			candidate{source: "Adoptium 官方（加速）", url: "https://ghfast.top/" + link, name: name},
		)
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("所有下载源均不可用，请检查网络后重试")
	}
	return out, nil
}

const (
	downloadAttempts = 4               // 单个镜像的最大尝试次数（含首次）
	downloadRetryGap = 1200 * time.Millisecond // 重试基础间隔，逐次递增
)

// download 把 url 下载到 dst：断点续传 + 多次重试。
// 镜像偶尔中途断流（unexpected EOF）是常态，必须能接着下而不是从头再来。
func download(url, dst string, report func(InstallProgress)) error {
	var lastErr error
	for attempt := 1; attempt <= downloadAttempts; attempt++ {
		err := downloadAttempt(url, dst, report)
		if err == nil {
			return nil
		}
		lastErr = err
		if attempt == downloadAttempts {
			break
		}
		have := int64(0)
		if st, serr := os.Stat(dst); serr == nil {
			have = st.Size()
		}
		report(InstallProgress{
			Stage:   "download",
			Percent: -1, // 负值表示“不确定”，前端只显示文字不推进进度条
			Message: fmt.Sprintf("连接中断（%v），已下载 %.1f MB，%d 秒后续传 %d/%d",
				err, float64(have)/1024/1024, attempt*2, attempt, downloadAttempts-1),
		})
		time.Sleep(time.Duration(attempt) * downloadRetryGap)
	}
	return lastErr
}

// downloadAttempt 执行一次下载尝试；若 dst 已有内容则带 Range 续传。
func downloadAttempt(url, dst string, report func(InstallProgress)) error {
	var offset int64
	if st, err := os.Stat(dst); err == nil {
		offset = st.Size()
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable && offset > 0 {
		return nil // 本地已有完整文件
	}
	if offset > 0 && resp.StatusCode == http.StatusOK {
		offset = 0 // 服务端不支持续传，只能从头下
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("下载失败：HTTP %d", resp.StatusCode)
	}

	flags := os.O_CREATE | os.O_WRONLY
	if offset > 0 {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}
	out, err := os.OpenFile(dst, flags, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()

	total := int64(0)
	if resp.ContentLength > 0 {
		total = resp.ContentLength + offset
	}
	report(InstallProgress{
		Stage:   "download",
		Percent: 0,
		Message: fmt.Sprintf("开始下载 %.1f MB", float64(total)/1024/1024),
	})

	buf := make([]byte, 128<<10)
	done := offset
	last := time.Now()
	lastPct := -1
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				return werr
			}
			done += int64(n)
			pct := 0
			if total > 0 {
				pct = int(done * 100 / total)
			}
			if time.Since(last) > 200*time.Millisecond && pct != lastPct {
				last = time.Now()
				lastPct = pct
				msg := fmt.Sprintf("已下载 %.1f MB", float64(done)/1024/1024)
				if total > 0 {
					msg = fmt.Sprintf("%.1f / %.1f MB", float64(done)/1024/1024, float64(total)/1024/1024)
				}
				report(InstallProgress{Stage: "download", Percent: pct, Message: msg})
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}

	// 有总长时必须下满，否则视为断流（交回上层重试）
	if total > 0 {
		if st, serr := os.Stat(dst); serr == nil && st.Size() < total {
			return fmt.Errorf("连接提前结束：%d/%d 字节", st.Size(), total)
		}
	}
	report(InstallProgress{Stage: "download", Percent: 100,
		Message: fmt.Sprintf("下载完成 %.1f MB", float64(done)/1024/1024)})
	return nil
}

// unzipInto 解压 zip 到 dst，自动去掉压缩包内唯一的顶层目录。
func unzipInto(src, dst string, report func(InstallProgress)) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	// 判断压缩包内是否所有条目都在同一个顶层目录下
	prefix := ""
	first := true
	for _, f := range r.File {
		top := strings.SplitN(strings.TrimPrefix(f.Name, "/"), "/", 2)[0]
		if first {
			prefix = top
			first = false
			continue
		}
		if top != prefix {
			prefix = ""
			break
		}
	}
	if prefix != "" {
		prefix += "/"
	}

	total := len(r.File)
	for i, f := range r.File {
		rel := strings.TrimPrefix(f.Name, prefix)
		if rel == "" {
			continue
		}
		target := filepath.Join(dst, filepath.FromSlash(rel))
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(dst)) {
			continue // 防路径穿越
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		w, err := os.Create(target)
		if err != nil {
			_ = rc.Close()
			return err
		}
		_, cerr := io.Copy(w, rc)
		_ = w.Close()
		_ = rc.Close()
		if cerr != nil {
			return cerr
		}
		if i%400 == 0 {
			report(InstallProgress{
				Stage:   "extract",
				Percent: i * 100 / total,
				Message: fmt.Sprintf("正在解压 %d/%d", i, total),
			})
		}
	}
	return nil
}

// ExistingInstall 检查启动器目录里是否已有该主版本的可用 Java，返回 java.exe 路径。
func ExistingInstall(javaDir string, major int) string {
	entries, err := os.ReadDir(javaDir)
	if err != nil {
		return ""
	}
	prefix := fmt.Sprintf("temurin-%d", major)
	var names []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), prefix) {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, n := range names {
		p := filepath.Join(javaDir, n, "bin", "java.exe")
		if info, ok := GetJavaVersion(p); ok {
			if m, _ := info["major"].(int); m == major {
				return p
			}
		}
	}
	return ""
}

// InstallJRE 下载并安装指定主版本的 JRE 到 javaDir，返回 java.exe 路径。
// javaDir 为空时使用 DefaultJavaDir()。已安装则直接返回现有路径。
func InstallJRE(major int, javaDir string, report func(InstallProgress)) (string, error) {
	if report == nil {
		report = func(InstallProgress) {}
	}
	if major < 8 {
		major = 8
	}
	if javaDir == "" {
		javaDir = DefaultJavaDir()
	}

	if p := ExistingInstall(javaDir, major); p != "" {
		report(InstallProgress{Stage: "done", Percent: 100,
			Message: fmt.Sprintf("Java %d 已存在，无需重复安装", major)})
		return p, nil
	}

	cands, err := resolveCandidates(major, report)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(javaDir, 0o755); err != nil {
		return "", err
	}
	tmpDir := filepath.Join(javaDir, ".tmp")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return "", err
	}

	// 逐个候选源尝试：单个源断流重试仍失败就换下一个源，半成品包必须清掉再下。
	var zipPath, usedName string
	var lastErr error
	for i, c := range cands {
		report(InstallProgress{Stage: "resolve", Percent: -1,
			Message: fmt.Sprintf("下载源 %d/%d：%s · %s", i+1, len(cands), c.source, c.name)})
		p := filepath.Join(tmpDir, c.name)
		if err := download(c.url, p, report); err != nil {
			lastErr = err
			report(InstallProgress{Stage: "resolve", Percent: -1,
				Message: fmt.Sprintf("%s 下载失败（%v），尝试下一个源", c.source, err)})
			_ = os.Remove(p)
			continue
		}
		zipPath, usedName = p, c.name
		break
	}
	if zipPath == "" {
		if lastErr == nil {
			lastErr = fmt.Errorf("所有下载源均失败")
		}
		return "", fmt.Errorf("下载失败：%v", lastErr)
	}
	_ = usedName
	defer os.Remove(zipPath)

	target := filepath.Join(javaDir, fmt.Sprintf("temurin-%d", major))
	// 清掉可能残留的半成品
	if _, statErr := os.Stat(target); statErr == nil {
		_ = os.RemoveAll(target)
	}
	report(InstallProgress{Stage: "extract", Percent: 0, Message: "正在解压到 " + target})
	if err := unzipInto(zipPath, target, report); err != nil {
		_ = os.RemoveAll(target)
		return "", err
	}

	javaExe := filepath.Join(target, "bin", "java.exe")
	report(InstallProgress{Stage: "verify", Percent: 100, Message: "正在校验安装结果…"})
	info, ok := GetJavaVersion(javaExe)
	if !ok {
		_ = os.RemoveAll(target)
		return "", fmt.Errorf("安装包已解压但 java.exe 无法运行，可能缺少运行库（需 VC++ 2015-2022）")
	}
	got, _ := info["major"].(int)
	if got != major {
		_ = os.RemoveAll(target)
		return "", fmt.Errorf("安装结果版本不符：期望 Java %d，实际 Java %d", major, got)
	}

	report(InstallProgress{Stage: "done", Percent: 100,
		Message: fmt.Sprintf("Java %d 安装完成：%s", major, javaExe)})
	return javaExe, nil
}
