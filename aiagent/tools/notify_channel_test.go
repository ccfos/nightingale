package tools

import "testing"

// paramsFingerprint 是 list_notify_rule_custom_params 去重分组的依据：
// 同一组参数无论 map 遍历顺序如何都必须得到同一指纹，不同取值必须区分开。
func TestParamsFingerprint(t *testing.T) {
	a := map[string]interface{}{"access_token": "tok-1", "bot_name": "夜莺"}
	b := map[string]interface{}{"bot_name": "夜莺", "access_token": "tok-1"}
	if paramsFingerprint(a) != paramsFingerprint(b) {
		t.Fatalf("same params should yield same fingerprint: %q vs %q", paramsFingerprint(a), paramsFingerprint(b))
	}

	c := map[string]interface{}{"access_token": "tok-2", "bot_name": "夜莺"}
	if paramsFingerprint(a) == paramsFingerprint(c) {
		t.Fatalf("different token should yield different fingerprint")
	}

	// key/value 串拼接不能产生歧义碰撞：值里含 ";"/"=" 时不得与拆成两个键值对的集合同指纹
	d := map[string]interface{}{"k": `a";x="1`}
	e := map[string]interface{}{"k": "a", "x": "1"}
	if paramsFingerprint(d) == paramsFingerprint(e) {
		t.Fatalf("ambiguous concat collision: %q", paramsFingerprint(d))
	}
}
