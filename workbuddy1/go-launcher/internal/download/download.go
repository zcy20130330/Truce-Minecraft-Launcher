// Package download 负责库文件/资源文件下载补全与校验（对应 prototype 的 launcher/download.py）。
// 镜像规则同 PCL：优先走 bmclapi 镜像，失败回退官方源。纯标准库实现。
package download

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"truce-launcher/internal/versions"
)

// UserAgent 是下载时的 UA。
const UserAgent = "Truce-Launcher/0.1 (Minecraft Launcher)"

// Item 是一个待下载/待校验的文件项。
type Item struct {
	Name  string // 库坐标或资源 key
	Local string // 磁盘绝对路径
	URL   string // 官方下载 URL
	SHA1  string
	Size  int64
	Key   string // 资源对象的 key（仅资源文件用）
}

func httpDownload(url, dest string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", UserAgent)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp := dest + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	buf := make([]byte, 65536)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				f.Close()
				return werr
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			return rerr
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}

func sha1Of(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha1.New()
	buf := make([]byte, 65536)
	for {
		n, rerr := f.Read(buf)
		if n > 0 {
			h.Write(buf[:n])
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return "", rerr
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// fileOK 判断文件是否通过校验（存在 + 大小匹配 + SHA1 匹配）。
func fileOK(path, sha1 string, size int64) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if size > 0 && info.Size() != size {
		return false
	}
	if sha1 != "" {
		got, err := sha1Of(path)
		if err != nil {
			return false
		}
		return strings.EqualFold(got, sha1)
	}
	return true
}

// ---------- 镜像 ----------

func mirror(original, kind string) []string {
	if kind == "library" {
		return []string{
			strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(original,
				"https://piston-data.mojang.com", "https://bmclapi2.bangbang93.com/maven"),
				"https://piston-meta.mojang.com", "https://bmclapi2.bangbang93.com/maven"),
				"https://libraries.minecraft.net", "https://bmclapi2.bangbang93.com/maven"),
			strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(original,
				"https://piston-data.mojang.com", "https://bmclapi2.bangbang93.com/libraries"),
				"https://piston-meta.mojang.com", "https://bmclapi2.bangbang93.com/libraries"),
				"https://libraries.minecraft.net", "https://bmclapi2.bangbang93.com/libraries"),
		}
	}
	if kind == "asset" {
		return []string{strings.ReplaceAll(original,
			"https://resources.download.minecraft.net", "https://bmclapi2.bangbang93.com/assets")}
	}
	return []string{
		strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(original,
			"https://piston-data.mojang.com", "https://bmclapi2.bangbang93.com"),
			"https://piston-meta.mojang.com", "https://bmclapi2.bangbang93.com"),
			"https://launcher.mojang.com", "https://bmclapi2.bangbang93.com"),
			"https://launchermeta.mojang.com", "https://bmclapi2.bangbang93.com"),
	}
}

func urlOrder(original, kind string, preferMirror bool) []string {
	mirrors := mirror(original, kind)
	if preferMirror {
		return append(mirrors, original)
	}
	return append([]string{original}, mirrors...)
}

func downloadFile(original, dest, kind, sha1 string, size int64, preferMirror bool) bool {
	for _, url := range urlOrder(original, kind, preferMirror) {
		if err := httpDownload(url, dest); err != nil {
			fmt.Printf("[download] 失败 %s: %v\n", url, err)
			continue
		}
		if fileOK(dest, sha1, size) {
			return true
		}
		os.Remove(dest)
	}
	return false
}

// ---------- 缺失文件列表 ----------

// MissingLibraries 返回需要下载的库文件列表。
func MissingLibraries(mcFolder string, libs []versions.Library, checkFiles bool) []Item {
	missing := []Item{}
	for _, lib := range libs {
		if !checkFiles {
			if _, err := os.Stat(lib.Local); err == nil {
				continue
			}
		}
		if fileOK(lib.Local, lib.SHA1, lib.Size) {
			continue
		}
		official := lib.URL
		if official == "" {
			rel, err := filepath.Rel(filepath.Join(mcFolder, "libraries"), lib.Local)
			if err == nil {
				official = "https://libraries.minecraft.net/" + filepath.ToSlash(rel)
			}
		}
		missing = append(missing, Item{
			Name:  lib.Name,
			Local: lib.Local,
			URL:   official,
			SHA1:  lib.SHA1,
			Size:  lib.Size,
		})
	}
	return missing
}

// MissingAssets 根据资源索引返回缺失的资源文件列表。
func MissingAssets(mcFolder string, indexJSON map[string]interface{}, checkFiles bool) []Item {
	missing := []Item{}
	objects, ok := indexJSON["objects"].(map[string]interface{})
	if !ok {
		return missing
	}
	virtual := false
	if v, ok := indexJSON["virtual"].(bool); ok {
		virtual = v
	}
	for key, objRaw := range objects {
		obj, ok := objRaw.(map[string]interface{})
		if !ok {
			continue
		}
		hash, _ := obj["hash"].(string)
		var size int64
		if s, ok := obj["size"].(float64); ok {
			size = int64(s)
		}
		if len(hash) < 2 {
			continue
		}
		var local string
		if virtual {
			local = filepath.Join(mcFolder, "assets", "virtual", "legacy", filepath.FromSlash(key))
		} else {
			local = filepath.Join(mcFolder, "assets", "objects", hash[:2], hash)
		}
		if !checkFiles {
			if _, err := os.Stat(local); err == nil {
				continue
			}
		}
		if fileOK(local, hash, size) {
			continue
		}
		official := "https://resources.download.minecraft.net/" + hash[:2] + "/" + hash
		missing = append(missing, Item{
			Key:   key,
			Local: local,
			URL:   official,
			SHA1:  hash,
			Size:  size,
		})
	}
	return missing
}

// DownloadMany 批量下载；progress(done,total,name)。返回 (ok,fail)。
func DownloadMany(items []Item, kind string, preferMirror bool, progress func(done, total int, name string)) (int, int) {
	total := len(items)
	if total == 0 {
		return 0, 0
	}
	okCount, failCount := 0, 0
	for _, it := range items {
		if downloadFile(it.URL, it.Local, kind, it.SHA1, it.Size, preferMirror) {
			okCount++
		} else {
			failCount++
		}
		if progress != nil {
			progress(okCount+failCount, total, it.Name)
		}
	}
	return okCount, failCount
}
