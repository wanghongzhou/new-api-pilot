package docscheck

import (
	"os"
	"path/filepath"
	"strings"
)

type designTextContract struct {
	path      string
	required  []string
	forbidden []string
}

func (current *checker) checkAuthoritativeDesignContracts() {
	contracts := []designTextContract{
		{
			path: "docs/多站点运营管理平台-详细设计-04-业务功能与平台API.md",
			required: []string{
				"六类 system-task 类型（task type）",
				"六类 current 各请求一次",
				"五个 current 完整性槽",
			},
			forbidden: []string{"五类 system-task", "五类 typed 结果"},
		},
		{
			path: "docs/多站点运营管理平台-详细设计-07-运维与验收.md",
			required: []string{
				"六类 task type，六类 current 各请求一次并映射为五个 current 完整性槽",
				"默认 390×844 移动端并额外覆盖 375px 窄屏",
			},
			forbidden: []string{"list/current 五类白名单", "桌面和 375px 移动端", "desktop 与 375px mobile", "桌面/375px"},
		},
		{
			path: "docs/多站点运营管理平台-详细设计-06-前端实现.md",
			required: []string{
				"四个 Playwright project",
				"chromium-tablet-768",
				"chromium-tablet-1024",
				"不另建第五个 project",
			},
		},
	}

	for _, contract := range contracts {
		absolutePath := filepath.Join(current.root, filepath.FromSlash(contract.path))
		payload, err := os.ReadFile(absolutePath)
		if err != nil {
			current.add("design-contract", absolutePath, "read authoritative design: %v", err)
			continue
		}
		text := string(payload)
		for _, required := range contract.required {
			if !strings.Contains(text, required) {
				current.add("design-contract", absolutePath, "missing required contract wording %q", required)
			}
		}
		for _, forbidden := range contract.forbidden {
			if strings.Contains(text, forbidden) {
				current.add("design-contract", absolutePath, "contains obsolete contract wording %q", forbidden)
			}
		}
	}
}
