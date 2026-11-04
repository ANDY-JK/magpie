// What the rest of the window calls of the Routing page, and the words its
// rows say of a request, which the Usage page's Requests and the context
// card say too — before the page's own script has come.
//
// Routing is one of the heavy pages, declared ahead of its script (pages.js,
// kernel.js's magpie.lazy): its page is in the window from the start, with
// its title, and its script comes the first time the reader opens it. What
// the window needs until then is here: kindName, purposeOf, purposeOptions,
// swapWhy and routedWhy say what a call was for and who answered, and
// openRoute, newGroupWith and panelRoutingShown bring the page in first and
// then call what it set in their place (client.js).
//
// Classic script, loaded with the window (index.html), after kernel.js and
// before app.js: what runs as it loads touches neither the page nor app.js.

// what a call was for when it isn't a turn of the conversation, as
// Codex names it (x-openai-subagent): its own guardian review of an
// approval, a thread's title, memories… — each on the model Codex picks
// for it, so a list of Luna calls under a Sol composer reads as it is.
// A web search is magpie's own, run for a model that can't search on the
// model it searches with (a DeepSeek chat showing GPT calls, #314)
const knownKind = (k) => Object.hasOwn(REQUEST_KINDS, k) ? REQUEST_KINDS[k] : null;
const kindName = (k) => knownKind(k) ? t(knownKind(k).name) : k;
window.kindName = kindName; // the Usage page's Requests say it too (#714)
// The catalog is generated from usage.PurposeKinds; unknown names stay literal.
const purposeOf = (kind) => !kind ? "unmarked" : knownKind(kind)?.purpose || "kind:" + kind;
const purposeOptions = (ids, selected) => [...new Set([...ids, ...(selected ? [selected] : [])])].sort().map((id) => {
  const kind = id.slice(5);
  return { v: id, name: id === "unmarked" ? t("Unmarked") : kindName(kind), note: id === "unmarked"
    ? t("No purpose was recorded; this may be a conversation turn or an older record.")
    : knownKind(kind) ? "" : t("Unrecognized request purpose") };
});
window.purposeOf = purposeOf;
window.purposeOptions = purposeOptions; // the Usage page's Requests filter by it
// ClinePass names a reply by the route it picked (its VMC,
// vmc/deepseek-v4p1-flash-1017-contributor-fallbacks), not by a model:
// such a reply is said as its route, with what was asked of it and
// whether it said who answered (zRain on Discord, 10-24). pin is the
// provider the try asked for; a Usage row doesn't keep it (pinUnknown).
const clineRoute = (tr) => /^vmc\//.test(tr.served || "") && /(^|\/)cline-pass\//.test(tr.model || "");
function swapWhy(tr, o = {}) {
  if (!clineRoute(tr)) return t("The vendor was asked for {sent}, but its reply says {served} answered: likely another model. The same model under a dated name or spelled otherwise isn't marked.", { sent: tr.model, served: tr.served });
  const s = t("ClinePass answered {sent} through {served}, a route it picks itself: the name is the route's, not another model's.", { sent: tr.model, served: tr.served });
  if (tr.upstream) return s + " " + t("Its reply says {upstream} answered.", { upstream: tr.upstream });
  if (o.pinUnknown) return s + " " + t("Its reply didn't say which provider answered.");
  if (tr.pin) return s + " " + t("{pin} was asked for, but its reply didn't say which provider answered, so whether ClinePass kept to it is unknown.", { pin: tr.pin });
  const tick = /(^|\/)cline-pass\/(deepseek|glm)/i.test(tr.model) ? " " + t("To ask for the maker's own API, tick “{tick}” in the provider's settings.", { tick: t("DeepSeek and GLM models only from their makers' own APIs") }) : "";
  return s + " " + t("No provider was asked for, and its reply didn't say which one answered.") + tick;
}
window.swapWhy = swapWhy; // the Usage page's Requests say it too
const routedWhy = (tr) => t("{sent} is a routing group of the remote magpie, and it routed the request to {served}: the group picking one of its models, not the vendor swapping the model.", { sent: tr.model, served: tr.served });
window.routedWhy = routedWhy; // and so do they

// ---------- what needs the page's own code ----------
// The page puts its own in their place as it mounts (client.js); a call
// before then brings it in first. mount is the kernel's way in: asking for
// it gives back the promise of a page that has run it. A page that didn't
// come leaves the shim where it was, and the call must say so then rather
// than call itself.
function routingComes(what, shim) {
  return Promise.resolve(magpie.call("routing", "mount")).then(() => {
    const real = window[what];
    if (real === shim) throw new Error(t("The Routing page didn't open."));
    return real;
  });
}
// openRoute opens one request in the page (the Usage page's Requests and
// the context card); newGroupWith opens a new group's editor with a model
// in it (the picker and a provider's editor)
for (const name of ["openRoute", "newGroupWith"]) {
  const shim = (...args) => routingComes(name, shim).then((real) => real(...args));
  window[name] = shim;
}
// The tray panel asks on every tab switch, and only its Routing tab has
// anything of the page's: the page comes the first time that tab is shown.
window.panelRoutingShown = function panelRoutingShown() {
  if (document.body.dataset.ptab !== "routing") return;
  return routingComes("panelRoutingShown", panelRoutingShown).then((real) => real());
};
