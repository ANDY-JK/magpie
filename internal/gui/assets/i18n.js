// magpie speaks English by default. Every other language is a file of its
// own (i18n.zh.js and the rest), loaded for the language the page is in;
// this file holds what they share: the locale, t(), the notices, and the
// loading of those files. Keys are the English strings as written in app.js
// and index.html; a missing key falls back to English, so a half-translated
// language still works. {name} slots are filled by t(str, vars).
const I18N = {};

// localeWordsHere names the languages whose words are here: each language
// file sets its own, so a block another script made for plugin words
// (kernel.js's magpie.strings) is never taken for the page's.
const localeWordsHere = new Set();

// LOCALE_FILES is each language's file: each adds its words to I18N.
// English has none: t() falls back to the key.
const LOCALE_FILES = { zh: "i18n.zh.js", "zh-TW": "i18n.zh-TW.js", ja: "i18n.ja.js", de: "i18n.de.js" };

// loadLocaleWords brings in lang's words when the page doesn't have them
// yet (a language picked in Settings after the page opened), in a script
// added to the page. It settles with whether they came; a file that won't
// come leaves English, as a key with no word does.
const localeLoading = {};
function loadLocaleWords(lang) {
  if (!LOCALE_FILES[lang] || localeWordsHere.has(lang)) return null;
  localeLoading[lang] ??= new Promise((settle) => {
    const s = document.createElement("script");
    s.src = LOCALE_FILES[lang];
    s.onload = s.onerror = () => { delete localeLoading[lang]; settle(localeWordsHere.has(lang)); };
    document.head.append(s);
  });
  return localeLoading[lang];
}

// magpieLocale is the page's language, as startLocaleWords picks it. The
// plugins' word files (plugins/<id>/i18n.js) read it, so the page is never
// half in one language and half in another: they load the words of this
// language, not one of their own.
window.magpieLocale = "en";

// magpieLocaleLoaders are the plugins' own word loaders: each plugin's
// i18n.js adds one here, and a language picked after the page opened is
// asked of every one of them, so a plugin's words are here before the
// window is drawn again in that language.
window.magpieLocaleLoaders = [];

// pluginLocaleWords asks every plugin for a language's words: each loader
// brings its own file in and settles with whether it came. It settles with
// nothing to wait for when they are all here already.
function pluginLocaleWords(lang) {
  const waiting = [];
  for (const load of window.magpieLocaleLoaders) {
    try {
      const p = load(lang);
      if (p) waiting.push(p);
    } catch (err) {
      console.error("[magpie] a plugin's words", err);
    }
  }
  return waiting.length ? Promise.all(waiting).then(() => {}) : null;
}

// startLocaleWords picks the page's language as setLocale does — a
// ?locale= URL parameter first, then the saved one (boot.js), then the
// system's — and gets its words while the page is being parsed, so app.js
// draws in it from its first line, as it did when this file held every
// language. Written into the page rather than read afterwards: a script
// added once the page is parsed would draw English first. The server can't
// make this choice: it sees neither the URL's ?locale= nor the system's
// language, and "system" isn't a language.
(function startLocaleWords() {
  if (typeof document === "undefined" || !document.scripts) return;
  const asked = new URLSearchParams(location.search).get("locale") || window.bootPrefs?.lang || "system";
  const lang = asked === "system" ? sysLocale((navigator.language || "").toLowerCase()) : asked;
  // the plugins' word files read this, so they load the language this page
  // is in and not one of their own
  window.magpieLocale = lang;
  if (LOCALE_FILES[lang] && document.readyState === "loading") document.write('<script src="' + LOCALE_FILES[lang] + '"></script>');
})();

