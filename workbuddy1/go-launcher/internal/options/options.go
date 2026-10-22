// Package options 负责游戏内 options.txt 维护（当前用于自动设置语言）。
// MC 语言存于 <游戏目录>/options.txt 的 lang 项（如 lang:zh_cn）。
// 必须在游戏进程启动之前写入——游戏退出时会自行覆写该文件。
package options

import (
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
)

const optionKey = "lang"

// decodeRaw 尝试按候选编码解码 options.txt，返回 (text, encoding)。
// 优先级：utf-8（含纯 ASCII）-> gbk -> latin-1 兜底。
func decodeRaw(raw []byte) (string, string) {
	if utf8.Valid(raw) {
		return string(raw), "utf-8"
	}
	if dec := simplifiedchinese.GBK.NewDecoder(); dec != nil {
		if out, err := dec.Bytes(raw); err == nil {
			return string(out), "gbk"
		}
	}
	return string(raw), "latin-1"
}

// ReadOption 读取某一项的值，文件不存在或没有该项返回 ("", false)。
func ReadOption(gameDir, key string) (string, bool) {
	path := filepath.Join(gameDir, "options.txt")
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	text, _ := decodeRaw(raw)
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, key+":") {
			return strings.TrimSpace(line[len(key)+1:]), true
		}
	}
	return "", false
}

// WriteOption 幂等写入：已有该项则替换，没有则追加。返回是否成功。
func WriteOption(gameDir, key, value string) bool {
	if err := os.MkdirAll(gameDir, 0o755); err != nil {
		return false
	}
	path := filepath.Join(gameDir, "options.txt")

	lines := []string{}
	encodingName := "utf-8"
	if _, err := os.Stat(path); err == nil {
		raw, err := os.ReadFile(path)
		if err != nil {
			return false
		}
		text, enc := decodeRaw(raw)
		encodingName = enc
		lines = strings.Split(text, "\n")
	}

	target := key + ":" + value
	hit := false
	out := []string{}
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, key+":") {
			if hit { // 重复项（异常文件）：丢弃多余行
				continue
			}
			out = append(out, target)
			hit = true
		} else {
			out = append(out, line)
		}
	}
	if !hit {
		out = append(out, target)
	}

	content := strings.Join(out, "\n") + "\n"
	var data []byte
	if encodingName == "gbk" {
		if enc := simplifiedchinese.GBK.NewEncoder(); enc != nil {
			if b, err := enc.Bytes([]byte(content)); err == nil {
				data = b
			} else {
				data = []byte(content)
			}
		} else {
			data = []byte(content)
		}
	} else {
		data = []byte(content)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return false
	}
	return true
}

// ApplyLanguage 设置游戏语言。language 为 "none" 时不干预游戏内现有设置。
func ApplyLanguage(gameDir, language string) string {
	if language == "" || language == "none" {
		if cur, ok := ReadOption(gameDir, optionKey); ok {
			return "保持游戏现有语言 lang:" + cur + "（可在设置里改成跟随系统）"
		}
		return "未设置语言（保持游戏内现有设置）"
	}
	old, _ := ReadOption(gameDir, optionKey)
	if !WriteOption(gameDir, optionKey, language) {
		return "写入 options.txt 失败"
	}
	if old == language {
		return "语言已是 lang:" + language
	}
	if old == "" {
		return "已设置游戏语言 lang:" + language
	}
	return "已设置游戏语言 lang:" + language + "（原 " + old + "）"
}
