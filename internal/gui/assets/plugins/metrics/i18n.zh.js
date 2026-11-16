// The Metrics page's Simplified Chinese words, the English the key: the
// kernel adds them to i18n.js's (magpie.strings). This file is one of the
// four (i18n.zh.js, i18n.zh-TW.js, i18n.ja.js, i18n.de.js); the page loads
// only the one for the language it is in. A key with no word here reads in
// English.
magpie.strings({
  zh: {
    "Read now": "立即读取",
    "Read at {time}": "读取于 {time}",
    "Running now": "正在运行",
    "Turns waiting on tools": "等待工具结果的回合",
    "Waiting ones say where they wait: a key's queue, or room in the minute.": "排队的请求会说明在等什么：等某个 key 的队列，或等这一分钟的余量。",
    "The gateway is served by another magpie on this computer.": "网关由本机上的另一个 magpie 提供。",
    "queued for a key or account": "在 key 或账号的队列中",
    "waiting for the minute": "等待这一分钟的余量",
    "asking a vendor": "已发往服务商",
    "masking its words": "正在脱敏",
    "The process itself": "本进程",
    "live memory": "在用内存",
    "taken from the system": "向系统申请的",
    "returned so far": "已归还的",
    "Go routines": "Go 例程",
    "CPU share": "CPU 占用",
    "running since": "已运行",
    "Memory, the last four hours": "内存，最近四小时",
    "CPU, the last four hours": "CPU，最近四小时",
    "Idle past half a minute, the gateway hands its memory back.": "空闲超过半分钟，网关就把内存交还给系统。",
    "Calls by hour": "按小时的调用",
    "Calls an hour": "每小时调用",
    "Failed an hour": "每小时失败",
    "An average of {n} calls an hour, {m} failed": "平均每小时 {n} 次调用，{m} 次失败",
    "Waiting requests aren't counted by hour yet.": "排队的请求还没有按小时统计。",
    "Shown elsewhere": "别处已有",
    "Which provider answered, and what it cost": "哪个供应商应答、花了多少",
    "Which route served what, and its totals": "哪条路由服务了什么，以及总计",
    "Who rests, and what failed lately": "谁在休息、最近什么失败",
  },
});