// Every notice magpie's agents give after a change (Notices in
// internal/agent/notice.go), as tNotice matches them; each has its
// translation in the language files. TestEveryNoticeIsTranslated keeps the
// two the same.
const NOTICES = [
  "Air runs magpie's models through OpenCode's ACP server, and OpenCode isn't installed here: install OpenCode (npm i -g opencode-ai), then pick Magpie in a new Air task.",
  "Ante reads its catalog at start-up — restart open ante sessions to use this.",
  "AstrBot reads cmd_config.json at start-up and writes it back whole when its WebUI saves settings — restart AstrBot to use this, before saving anything in its WebUI.",
  "QwenPaw reads its providers at start-up — restart QwenPaw once after connecting it, and after magpie's models change; a model picked for an agent after that is used from its next message.",
  "AtomCode reads its settings at start-up — restart open atomcode sessions to use this.",
  "Claude Desktop reads this at start-up — quit and reopen it to run on magpie (Code and Cowork, no Anthropic sign-in).",
  "Claude Desktop reads this at start-up — quit and reopen it to sign in with Anthropic again.",
  "Chats and projects from your Anthropic sign-in stay in Desktop's own profile, not shown on magpie; nothing is deleted, and turning this off brings them back.",
  "Your chats and projects from before are back there.",
  "Cline reads its provider as a session starts — open sessions keep the model they have; new ones use this. Reload VS Code's window for its extension.",
  "Cline reads its provider as a session starts — open sessions keep the model they have; new ones use this. Reload VS Code's window for its extension; Cline's desktop app keeps the model picked in its own composer — pick magpie's there.",
  "Cline's desktop app keeps the model and effort picked in its own composer — pick magpie's there; this sets Cline's CLI and VS Code extension.",
  "CodeBuddy Code reads its model when a session starts — restart open codebuddy sessions, or run /clear in them, to use this.",
  "Codex builds its model list at start-up — restart the Codex app, open codex sessions and the app-server they share ({command}) to see this.",
  "Codex builds its model list at start-up, and the app-server open codex sessions share started before this: restart it (Restart on magpie's Agents page, or {command}) and the Codex app to see this.",
  "Codex's own multi_agent_v2 is on in the config magpie routes, which takes over the multi-agent V1 magpie's models ask for; turn it off in Codex's config to run subagents on magpie's models.",
  "Codex's multi-agent V1 is on, but magpie couldn't read Codex's config to check its own multi_agent_v2.",
  "Command Code wants its own sign-in (cmd login) even for models through magpie.",
  "Devin reads its model at start-up — restart open Devin sessions to use this.",
  "Droid reads its settings at start-up — restart open droid sessions to use this.",
  "Empryo reads its config at start-up — restart open empryo sessions to use this.",
  "Gemini CLI reads its settings at start-up — restart open gemini sessions to see this.",
  "Goose loads its providers at start-up — quit and reopen Goose (and open goose sessions) to use magpie's models.",
  "Grok Build reads its settings at start-up — restart open grok sessions to use this.",
  "Hermes reads its settings at start-up — restart open Hermes sessions to use this.",
  "In Air, start a new task and pick Magpie in its agent menu: magpie's models and routing groups are in its model menu.",
  "In your JetBrains IDE (2026.3, or 2026.2 with the Air plugin), pick Magpie in Air's agent menu — restart the IDE if it isn't listed yet: magpie's models and routing groups are in its model menu.",
  "Install GitHub Copilot Chat in {agent} to see magpie's models in its Chat.",
  "It reads its settings at start-up — restart open Command Code sessions to use this.",
  "Kimi Code reads its settings at start-up — restart open kimi sessions to use this.",
  "Kimi Code reads this as it runs; in its web or desktop app, reopen Settings › Providers or reload the page to see new models.",
  "MothX reads its settings at start-up — restart open mothx sessions to use this.",
  "Zoo Code reads magpie's profile when it starts — run Developer: Reload Window in VS Code to use this.",
  "jcode reads its settings at start-up — restart open jcode sessions to use this.",
  "OpenClaw takes this up without a restart. A chat that already has a model keeps it: send /model default in that chat to move it onto this one.",
  "Magpie is a custom provider in Grok App's Settings › Account. A chat can pick magpie's models in its model picker, and Grok App stays on its current route until you press Use on magpie there.",
  "MiniMax Code's desktop app keeps the model last picked in it: restart the app, then pick a magpie model once in its model menu (under magpie). Open mcode sessions need a restart too.",
  "MiniMax Code reads its settings at start-up — restart open mcode sessions to use this.",
  "Mister Morph's Console uses this for new tasks; restart open morph chats to use it there.",
  "Muse Code reads its settings at start-up — restart open muse sessions to use this.",
  "New dsh sessions start on this. A session made before keeps its model, open now or reopened later, until you pick another in it with /model or the model button by its message box. A dsh started with -p or in a terminal reads it at start-up.",
  "DeepSeek Harness Desktop hasn't been opened yet: it starts on this the first time it opens.",
  "OpenBot reads its custom endpoints at start-up — restart OpenBot to see magpie's models in its model picker, before adding or editing an endpoint there.",
  "OpenChamber takes the new model for new sessions; sessions already open keep theirs.",
  "Kiro Crew takes the new harness for new sessions; sessions already open keep theirs.",
  "Kiro Crew runs on {harness}, so magpie connected {harness} too; turning Kiro Crew off disconnects it again.",
  "Kiro Crew runs {harness} through {adapter}, which magpie didn't find here: install it with {install}, then start a new session.",
  "Kiro Crew's config.local.json sets its harness (agent.acp_backend), and that wins over config.json, so magpie leaves it alone: set it there to a harness connected to magpie.",
  "OpenChamber's project {project} has a default model of its own ({model}), which it uses there instead.",
  "OpenPencil reads its settings when it starts and writes them all back when it quits — quit OpenPencil before changing this, then open it and pick a magpie model in its chat's model picker.",
  "NewMax keeps its settings in memory while it runs and writes them all back when one changes — quit NewMax before changing this, then open it and pick a magpie model in its model picker.",
  "Pencil reads its custom models at start-up — restart Pencil to see magpie's models in its model picker.",
  "Antigravity reads its gateway at start-up — restart Antigravity to use magpie's models. On magpie it doesn't use your Google sign-in; turning it off here gives that back. An agy not connected on its own row follows it too.",
  "Antigravity's {path} sets a gateway of its own, which wins over ~/.antigravity/settings.json, so it doesn't reach magpie",
  "AGY_LLM_GATEWAY_URL is set to {url}, which wins over ~/.antigravity/settings.json, so Antigravity started with it doesn't reach magpie",
  "Qwen Code reads a session's model at start-up — start a new session, or /model anew, to use this.",
  "Reasonix Studio 2.x and the native 1.39.x/2.x CLI read global settings at startup. Start a new process to load changes. Project/session models can override Executor and Plan.",
  "Restart Zed if it still asks for an API key: magpie has configured its gateway credential in the system credential store.",
  "Snow CLI reads its profile at start-up — restart open snow sessions to use this.",
  "WSL {distro} is in {net} networking with localhostForwarding=false in %UserProfile%\\.wslconfig, so 127.0.0.1 there, where its {agent} was pointed, isn't relayed to Windows, where magpie listens. Remove localhostForwarding=false under [wsl2] (it is on by default) and run wsl --shutdown.",
  "WSL {distro} is in {net} networking, not mirrored, so its {agent} was pointed at Windows ({url}) rather than 127.0.0.1; magpie listens there for WSL, and it answers once Windows' firewall lets WSL in. For 127.0.0.1, set networkingMode=mirrored under [wsl2] in %UserProfile%\\.wslconfig and run wsl --shutdown, then pick the model again.",
  "WSL {distro} isn't running: magpie shows what it last saw there, and starts it only to change something.",
  "ZCode reads its providers at start-up — restart ZCode to see magpie's models in its picker.",
  "agy takes magpie's gateway and model only from how it is started — start it with {command}",
  "Type agy in a new terminal: magpie added it to {profiles}, to start agy on magpie's gateway and model (in a terminal open now: {command}).",
  "an open Claude Code session keeps the effort it started with — restart it to use this.",
  "an open Claude Code session keeps the ultracode it started with — restart it to use this.",
  "dsh reads its config at start-up — restart open dsh sessions to use this.",
  "dsh's web search goes through magpie, which has nothing to search with yet: set up a provider that searches by itself (DeepSeek, Claude, Codex, …) or a search API under Settings › Web search.",
  "fx reads its settings at start-up — restart open fx sessions to use this.",
  "magpie couldn't ask WSL {distro} its networking mode (it has no wslinfo) and %UserProfile%\\.wslconfig doesn't set it to mirrored, so its {agent} was pointed at Windows ({url}) rather than 127.0.0.1; magpie listens there for WSL, and it answers once Windows' firewall lets WSL in. For 127.0.0.1, set networkingMode=mirrored under [wsl2] in %UserProfile%\\.wslconfig and run wsl --shutdown, then pick the model again.",
  "magpie restarted the app-server codex sessions share, which none was on, so new codex sessions have this; restart the Codex app, if it is open, to see it there.",
  "magpie's models are in Copilot Chat's model picker in your JetBrains IDE, under magpie, once the IDE restarts. They need Copilot signed in to GitHub on a plan with Bring Your Own Key: Copilot Free, Pro and Pro+ have it, Business and Enterprise when the organisation's policy allows it.",
  "magpie's models are in {agent}'s Chat model picker under magpie, from GitHub Copilot Chat {version}'s OpenAI Compatible provider, which it offers only while it is signed in to GitHub with a personal Copilot plan.",
  "magpie's models are in {agent}'s Chat model picker, under magpie, in each of its profiles (VS Code 1.122 or later). If they don't show, run Developer: Reload Window in {agent}.",
  "omp reads its settings at start-up — restart open omp sessions to use this.",
  "{agent} in WSL {distro} builds its model list at start-up — restart it, the app-server its sessions share ({command} there) and the Codex app's WSL connection to see this.",
  "{agent} in WSL {distro} loads its providers at start-up — restart open goose sessions to use magpie's models.",
  "{agent} in WSL {distro} reads a session's model at start-up — start a new session, or /model anew, to use this.",
  "{agent} in WSL {distro} reads its catalog at start-up — restart open ante sessions to use this.",
  "{agent} in WSL {distro} reads its config at start-up — restart open dsh sessions to use this.",
  "{agent} in WSL {distro} reads its config at start-up — restart open empryo sessions to use this.",
  "{agent} in WSL {distro} reads its config at start-up — restart open mimo sessions to use this.",
  "{agent} in WSL {distro} reads its config at start-up — restart open opencode sessions to use this.",
  "{agent} in WSL {distro} reads its model when a session starts — restart open codebuddy sessions, or run /clear in them, to use this.",
  "{agent} in WSL {distro} reads its profile at start-up — restart open snow sessions to use this.",
  "{agent} in WSL {distro} reads its provider as a session starts — open sessions keep the model they have; new ones use this. Reload VS Code's window for its extension.",
  "{agent} in WSL {distro} reads its settings as a session starts — open sessions keep the model they have; new ones use this.",
  "{agent} in WSL {distro} reads its settings at start-up — restart open Hermes sessions to use this.",
  "{agent} in WSL {distro} reads its settings at start-up — restart open atomcode sessions to use this.",
  "{agent} in WSL {distro} reads its settings at start-up — restart open crush sessions to use this.",
  "{agent} in WSL {distro} reads its settings at start-up — restart open droid sessions to use this.",
  "{agent} in WSL {distro} reads its settings at start-up — restart open fx sessions to use this.",
  "{agent} in WSL {distro} reads its settings at start-up — restart open gemini sessions to see this.",
  "{agent} in WSL {distro} reads its settings at start-up — restart open grok sessions to use this.",
  "{agent} in WSL {distro} reads its settings at start-up — restart open kimi sessions to use this.",
  "{agent} in WSL {distro} reads this as it runs; in its web or desktop app, reopen Settings › Providers or reload the page to see new models.",
  "{agent} in WSL {distro} takes magpie's gateway and model only from how it is started — type agy in a new terminal there: magpie put it in the shell's profile.",
  "{agent} in WSL {distro} reads its settings at start-up — restart open mothx sessions to use this.",
  "{agent} in WSL {distro} reads its settings at start-up — restart open mcode sessions to use this.",
  "{agent} in WSL {distro} reads its settings at start-up — restart open muse sessions to use this.",
  "{agent} in WSL {distro} reads its settings at start-up — restart open omp sessions to use this.",
  "{agent} in WSL {distro} reads its settings at start-up — start a new reasonix process to use this.",
  "{agent} in WSL {distro} uses this for new tasks in its Console — restart open morph chats to use it there.",
  "{agent} in WSL {distro} wants its own sign-in there (cmd login) even for models through magpie, and reads its settings at start-up — restart open Command Code sessions to use this.",
  "{agent} reads its settings as a session starts — open sessions keep the model they have; new ones use this.",
  "{agent} reads magpie's gateway from CURSOR_LOCAL_AGENT_BASE_URL and CURSOR_LOCAL_AGENT_API_KEY, now set for your user: quit it (the app, not only its window) and open it again, as it keeps the model list it was first given until it quits. A base URL or API key set in its Open configuration or a model's settings comes first, so leave both empty (or set the base URL to {url} and the key to {key}).",
  "{agent}'s Chat features must be enabled (chat.disableAIFeatures=false) and its product.json must include defaultChatAgent and trustedExtensionAuthAccess for GitHub.copilot-chat. Then run Developer: Reload Window in {agent}.",
  "{agent}'s GitHub Copilot Chat {version} has neither a Custom Endpoint nor an OpenAI Compatible provider, so it can't list magpie's models.",
  "{agent}'s daemon runs on magpie's gateway: open {agent} again (reload its window if it is open). It still needs you signed in with your Claude account, which magpie never sends on; pick a model of magpie's in its model menu.",
  "{file} sets {var}, so Claude Code sends its requests to {cloud}, not to magpie: take it out there to go through magpie.",
  "{var} is set in your shell's environment, so Claude Code started there sends its requests to {cloud}, not to magpie: take it out of your shell's profile to go through magpie.",
  "~/.dsh/settings.yaml sets its own DeepSeek endpoint or key, which dsh puts over magpie's; clear it in dsh's Models page to go through magpie.",
  "magpie doesn't restart {agent} on this computer.",
  "{agent} isn't running on the old settings any more.",
  "magpie couldn't ask {agent} to quit. Quit it yourself, then open it again.",
  "{agent} is still open; it may be asking something in its window. Answer it there, or quit it, then open it again.",
  "{agent} is still running. Quit it, then open it again.",
  "{agent} quit, but didn't open again. Open it yourself.",
  "The {agent} app isn't running any more. It opens on the new account.",
  "magpie couldn't restart the {agent} app's app-server. Quit the app and open it again.",
  "The {agent} app's app-server ended, but the app didn't start another. Quit the app and open it again.",
  "magpie couldn't tell whether the {agent} app restarted. Quit the app and open it again.",
];

