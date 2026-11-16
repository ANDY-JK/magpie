// A key row's day and its allowance card, drawn by renderKeyAccounts and
// kept here so the core's scripts stay light (app.js imports this as an
// editor first draws its keys, as it does ratewindows.js). The day is the
// Usage summary's own key groups (Summary.providerKeys, each keyed
// provider#fingerprint): the rows' ranking read has no key dimension, so
// this is its own read, every 30s at most (readLoads's pattern), the editor
// drawn again only when it changed and nobody types into it. The card is
// the key's among the cards already read (the routing backend's key windows
// among them), matched by name, drawn as an account row's is, its refresh
// reading this card alone: opening the editor never asks a vendor (#1518),
// and two keys of one name take their cards in the keys' order, as
// withKeyReadings claims them.
window.keyUsageMod = (() => {
  let keyUsage = { json: "", keys: [] }, keyUsageAt = 0;
  // the key rows live in the editor alone; not while someone types into it
  const redraw = () => {
    if (document.hidden || editing === null || typingInEditor()) return;
    renderProviders(true);
  };
  // the key's group by fingerprint, else by name at the same provider (a
  // key replaced at the vendor keeps its name; its past calls stay under
  // the old fingerprint, by that name)
  const group = (p, k) => keyUsage.keys.find((g) => g.id === p.id + "#" + k.id)
    || (k.name ? keyUsage.keys.find((g) => g.provider === p.id && g.providerKeyName === k.name) : null) || null;
  // the key's day as the Usage page's Today counts it; none when the key
  // made no calls today
  const line = (g) => {
    const out = el("div", "aq key-usage"), cost = fmtCost(g);
    let say = t("Today {n} calls", { n: g.calls });
    if (cost) say += " · " + cost;
    if (g.input || g.output) say += " · " + t("{a} in · {b} out", { a: fmtN(g.input), b: fmtN(g.output) });
    const s = el("span", "", say);
    s.title = t("What this key was asked today, as the Usage page's Today counts it");
    out.append(s);
    return out;
  };
  const card = (p, k, claim) => {
    if (!quotas) return null;
    const who = (k.name || k.masked).toLowerCase();
    const q = quotas.find((c) => c.provider === p.id && (c.user || "").toLowerCase() === who && !claim.has(usageAcctKey(c)));
    if (!q) return null;
    claim.add(usageAcctKey(q));
    const aq = accountQuota({ [k.name || k.masked]: q }, k.name || k.masked);
    aq.title ||= t("This key's allowance, as magpie has read it; refresh asks for this card alone");
    aq.append(quotaRefresh(q, redraw));
    return aq;
  };
  return {
    read() {
      if (Date.now() - keyUsageAt < 3e4) return;
      keyUsageAt = Date.now();
      api("usage?period=today" + tz()).then((r) => {
        const keys = r?.providerKeys || [], json = JSON.stringify(keys);
        if (json === keyUsage.json) return;
        keyUsage = { json, keys };
        redraw();
      }).catch(() => { keyUsageAt = 0; });
    },
    rows(p, k, claim) {
      const g = group(p, k);
      return [g, g ? line(g) : null, card(p, k, claim)];
    },
  };
})();
