// The Metrics page's Japanese words, the English the key: the kernel adds
// them to i18n.js's (magpie.strings). This file is one of the four
// (i18n.zh.js, i18n.zh-TW.js, i18n.ja.js, i18n.de.js); the page loads only
// the one for the language it is in. A key with no word here reads in
// English.
magpie.strings({
  ja: {
    "Read now": "今すぐ読み込む",
    "Read at {time}": "{time} に読み込み",
    "Running now": "実行中",
    "Turns waiting on tools": "ツール結果待ちのターン",
    "Waiting ones say where they wait: a key's queue, or room in the minute.": "待機中のリクエストは何を待つかを示します: キーのキュー、またはこの1分の余り。",
    "The gateway is served by another magpie on this computer.": "ゲートウェイはこのコンピューター上の別の magpie が提供しています。",
    "queued for a key or account": "キーかアカウントのキューで待機中",
    "waiting for the minute": "この1分の余りを待機中",
    "asking a vendor": "プロバイダーに送信中",
    "masking its words": "マスキング中",
    "The process itself": "このプロセス",
    "live memory": "使用中のメモリ",
    "taken from the system": "システムから確保した分",
    "returned so far": "返還済みの分",
    "Go routines": "Go ルーチン",
    "CPU share": "CPU 使用率",
    "running since": "起動してから",
    "Memory, the last four hours": "メモリ、過去4時間",
    "CPU, the last four hours": "CPU、過去4時間",
    "Idle past half a minute, the gateway hands its memory back.": "30秒以上アイドルが続くと、ゲートウェイはメモリをシステムに返します。",
    "Calls by hour": "時間ごとの呼び出し",
    "Calls an hour": "1時間あたりの呼び出し",
    "Failed an hour": "1時間あたりの失敗",
    "An average of {n} calls an hour, {m} failed": "平均1時間あたり {n} 回の呼び出し、失敗 {m} 回",
    "Waiting requests aren't counted by hour yet.": "待機中のリクエストはまだ時間ごとには数えていません。",
    "Shown elsewhere": "他のページにあるもの",
    "Which provider answered, and what it cost": "どのプロバイダーが応答し、いくらかかったか",
    "Which route served what, and its totals": "どのルートが何を処理したか、その合計",
    "Who rests, and what failed lately": "誰が休止中か、最近の失敗",
  },
});
