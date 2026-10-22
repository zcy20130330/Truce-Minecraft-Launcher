// Package java 负责 Java 检测与版本解析（对应 prototype 的 launcher/java.py）。
package java

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

func candidateRoots() []string {
	expand := func(s string) string {
		s = strings.ReplaceAll(s, "%LOCALAPPDATA%", os.Getenv("LOCALAPPDATA"))
		s = strings.ReplaceAll(s, "%USERPROFILE%", os.Getenv("USERPROFILE"))
		return s
	}
	return []string{
		`C:\Program Files\Java`,
		`C:\Program Files\Eclipse Adoptium`,
		`C:\Program Files\Microsoft`,
		`C:\Program Files (x86)\Java`,
		`C:\Program Files\Zulu`,
		`C:\Program Files\Amazon Corretto`,
		`C:\Program Files\BellSoft`,
		`C:\Program Files\RedHat`,
		expand(`%LOCALAPPDATA%\Programs\Eclipse Adoptium`),
		expand(`%LOCALAPPDATA%\Programs\Microsoft`),
		expand(`%USERPROFILE%\.jdks`),
	}
}

var versionRe = regexp.MustCompile(`version "([0-9._]+)"`)

func parseJavaVersion(text string) (major, minor, patch int, ok bool) {
	m := versionRe.FindStringSubmatch(text)
	if m == nil {
		re2 := regexp.MustCompile(`version "([0-9.]+)"`)
		m = re2.FindStringSubmatch(text)
	}
	if m == nil {
		return 0, 0, 0, false
	}
	raw := m[1]
	if strings.HasPrefix(raw, "1.") {
		parts := strings.Split(raw, ".")
		maj, _ := strconv.Atoi(parts[1])
		return maj, 0, 0, true
	}
	parts := strings.Split(raw, ".")
	maj, _ := strconv.Atoi(parts[0])
	mn := 0
	if len(parts) > 1 {
		mn, _ = strconv.Atoi(parts[1])
	}
	pt := 0
	if len(parts) > 2 {
		pt, _ = strconv.Atoi(parts[2])
	}
	return maj, mn, pt, true
}

// GetJavaVersion 解析 java -version 输出，返回 {path,version[],major}。
func GetJavaVersion(javaPath string) (map[string]interface{}, bool) {
	cmd := exec.Command(javaPath, "-version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, false
	}
	maj, mn, pt, ok := parseJavaVersion(string(out))
	if !ok {
		return nil, false
	}
	return map[string]interface{}{
		"path":    javaPath,
		"version": []int{maj, mn, pt},
		"major":   maj,
	}, true
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

// DetectJava 检测系统 Java，返回按版本号从高到低排序的列表。
// extraRoots 为额外要扫描的根目录（例如启动器自带的 java 安装目录）。
func DetectJava(extraRoots ...string) []map[string]interface{} {
	found := map[string]bool{}
	var paths []string

	add := func(p string) {
		if p == "" {
			return
		}
		if _, err := os.Stat(p); err != nil {
			return
		}
		lower := strings.ToLower(p)
		if !found[lower] {
			found[lower] = true
			paths = append(paths, p)
		}
	}

	for _, root := range candidateRoots() {
		if !dirExists(root) {
			continue
		}
		filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if !info.IsDir() && strings.EqualFold(info.Name(), "java.exe") {
				add(p)
			}
			return nil
		})
	}
	// 注册表登记的 JDK/JRE（很多安装包不落在 Program Files 下）
	for _, home := range registryJavaHomes() {
		add(filepath.Join(home, "bin", "java.exe"))
	}
	// 启动器自己下载安装的 Java
	for _, root := range extraRoots {
		if root == "" || !dirExists(root) {
			continue
		}
		add(filepath.Join(root, "bin", "java.exe"))
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				add(filepath.Join(root, e.Name(), "bin", "java.exe"))
			}
		}
	}
	if jh := os.Getenv("JAVA_HOME"); jh != "" {
		add(filepath.Join(jh, "bin", "java.exe"))
	}
	for _, d := range strings.Split(os.Getenv("PATH"), ";") {
		d = strings.TrimSpace(strings.Trim(d, `"`))
		if d != "" {
			add(filepath.Join(d, "java.exe"))
		}
	}

	var result []map[string]interface{}
	for _, p := range paths {
		if info, ok := GetJavaVersion(p); ok {
			result = append(result, info)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		vi := result[i]["version"].([]int)
		vj := result[j]["version"].([]int)
		for k := 0; k < len(vi) && k < len(vj); k++ {
			if vi[k] != vj[k] {
				return vi[k] > vj[k]
			}
		}
		return len(vi) > len(vj)
	})
	return result
}

// SelectJava 按需求选择 Java：优先精确 major，其次 >= required 的最低版本。
// 没有任何满足要求的版本时返回 nil——绝不能拿低版本硬跑，
// 那必然以 "UnsupportedClassVersionError" 启动失败。
func SelectJava(javaList []map[string]interface{}, requiredMajor int) map[string]interface{} {
	if len(javaList) == 0 {
		return nil
	}
	if requiredMajor <= 0 {
		return javaList[0]
	}
	for _, j := range javaList {
		if major, _ := j["major"].(int); major == requiredMajor {
			return j
		}
	}
	var eligible []map[string]interface{}
	for _, j := range javaList {
		if major, _ := j["major"].(int); major >= requiredMajor {
			eligible = append(eligible, j)
		}
	}
	if len(eligible) > 0 {
		return eligible[len(eligible)-1] // 已按版本降序，取最低合格
	}
	return nil
}
