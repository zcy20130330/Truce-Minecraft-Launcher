// Package versions 负责版本扫描与 JSON 解析（对应 prototype 的 launcher/versions.py）。
// 实现版本 JSON 读取、沿 inheritsFrom 递归继承合并、libraries 补全（含 natives 识别）、
// maven 路径推算、版本扫描。纯标准库实现。
package versions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// arch 仅处理 64 位。
const arch = "64"

// Version 是一个 Minecraft 版本（可能继承自其它版本）。
type Version struct {
	McFolder string
	Name     string
	Path     string
	JSONPath string
	JSON     map[string]interface{}
	Error    string
	BaseID   string
}

// Library 是一条支持库/原生库信息。
type Library struct {
	Name    string // maven 坐标，如 org.lwjgl:lwjgl:3.3.3
	Path    string // libraries 目录下的相对路径（maven 路径）
	Local   string // 磁盘绝对路径
	URL     string // 官方下载 URL（可能为空）
	SHA1    string
	Size    int64
	Natives string // 若为原生库，记录其 classifier（如 natives-windows），否则为空
}

// ---------- 通用 map 访问辅助 ----------

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

// ---------- 路径辅助 ----------

func VersionFolder(mcFolder, name string) string {
	return filepath.Join(mcFolder, "versions", name)
}

func VersionJSONPath(mcFolder, name string) string {
	return filepath.Join(VersionFolder(mcFolder, name), name+".json")
}

