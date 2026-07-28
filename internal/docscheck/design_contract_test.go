package docscheck

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAuthoritativeDesignContractsFreezeSystemTaskAndMobileBaselines(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(root, "docs", "多站点运营管理平台-详细设计-04-业务功能与平台API.md"), `
六类 system-task 类型（task type）
六类 current 各请求一次
五个 current 完整性槽
`)
	writeTestFile(t, filepath.Join(root, "docs", "多站点运营管理平台-详细设计-06-前端实现.md"), `
默认移动 390×844 项目
额外覆盖 375px
不增加第三个 Playwright project
`)
	writeTestFile(t, filepath.Join(root, "docs", "多站点运营管理平台-详细设计-07-运维与验收.md"), `
六类 task type，六类 current 各请求一次并映射为五个 current 完整性槽
默认 390×844 移动端并额外覆盖 375px 窄屏
`)

	current := &checker{root: root}
	current.checkAuthoritativeDesignContracts()
	if len(current.issues) != 0 {
		t.Fatalf("valid design contracts produced issues: %#v", current.issues)
	}

	writeTestFile(t, filepath.Join(root, "docs", "多站点运营管理平台-详细设计-04-业务功能与平台API.md"), `
平台提供五类 system-task。
全局/站点只读任务列表与统计、五类 typed 结果。
`)
	writeTestFile(t, filepath.Join(root, "docs", "多站点运营管理平台-详细设计-07-运维与验收.md"), `
system-task 仅采 Root list/current 五类白名单。
desktop 与 375px mobile 覆盖。
`)

	current = &checker{root: root}
	current.checkAuthoritativeDesignContracts()
	if !containsIssue(current.issues, "obsolete contract wording") ||
		!containsIssue(current.issues, "missing required contract wording") {
		t.Fatalf("system-task/mobile drift was not rejected: %#v", current.issues)
	}
}
