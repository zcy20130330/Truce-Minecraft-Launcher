package java

import "fmt"

// Recommendation 描述"某个游戏版本该用哪个 Java"的结论。
type Recommendation struct {
	// Required 是游戏版本 JSON 里声明的 Java 主版本要求（0 表示版本文件未标注）。
	Required int `json:"required"`
	// Best 是本机最合适的 Java（nil 表示本机没有可用版本）。
	Best map[string]interface{} `json:"best"`
	// NeedInstall 为 true 时表示建议自动安装缺失的 Java。
	NeedInstall bool `json:"need_install"`
	// InstallMajor 是建议自动安装的主版本。
	InstallMajor int `json:"install_major"`
	// Reason 是给界面直接展示的一句话说明。
	Reason string `json:"reason"`
}

// MajorOf 取一条 Java 记录的主版本号。
func MajorOf(j map[string]interface{}) int {
	if j == nil {
		return 0
	}
	m, _ := j["major"].(int)
	return m
}

// PathOf 取一条 Java 记录的 java.exe 路径。
func PathOf(j map[string]interface{}) string {
	if j == nil {
		return ""
	}
	p, _ := j["path"].(string)
	return p
}

// Recommend 依据游戏版本要求挑选 Java：
//   - 精确同主版本优先（PCL/HMCL 的一致做法：mod 生态对同版本最友好）
//   - 其次取 >= 要求版本中最低的那一个
//   - 一个都不满足时判定为"需要安装"，避免拿低版本硬跑导致启动必败
//
// requirement <= 0 表示版本文件未标注 Java 要求，此时取本机最高版本。
func Recommend(javaList []map[string]interface{}, requirement int) Recommendation {
	r := Recommendation{Required: requirement}

	if requirement <= 0 {
		if len(javaList) == 0 {
			r.NeedInstall = true
			r.InstallMajor = 21
			r.Reason = "本机未检测到 Java，建议安装 Java 21"
			return r
		}
		r.Best = javaList[0]
		r.InstallMajor = MajorOf(javaList[0])
		r.Reason = fmt.Sprintf("版本未标注 Java 要求，使用本机最高的 Java %d", MajorOf(javaList[0]))
		return r
	}

	if best := SelectJava(javaList, requirement); best != nil {
		r.Best = best
		r.InstallMajor = MajorOf(best)
		if MajorOf(best) == requirement {
			r.Reason = fmt.Sprintf("已匹配 Java %d", requirement)
		} else {
			r.Reason = fmt.Sprintf("该版本要求 Java %d，将使用 Java %d", requirement, MajorOf(best))
		}
		return r
	}

	r.NeedInstall = true
	r.InstallMajor = requirement
	if len(javaList) == 0 {
		r.Reason = fmt.Sprintf("本机未检测到 Java，请安装 Java %d", requirement)
	} else {
		highest := MajorOf(javaList[0])
		r.Reason = fmt.Sprintf("该版本要求 Java %d，本机只有 Java %d，请安装", requirement, highest)
	}
	return r
}
