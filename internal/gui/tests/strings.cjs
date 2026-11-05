// The window's words in each language, as the page has them: one file per
// language (assets/i18n.zh.js and the rest, which i18n.js loads for the
// page's language), and each plugin's (assets/plugins/<id>/i18n.<lang>.js,
// which the plugin's own i18n.js loads for it). files says which file said
// which; a key two files say is in twice.
const fs = require("node:fs");
const path = require("node:path");

const assets = path.resolve(__dirname, "../assets");
const LANGS = ["zh", "zh-TW", "ja", "de"];

function strings() {
  const I18N = {};
  const files = [];
  // each language file adds its words to I18N, as it does in the page
  for (const lang of LANGS) {
    const file = `i18n.${lang}.js`;
    const src = fs.readFileSync(path.join(assets, file), "utf8");
    new Function("I18N", "localeWordsHere", src)(I18N, new Set());
    files.push({ file, src, dict: { [lang]: I18N[lang] } });
  }
  const twice = [], plugins = [];
  const dir = path.join(assets, "plugins");
  for (const id of fs.readdirSync(dir).sort()) {
    const loader = path.join(dir, id, "i18n.js");
    if (!fs.existsSync(loader)) continue;
    // the loader holds no words: it brings in the language the page is in
    const entry = { file: path.relative(assets, loader), id, dict: {} };
    plugins.push(entry);
    files.push({ file: entry.file, src: fs.readFileSync(loader, "utf8"), dict: {} });
    for (const lang of LANGS) {
      const file = path.join(dir, id, `i18n.${lang}.js`);
      if (!fs.existsSync(file)) continue;
      const src = fs.readFileSync(file, "utf8");
      let said;
      new Function("magpie", src)({ strings: (d) => { said = d; } });
      if (Object.keys(said).length !== 1 || !(lang in said)) {
        throw new Error(`${id}/i18n.${lang}.js says ${Object.keys(said).join(", ")}`);
      }
      entry.dict[lang] = said[lang];
      files.push({ file: path.relative(assets, file), src, dict: { [lang]: said[lang] } });
    }
    for (const [lang, words] of Object.entries(entry.dict)) {
      const to = (I18N[lang] ||= {});
      for (const [k, v] of Object.entries(words)) {
        if (k in to) twice.push(`${lang} ${JSON.stringify(k)} in ${entry.file}`);
        else to[k] = v;
      }
    }
  }
  return { I18N, files, twice, plugins };
}

// allWords has the page load every language's words, core and plugins: each
// language is a file of its own now (i18n.js, plugins/<id>/i18n.js), and a
// check that reads another language's key in the page must ask for it first.
async function allWords(page) {
  await page.evaluate(async () => {
    for (const l of ["zh", "zh-TW", "ja", "de"]) { await loadLocaleWords(l); await pluginLocaleWords(l); }
  });
}

module.exports = { strings, allWords };
