// What the window calls of the Masking page before its script has come.
//
// Insights › Masking is one of the heavy pages, declared ahead of its
// script (pages.js, kernel.js's magpie.lazy): its page is in the window
// from the start, with its title, and its script comes the first time the
// reader opens it. Settings › Privacy's Try it says what masking did with
// the text it tries (app.js), and asks of the page's own matchMatches
// which stand-ins are in it: here, before the page.
//
// Classic script, loaded with the window (index.html), after kernel.js and
// before app.js: what runs as it loads touches neither the page nor app.js.

// maskMatches is text's stand-ins as matchAll gives them: its {{…}}
// placeholders, and the made-up keys "Keep a key's form" sent (redact's
// shape.go), the keys of info that are no placeholder, which only the
// record's own list tells from the text around them (#3599)
const maskLookRes = new WeakMap();
function maskMatches(text, info) {
  let re = maskLookRes.get(info);
  if (!re) {
    const looks = [...info.keys()].filter((p) => p && !p.startsWith("{{")).sort((a, b) => b.length - a.length)
      .map((p) => p.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"));
    re = looks.length ? new RegExp(PLACEHOLDER.source + "|" + looks.join("|"), "g") : PLACEHOLDER;
    maskLookRes.set(info, re);
  }
  return text.matchAll(re);
}