// The active language: a ?locale= URL parameter wins, then the saved
// preference, then the system.
let locale = "en";
// localeWanted is the language asked for last: two picked quickly, the
// second before the first's file is in, leave the second, as two saves do
let localeWanted = "en";
function setLocale(pref) {
  const p = params.get("locale") || pref || "system";
  const sys = (navigator.language || "").toLowerCase();
  const was = locale;
  const want = p === "system" ? sysLocale(sys) : p;
  localeWanted = want;
  // a language whose words aren't here yet: they come in first — the core's,
  // then every plugin's — and the page is drawn in them then (relang.js),
  // never halfway in English. A file that won't come leaves English, as a
  // missing key does.
  const changing = was !== want;
  const coming = changing && !localeWordsHere.has(want) ? loadLocaleWords(want) : null;
  const drawn = () => {
    if (locale === want || localeWanted !== want) return; // here, or since replaced
    applyLocale(want, was);
    import("./relang.js").then(() => relangWords(), () => {});
  };
  if (coming) return void coming.then(() => (localeWanted === want ? pluginLocaleWords(want) : null)).then(drawn, drawn);
  if (changing) {
    // the core's words are here, the plugins' may not be: they come in
    // before the window is drawn again, as the core's do
    const plug = pluginLocaleWords(want);
    if (plug) return void plug.then(drawn, drawn);
  }
  applyLocale(want, was);
}