func loadJSON(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// NewVersion 构造并加载版本。
func NewVersion(mcFolder, name string) *Version {
	v := &Version{
		McFolder: mcFolder,
		Name:     name,
		Path:     VersionFolder(mcFolder, name),
		JSONPath: VersionJSONPath(mcFolder, name),
		BaseID:   name,
	}
	v.load()
	return v
}

// load 读取版本 JSON 并沿 inheritsFrom 递归合并。
func (v *Version) load() {
	j, err := loadJSON(v.JSONPath)
	if err != nil {
		v.Error = fmt.Sprintf("无法读取版本 JSON：%v", err)
		return
	}
	v.JSON = j

	seen := map[string]bool{v.Name: true}
	inherit, _ := asString(j["inheritsFrom"])
	var chain []*Version
	for inherit != "" && inherit != v.Name && !seen[inherit] {
		seen[inherit] = true
		parent := NewVersion(v.McFolder, inherit)
		if parent.JSON != nil {
			chain = append(chain, parent)
			if inh2, ok := asString(parent.JSON["inheritsFrom"]); ok && inh2 != "" {
				inherit = inh2
				v.BaseID = inh2
			} else {
				inherit = ""
			}
		} else {
			break
		}
	}

	if len(chain) == 0 {
		return
	}

	// 从最底层父版本开始向上覆盖，最后用自身覆盖（与 Python 一致）
	merged := map[string]interface{}{}
	for k, val := range chain[len(chain)-1].JSON {
		merged[k] = val
	}
	for _, parent := range chain[:len(chain)-1] {
		for k, val := range parent.JSON {
			merged[k] = val
		}
	}
	for k, val := range v.JSON {
		merged[k] = val
	}

	// libraries 合并：自身在前，父版本追加去重
	selfLibs, _ := asSlice(v.JSON["libraries"])
	mergedLibs := make([]interface{}, 0, len(selfLibs))
	for _, l := range selfLibs {
		mergedLibs = append(mergedLibs, l)
	}
	for _, parent := range chain {
		parentLibs, _ := asSlice(parent.JSON["libraries"])
		for _, l := range parentLibs {
			if !containsLib(mergedLibs, l) {
				mergedLibs = append(mergedLibs, l)
			}
		}
	}
	merged["libraries"] = mergedLibs
	merged["inheritsFrom"] = chain[len(chain)-1].Name
	v.JSON = merged
}

func containsLib(list []interface{}, l interface{}) bool {
	target, err := json.Marshal(l)
	if err != nil {
		return false
	}
	for _, x := range list {
		s, err := json.Marshal(x)
		if err == nil && string(s) == string(target) {
			return true
		}
	}
	return false
}

// ---------- 属性 ----------

func (v *Version) MainClass() string {
	if s, ok := asString(v.JSON["mainClass"]); ok {
		return s
	}
	return ""
}

func (v *Version) ReleaseTime() string {
	if s, ok := asString(v.JSON["releaseTime"]); ok {
		return s
	}
	return ""
}

func (v *Version) Type() string {
	if v.JSON == nil {
		return "error"
	}
	if s, ok := asString(v.JSON["type"]); ok {
		return s
	}
	return "unknown"
}

func (v *Version) AssetsID() string {
	if v.JSON == nil {
		return ""
	}
	if s, ok := asString(v.JSON["assets"]); ok && s != "" {
		return s
	}
	maj := v.Major()
	if maj <= 5 {
		return "legacy"
	}
	if maj == 6 {
		return "pre-1.6"
	}
	return ""
}

func (v *Version) Major() int {
	re := regexp.MustCompile(`(\d+)\.(\d+)`)
	m := re.FindStringSubmatch(v.BaseID)
	if m != nil {
		n, _ := strconv.Atoi(m[2])
		return n
	}
	return 0
}

func (v *Version) JavaRequirement() int {
	if v.JSON != nil {
		if jv, ok := v.JSON["javaVersion"]; ok {
			switch t := jv.(type) {
			case map[string]interface{}:
				if mv, ok := t["majorVersion"]; ok {
					if f, ok := mv.(float64); ok {
						return int(f)
					}
				}
				if mv, ok := t["major"]; ok {
					if f, ok := mv.(float64); ok {
						return int(f)
					}
				}
			case float64:
				return int(t)
			case string:
				if n, err := strconv.Atoi(t); err == nil {
					return n
				}
			}
		}
	}
	maj := v.Major()
	switch {
	case maj >= 21:
		return 21
	case maj >= 18:
		return 17
	case maj >= 16:
		return 16
	default:
		return 8
	}
}

func (v *Version) Loaders() []string {
	if v.JSON == nil {
		return nil
	}
	libs := ""
	if ls, ok := asSlice(v.JSON["libraries"]); ok {
		for _, l := range ls {
			if m, ok := asMap(l); ok {
				if s, ok := asString(m["name"]); ok {
					libs += s + " "
				}
			}
		}
	}
	lower := strings.ToLower(v.Name + " " + libs)
	var tags []string
	if strings.Contains(lower, "forge") {
		tags = append(tags, "Forge")
	}
	if strings.Contains(lower, "neoforge") {
		tags = append(tags, "NeoForge")
	}
	if strings.Contains(lower, "fabric") {
		tags = append(tags, "Fabric")
	}
	if strings.Contains(lower, "quilt") {
		tags = append(tags, "Quilt")
	}
	if strings.Contains(lower, "optifine") {
		tags = append(tags, "OptiFine")
	}
	if strings.Contains(lower, "liteloader") {
		tags = append(tags, "LiteLoader")
	}
	return tags
}

func (v *Version) IsOld() bool {
	if v.JSON == nil {
		return false
	}
	_, ok := v.JSON["minecraftArguments"]
	return ok
}

// ---------- rules 过滤 ----------

func filterRules(entries []interface{}, osName string) []interface{} {
	out := []interface{}{}
	for _, e := range entries {
		lib, ok := asMap(e)
		if !ok {
			continue
		}
		rulesRaw, ok := asSlice(lib["rules"])
		if !ok {
			out = append(out, e)
			continue
		}
		allPass := true
		for _, r := range rulesRaw {
			if !matchRule(r, osName) {
				allPass = false
				break
			}
		}
		if allPass {
			out = append(out, e)
		}
	}
	return out
}

func matchRule(rule interface{}, osName string) bool {
	r, ok := asMap(rule)
	if !ok {
		return true
	}
	action := "allow"
	if a, ok := asString(r["action"]); ok {
		action = a
	}
	okFlag := true
	if osRule, ok := asMap(r["os"]); ok {
		if n, ok := asString(osRule["name"]); ok && n != "unknown" && n != osName {
			okFlag = false
		}
		if a, ok := asString(osRule["arch"]); ok {
			if a == "x86" && arch != "x86" {
				okFlag = false
			}
			if strings.Contains(a, "arm") && !strings.Contains(a, "x86") {
				okFlag = false
			}
		}
	}
	if features, ok := asMap(r["features"]); ok {
		if dv, ok := features["is_demo_user"]; ok {
			if b, ok := dv.(bool); ok && b {
				okFlag = false
			}
		}
	}
	return (action == "allow") == okFlag
}

// ---------- libraries 补全 ----------

// GetLibraries 返回合并、过滤后的库列表（含 natives 标记）。
func (v *Version) GetLibraries() []Library {
	result := []Library{}
	if v.JSON == nil {
		return result
	}
	libsRaw, ok := asSlice(v.JSON["libraries"])
	if !ok {
		return result
	}
	osName := "windows"
	for _, entry := range filterRules(libsRaw, osName) {
		lib, ok := asMap(entry)
		if !ok {
			continue
		}
		name, _ := asString(lib["name"])
		downloads, _ := asMap(lib["downloads"])
		artifact, _ := asMap(downloads["artifact"])
		classifiers, _ := asMap(downloads["classifiers"])

		parts := strings.Split(name, ":")
		var classifier string
		if len(parts) >= 4 {
			classifier = parts[3]
		}
		native := ""
		if classifier != "" && strings.HasPrefix(classifier, "natives-") {
			native = classifier
		} else {
			if nativesObj, ok := asMap(lib["natives"]); ok {
				if n, ok := asString(nativesObj["windows"]); ok {
					native = n
				}
			}
			if native != "" {
				native = strings.ReplaceAll(native, "${arch}", arch)
			}
		}
		if native != "" && native != "natives-windows" {
			if strings.HasSuffix(native, "-arm64") || strings.HasSuffix(native, "-x86") {
				continue
			}
		}

		var path, url, sha1 string
		var size int64
		if native != "" {
			var entry map[string]interface{}
			if cls, ok := asMap(classifiers[native]); ok {
				entry = cls
			}
			if entry == nil && classifier != "" {
				entry = artifact
			}
			if entry != nil {
				path, _ = asString(entry["path"])
				url, _ = asString(entry["url"])
				sha1, _ = asString(entry["sha1"])
				if sz, ok := entry["size"].(float64); ok {
					size = int64(sz)
				}
			}
			if path == "" && name != "" {
				path = mavenPath(name, native)
			}
		} else {
			if p, ok := asString(artifact["path"]); ok && p != "" {
				path = p
			} else if name != "" {
				path = mavenPath(name, "")
			}
			url, _ = asString(artifact["url"])
			sha1, _ = asString(artifact["sha1"])
			if sz, ok := artifact["size"].(float64); ok {
				size = int64(sz)
			}
		}
		if path == "" {
			continue
		}
		local := filepath.Join(v.McFolder, "libraries", filepath.FromSlash(path))
		result = append(result, Library{
			Name:    name,
			Path:    path,
			Local:   local,
			URL:     url,
			SHA1:    sha1,
			Size:    size,
			Natives: native,
		})
	}
	return result
}

// mavenPath 把 maven 坐标转成仓库相对路径。
func mavenPath(name string, classifier string) string {
	parts := strings.Split(name, ":")
	if len(parts) < 3 {
		return ""
	}
	group, artifact, version := parts[0], parts[1], parts[2]
	if len(parts) >= 4 && classifier == "" {
		classifier = parts[3]
	}
	base := strings.ReplaceAll(group, ".", "/") + "/" + artifact + "/" + version + "/" + artifact + "-" + version
	if classifier != "" {
		return base + "-" + classifier + ".jar"
	}
	return base + ".jar"
}

// ---------- 扫描 ----------

// ScanVersions 扫描 versions 目录，返回版本信息列表。
func ScanVersions(mcFolder string) []map[string]interface{} {
	result := []map[string]interface{}{}
	versionsDir := filepath.Join(mcFolder, "versions")
	entries, err := os.ReadDir(versionsDir)
	if err != nil {
		return result
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Slice(names, func(i, j int) bool {
		return strings.ToLower(names[i]) > strings.ToLower(names[j])
	})
	for _, name := range names {
		jp := VersionJSONPath(mcFolder, name)
		if _, err := os.Stat(jp); err != nil {
			continue
		}
		v := NewVersion(mcFolder, name)
		if v.Error != "" {
			result = append(result, map[string]interface{}{
				"name": name, "state": "error", "error": v.Error,
			})
			continue
		}
		result = append(result, map[string]interface{}{
			"name":         v.Name,
			"type":         v.Type(),
			"release_time": v.ReleaseTime(),
			"major":        v.Major(),
			"loaders":      strings.Join(v.Loaders(), ","),
			"java":         v.JavaRequirement(),
			"state":        "ok",
		})
	}
	return result
}

// GetVersion 取得（并解析）版本对象。
func GetVersion(mcFolder, name string) *Version {
	return NewVersion(mcFolder, name)
}
