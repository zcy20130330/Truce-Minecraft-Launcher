// Package auth 负责登录（对应 prototype 的 launcher/auth.py）。
// 当前仅实现离线登录；微软正版 OAuth 留待后续迭代。
package auth

import (
	"crypto/md5"
	"fmt"
)

// Account 是登录结果。
type Account struct {
	Name        string
	UUID        string
	AccessToken string // 离线模式下等于 UUID（与 Java 端约定一致）
	UserType    string
}

// OfflineLogin 离线登录：UUID 采用 Java 的 UUID.nameUUIDFromBytes("OfflinePlayer:"+name)
// 算法，保证离线 UUID 与 Java 版客户端算出的完全一致，服务器能正确识别。
func OfflineLogin(username string) *Account {
	if username == "" {
		username = "Player"
	}
	sum := md5.Sum([]byte("OfflinePlayer:" + username))
	b := sum[:]
	// version 3（name-based MD5）
	b[6] = (b[6] & 0x0F) | 0x30
	// IETF variant
	b[8] = (b[8] & 0x3F) | 0x80

	uid := fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
	return &Account{
		Name:        username,
		UUID:        uid,
		AccessToken: uid,
		UserType:    "legacy",
	}
}

// ToMap 转成启动参数需要的扁平映射（对应 Python 的 login 字典键名）。
func (a *Account) ToMap() map[string]string {
	return map[string]string{
		"name":         a.Name,
		"uuid":         a.UUID,
		"access_token": a.AccessToken,
		"user_type":    a.UserType,
	}
}