// applyLocale makes want the page's language: its tag on <html>, and the
// window's own words in it. It says whether the language changed.
function applyLocale(want, was) {
  locale = want;
  window.magpieLocale = want;
  // set only when it changes: settings come back on every refresh, and a
  // view that redraws for a new language shouldn't redraw for the same one
  const lang = LANG_TAGS[locale] || "en";
  if (document.documentElement.lang !== lang) document.documentElement.lang = lang;
  // the window's own words are rewritten only for a new language: a
  // settings refresh, and every return to the window (loadBack), sends the
  // same one again, and rewrote all of them and re-fitted the header each
  // time. kernel.js still translates a page's markup as it is added.
  if (was !== locale) translateStatic();
  return was !== locale;
}

// sysLocale is the language a system tag reads in: Traditional Chinese for
// Taiwan, Hong Kong, Macau and any zh-Hant tag, Simplified for other Chinese.
function sysLocale(sys) {
  if (/^zh-(tw|hk|mo|hant)\b/.test(sys)) return "zh-TW";
  return sys.startsWith("zh") ? "zh" : sys.startsWith("ja") ? "ja" : sys.startsWith("de") ? "de" : "en";
}

// intlLang is the tag every date, time and number the GUI shows is
// formatted in: the GUI's language, never the system's alone. English takes
// the system's own English (en-GB keeps its day-month order) and is plain
// "en" on a system in another language, so a Chinese Mac set to English
// reads "Oct 11", not "10月11日" (#1843). Never format with no locale, [] or
// undefined: those follow the system, not the language picked here.
const LANG_TAGS = { zh: "zh-CN", "zh-TW": "zh-TW", ja: "ja-JP", de: "de-DE" };
function intlLang() {
  if (LANG_TAGS[locale]) return LANG_TAGS[locale];
  const sys = navigator.language || "";
  return /^en(-|$)/i.test(sys) ? sys : "en";
}

