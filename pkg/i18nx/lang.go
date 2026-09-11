package i18nx

import "strings"

// 项目内的标准语言码，与前端 src/i18n.ts 的 languages 保持一致
const (
	LangZhCN = "zh_CN"
	LangZhHK = "zh_HK"
	LangEnUS = "en_US"
	LangJaJP = "ja_JP"
	LangRuRU = "ru_RU"
	LangPtBR = "pt_BR"
	LangEsES = "es_ES"
	LangIdID = "id_ID"
	LangKoKR = "ko_KR"
	LangFrFR = "fr_FR"
)

// NormalizeLang 把各种写法的语言标识归一为标准语言码：大小写不敏感，"-" 与 "_"
// 等价（前端只发标准码，但第三方直连 API 可能传 en-us、ZH-CN）。
//
// 无法识别的值原样返回，空串保持空串：各调用方的缺省语言不同（页面接口默认中文，
// 词条翻译缺省即英文原文），由调用方自行兜底，这里不替它们决定。
func NormalizeLang(lang string) string {
	switch strings.ToLower(strings.ReplaceAll(lang, "-", "_")) {
	case "zh", "cn", "zh_cn":
		return LangZhCN
	case "zh_hk", "zh_tw", "zh_mo":
		return LangZhHK
	case "en", "en_us", "en_gb":
		return LangEnUS
	case "ja", "jp", "ja_jp":
		return LangJaJP
	case "ru", "ru_ru":
		return LangRuRU
	case "pt", "pt_br", "pt_pt":
		return LangPtBR
	case "es", "es_es", "es_419", "es_mx":
		return LangEsES
	case "id", "id_id", "in", "in_id":
		return LangIdID
	case "ko", "kr", "ko_kr":
		return LangKoKR
	case "fr", "fr_fr", "fr_ca", "fr_be", "fr_ch":
		return LangFrFR
	default:
		return lang
	}
}
