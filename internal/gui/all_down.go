package gui

// allDownText is the notification that every channel that can answer a
// model is resting (the gateway's AllDownAlert, the review's D-01, 11-16),
// in the language (en, zh, zh-TW, ja or de): when a model is named, every
// key and account of it; the notice is the outage's, not the
// recovery's.
func allDownText(lang, model string) (title, body string) {
	switch lang {
	case "zh":
		if model != "" {
			title, body = "渠道全部不可用", "「"+model+"」的每个密钥或账号都在故障后休眠，暂无渠道可应答"
		} else {
			title, body = "渠道全部不可用", "每个密钥或账号都在故障后休眠，暂无渠道可应答"
		}
	case "zh-TW":
		if model != "" {
			title, body = "渠道全部不可用", "「"+model+"」的每個金鑰或帳號都在故障後休眠，暫無渠道可應答"
		} else {
			title, body = "渠道全部不可用", "每個金鑰或帳號都在故障後休眠，暫無渠道可應答"
		}
	case "ja":
		if model != "" {
			title, body = "全チャネル応答不能", model+" の全キー・アカウントが失敗後の休止中で、応答できるチャネルがありません"
		} else {
			title, body = "全チャネル応答不能", "全キー・アカウントが失敗後の休止中で、応答できるチャネルがありません"
		}
	case "de":
		if model != "" {
			title, body = "Alle Kanäle außer Betrieb", "Jeder Schlüssel und jedes Konto für "+model+" ruht nach einer Störung: kein Kanal kann gerade antworten"
		} else {
			title, body = "Alle Kanäle außer Betrieb", "Jeder Schlüssel und jedes Konto ruht nach einer Störung: kein Kanal kann gerade antworten"
		}
	default:
		if model != "" {
			title, body = "No channel can answer", "Every key or account of "+model+" rests after a failure, so none can answer right now"
		} else {
			title, body = "No channel can answer", "Every key or account rests after a failure, so none can answer right now"
		}
	}
	return title, body
}