function t(s, vars) {
  let out = I18N[locale]?.[s] ?? s;
  if (vars) for (const k in vars) out = out.split("{" + k + "}").join(vars[k]);
  return out;
}

// tNotice is an agent's advice after a change (state.notice) in the
// reader's language (#1508). magpie writes it from the templates in
// NOTICES, with names, paths and commands in their {slots}, and may join
// several with a space: each is matched back to its template, the longest
// first, and translated. One that doesn't match whole (a plugin's own
// advice) reads as given.
let noticeRes;
function tNotice(s) {
  if (!s || !I18N[locale]) return s;
  if (I18N[locale][s]) return I18N[locale][s];
  noticeRes ??= NOTICES.map((en) => {
    const parts = en.split(/\{(\w+)\}/), names = [];
    let re = "";
    parts.forEach((p, i) => {
      if (i % 2 === 0) re += p.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
      // a slot never holds a sentence's end, so it can't take in the
      // notice joined after it
      else names.push(p), (re += i === parts.length - 2 && parts[i + 1] === "" ? "((?:(?!\\. ).)+)" : "((?:(?!\\. ).)+?)");
    });
    return { en, names, re: new RegExp("^" + re + "(?= |$)"), fixed: parts.filter((_, i) => i % 2 === 0).join("").length };
  });
  const out = [];
  let rest = s;
  while (rest) {
    let hit = null;
    for (const n of noticeRes) {
      const m = n.re.exec(rest);
      if (m && (!hit || m[0].length > hit.m[0].length || (m[0].length === hit.m[0].length && n.fixed > hit.n.fixed))) hit = { n, m };
    }
    if (!hit) return s;
    const vars = {};
    hit.n.names.forEach((k, i) => (vars[k] ??= hit.m[i + 1]));
    out.push(t(hit.n.en, vars));
    rest = rest.slice(hit.m[0].length).replace(/^ /, "");
  }
  return out.join(/^(zh|ja)/.test(locale) ? "" : " ");
}

