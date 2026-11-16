// The Metrics page's Traditional Chinese words, the English the key: the
// kernel adds them to i18n.js's (magpie.strings). This file is one of the
// four (i18n.zh.js, i18n.zh-TW.js, i18n.ja.js, i18n.de.js); the page loads
// only the one for the language it is in. A key with no word here reads in
// English.
magpie.strings({
  "zh-TW": {
    "Read now": "立即讀取",
    "Read at {time}": "讀取於 {time}",
    "Running now": "正在執行",
    "Turns waiting on tools": "等待工具結果的回合",
    "Waiting ones say where they wait: a key's queue, or room in the minute.": "排隊的請求會說明在等什麼：等某個 key 的佇列，或等這一分鐘的餘量。",
    "The gateway is served by another magpie on this computer.": "閘道由本機上的另一個 magpie 提供。",
    "queued for a key or account": "在 key 或帳號的佇列中",
    "waiting for the minute": "等待這一分鐘的餘量",
    "asking a vendor": "已送至供應商",
    "masking its words": "正在遮罩",
    "The process itself": "本程序",
    "live memory": "在用記憶體",
    "taken from the system": "向系統申請的",
    "returned so far": "已歸還的",
    "Go routines": "Go 例程",
    "CPU share": "CPU 佔用",
    "running since": "已執行",
    "Memory, the last four hours": "記憶體，最近四小時",
    "CPU, the last four hours": "CPU，最近四小時",
    "Idle past half a minute, the gateway hands its memory back.": "空閒超過半分鐘，閘道就把記憶體交還給系統。",
    "Calls by hour": "按小時的呼叫",
    "Calls an hour": "每小時呼叫",
    "Failed an hour": "每小時失敗",
    "An average of {n} calls an hour, {m} failed": "平均每小時 {n} 次呼叫，{m} 次失敗",
    "Waiting requests aren't counted by hour yet.": "排隊的請求還沒有按小時統計。",
    "Shown elsewhere": "別處已有",
    "Which provider answered, and what it cost": "哪個供應商應答、花了多少",
    "Which route served what, and its totals": "哪條路由服務了什麼，以及總計",
    "Who rests, and what failed lately": "誰在休息、最近什麼失敗",
  },
});
