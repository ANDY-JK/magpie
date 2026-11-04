// The Usage page's words, one language to a file (i18n.zh.js and the rest):
// the page parses only the language it is in, as i18n.js does for the
// window's own words. English needs none of them: the key is the English
// word. The language is the one i18n.js picked (magpieLocale), so the page
// is never half in one language and half in another; one picked after the
// page opened is asked of every plugin before the window is drawn again in
// it (magpieLocaleLoaders).
(() => {
  const base = new URL(".", document.currentScript.src);
  const FILES = { zh: "i18n.zh.js", "zh-TW": "i18n.zh-TW.js", ja: "i18n.ja.js", de: "i18n.de.js" };
  const here = new Set(); // the languages whose words are here
  const coming = {};
  // load brings in a language's words and settles with whether they came; a
  // file that won't come leaves English, as a key with no word does.
  function load(lang) {
    const file = FILES[lang];
    if (!file || here.has(lang)) return null;
    if (coming[lang]) return coming[lang];
    coming[lang] = new Promise((settle) => {
      const s = document.createElement("script");
      s.src = new URL(file, base).href;
      s.onload = () => { here.add(lang); delete coming[lang]; settle(true); };
      s.onerror = () => { delete coming[lang]; settle(false); };
      document.head.append(s);
    });
    return coming[lang];
  }
  // while the page is parsed, the language's words are written in, so the
  // page's first paint is in it and no English flashes
  const lang = window.magpieLocale;
  if (lang && FILES[lang] && document.readyState === "loading") {
    document.write('<script src="' + new URL(FILES[lang], base).href + '"></script>');
    here.add(lang);
  }
  // i18n.js asks every plugin for a language picked after the page opened
  (window.magpieLocaleLoaders ||= []).push(load);
})();