// tHeading is a release's section heading in the GUI's language (Wei He
// on Discord: past versions' headings switched between Chinese and
// English). The Chinese notes are a model's translation, which keeps the
// English headings in some releases and words them its own way in others
// (功能 or 新功能; 错误修复, 缺陷修复 or Bug 修复), and a release without
// Chinese, or a GUI in a language the notes don't come in, has English
// ones. A heading said either way is the GUI's own word for it; one it
// doesn't know reads as written.
const NOTE_HEADINGS = {
  "Features": ["features", "new features", "功能", "新功能", "新特性", "特性"],
  "Bug Fixes": ["bug fixes", "bug fix", "bugfixes", "fixes", "错误修复", "缺陷修复", "bug 修复", "bug修复", "问题修复", "修复", "錯誤修正", "錯誤修復"],
  "Improvements": ["improvements", "改进", "优化", "改進"],
  "Changes": ["changes", "更新", "变更", "變更"],
  "Documentation": ["documentation", "docs", "文档"],
  "Performance Improvements": ["performance improvements", "performance", "性能改进", "性能优化", "效能改進"],
  "Security": ["security", "安全", "安全性"],
  "Provider Changes": ["provider changes", "提供商变更", "供应商变更", "供應商變更"],
};
let noteHeadingKeys;
function tHeading(s) {
  noteHeadingKeys ??= new Map(Object.entries(NOTE_HEADINGS).flatMap(([k, ws]) => ws.map((w) => [w, k])));
  const k = noteHeadingKeys.get(s.trim().replace(/[:：]$/, "").replace(/\s+/g, " ").toLowerCase());
  return k ? t(k) : s;
}

