package gui

// allDownText is the notification that no channel can answer a model
// (the gateway's AllDownAlert, the review's D-01, 11-16), in the language
// (en, zh, zh-TW, ja or de): when a model is named, no channel of it. It
// names no reason, as an outage is every key or account of the model
// resting or the proxy or network down, and says no time to wait: the
// notice is the outage's, not the recovery's.
func allDownText(lang, model string) (title, body string) {
	switch lang {
	case "zh":
		if model != "" {
			title, body = "渠道全部不可用", "「"+model+"」暂无渠道可应答"
		} else {
			title, body = "渠道全部不可用", "暂无渠道可应答"
		}
	case "zh-TW":
		if model != "" {
			title, body = "渠道全部不可用", "「"+model+"」暫無渠道可應答"
		} else {
			title, body = "渠道全部不可用", "暫無渠道可應答"
		}
	case "ja":
		if model != "" {
			title, body = "全チャネル応答不能", model+" に応答できるチャネルがありません"
		} else {
			title, body = "全チャネル応答不能", "応答できるチャネルがありません"
		}
	case "de":
		if model != "" {
			title, body = "Alle Kanäle außer Betrieb", "Kein Kanal kann "+model+" gerade beantworten"
		} else {
			title, body = "Alle Kanäle außer Betrieb", "Kein Kanal kann gerade antworten"
		}
	default:
		if model != "" {
			title, body = "No channel can answer", "No channel can answer "+model+" right now"
		} else {
			title, body = "No channel can answer", "No channel can answer right now"
		}
	}
	return title, body
}
