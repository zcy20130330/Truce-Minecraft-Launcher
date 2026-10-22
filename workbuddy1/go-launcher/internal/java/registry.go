package java

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

// javaRegRoots 是各发行版在注册表中登记 JDK/JRE 的父键。
var javaRegRoots = []string{
	`SOFTWARE\JavaSoft\JDK`,
	`SOFTWARE\JavaSoft\Java Development Kit`,
	`SOFTWARE\JavaSoft\JRE`,
	`SOFTWARE\JavaSoft\Java Runtime Environment`,
	`SOFTWARE\Eclipse Adoptium\JDK`,
	`SOFTWARE\Eclipse Foundation\JDK`,
	`SOFTWARE\Microsoft\JDK`,
	`SOFTWARE\Azul Systems\Zulu`,
	`SOFTWARE\Amazon Corretto`,
	`SOFTWARE\BellSoft\LibericaJDK`,
	`SOFTWARE\Semeru\JDK`,
}

// javaHomeValues 是各发行版用来记录安装目录的值名，按优先级尝试。
var javaHomeValues = []string{"JavaHome", "Path", "InstallationPath", "Home", "InstallationDirectory"}

// registryJavaHomes 从注册表收集 JDK/JRE 的安装根目录。
// 覆盖 HKLM/HKCU 的 64 位与 32 位（WOW6432Node）两个视图。
func registryJavaHomes() []string {
	var out []string
	seen := map[string]bool{}

	push := func(home string) {
		home = strings.TrimSpace(strings.Trim(home, `"`))
		if home == "" {
			return
		}
		key := strings.ToLower(home)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, home)
	}

	for _, base := range []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER} {
		for _, view := range []uint32{0, registry.WOW64_32KEY} {
			access := uint32(registry.READ) | view
			for _, root := range javaRegRoots {
				k, err := registry.OpenKey(base, root, access)
				if err != nil {
					continue
				}
				names, err := k.ReadSubKeyNames(-1)
				_ = k.Close()
				if err != nil {
					continue
				}
				for _, name := range names {
					sub, err := registry.OpenKey(base, root+`\`+name, access)
					if err != nil {
						continue
					}
					for _, val := range javaHomeValues {
						if v, _, err := sub.GetStringValue(val); err == nil && v != "" {
							push(v)
							break
						}
					}
					_ = sub.Close()
				}
			}
		}
	}
	return out
}