// withNotice is a status line followed by the agent's advice, as the
// language ends a sentence.
function withNotice(msg, notice) {
  return msg + (/^(zh|ja)/.test(locale) ? "。" : ". ") + tNotice(notice);
}

// Static text in index.html: elements marked data-t (text) or data-tt
// (title) keep their English in a data attribute and are re-rendered on a
// language change. A data-tt with a value is the title's English itself
// (#653: the Hide accounts buttons had no title, so no tooltip showed).
function translateStatic() {
  for (const e of document.querySelectorAll("[data-t]")) {
    if (e.dataset.en === undefined) e.dataset.en = e.textContent;
    e.textContent = t(e.dataset.en);
  }
  for (const e of document.querySelectorAll("[data-tt]")) {
    if (e.dataset.enTitle === undefined) e.dataset.enTitle = e.dataset.tt || e.title;
    e.title = t(e.dataset.enTitle);
    // data-keys: its shortcut, the Mac's before the | and the others' after
    // (Settings' ⌘, #670); a browser tab keeps its own
    const keys = e.dataset.keys?.split("|")[/^Mac/.test(navigator.platform) ? 0 : 1];
    if (keys && !window.bootPrefs?.web) e.title += " (" + keys + ")";
  }
  for (const e of document.querySelectorAll("[data-tp]")) {
    if (e.dataset.enPlaceholder === undefined) e.dataset.enPlaceholder = e.placeholder;
    e.placeholder = t(e.dataset.enPlaceholder);
  }
  // another language makes the tabs wider in the header as it was fitted, and
  // they ran under the magpie and the window's buttons (kiwiflysky on X)
  window.fitTop?.();
}
