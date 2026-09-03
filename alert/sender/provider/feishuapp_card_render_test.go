package provider

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
)

// 卡片 JSON 由模板手拼，逗号/引号位置易错；两个用例分别覆盖
// 带截图与不带截图两种拼接分支，产物必须是合法 JSON。
func TestRenderFeishuCardJSON_ValidJSON(t *testing.T) {
	req := &NotifyRequest{
		TplContent: map[string]interface{}{},
		Events:     []*models.AlertCurEvent{{Hash: "h"}},
	}
	for name, imageKey := range map[string]string{"withoutImage": "", "withImage": "img_key_1"} {
		t.Run(name, func(t *testing.T) {
			out, err := renderFeishuCardJSON(req, `Test "Title"`, "## body\nwith \"quotes\"", imageKey)
			if err != nil {
				t.Fatal(err)
			}
			if !json.Valid([]byte(out)) {
				t.Fatalf("rendered card is not valid JSON:\n%s", out)
			}
			if imageKey != "" && !strings.Contains(out, "img_key_1") {
				t.Fatal("image key missing from rendered card")
			}
		})
	}
}
