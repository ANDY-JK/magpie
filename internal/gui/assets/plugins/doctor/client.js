// The Doctor page: who is resting, why, and what failed lately — and Try
// now for a key or account that is resting, as its row on the Providers
// page has. A page the Doctor plugin adds to the window (kernel.js), its
// words in i18n.js beside this file. Nothing here changes a provider by
// itself: Try now ends one rest (gateway/unrest), as a click on the
// Providers page does, and Open goes to the provider.
magpie.plugin("doctor", (ctx) => {
  let page = null, reading = null;
  ctx.slots.add("page", {
    key: "doctor",
    title: "Doctor",
    html: `
  <div class="dr-head">
    <div class="dr-title"><b id="drTitle"></b><span class="hint" id="drSub"></span></div>
    <span class="grow"></span>
    <span class="dr-read" id="drRead"></span>
    <button class="text action" id="drReload" type="button"></button>
  </div>
  <section class="dr-cards" id="drCards"></section>
  <div class="row-head"><span class="label" id="drRestHead"></span><span class="grow"></span><span class="dr-count" id="drRestCount"></span></div>
  <div class="list" id="drRests"></div>
  <p class="hint dr-none" id="drNoRests" hidden></p>
  <div class="row-head"><span class="label" id="drFailHead"></span><span class="grow"></span><span class="dr-count" id="drFailCount"></span></div>
  <div class="list" id="drFails"></div>
  <p class="hint dr-none" id="drNoFails" hidden></p>
  <p class="hint dr-note" id="drNote"></p>
  <div class="plugin-slot" data-slot="doctor" hidden></div>`,
    mount(main) {
      page = main;
      main.querySelector("#drTitle").textContent = t("Doctor");
      main.querySelector("#drSub").textContent = t("Who is resting, why, and what failed lately");
      main.querySelector("#drReload").textContent = t("Read again");
      main.querySelector("#drRestHead").textContent = t("Resting now");
      main.querySelector("#drFailHead").textContent = t("Failed lately");
      main.querySelector("#drNoRests").textContent = t("Nothing is resting now.");
      main.querySelector("#drNoFails").textContent = t("Nothing failed lately.");
      main.querySelector("#drNote").textContent = t("Resting is magpie passing a key or account over until its time. Try now asks it again at once; refused again, it rests again.");
      main.querySelector("#drReload").onclick = () => load();
    },
    open() { load(); },
    // the window came back: read again, quietly (an answer that hasn't
    // changed draws the same page)
    refresh() { if (page && !document.hidden) load(true); },
  });

  // restLines: every key and account of a provider the gateway is passing
  // over, with the rest it rests
  const restLines = (p) => {
    const out = [];
    for (const k of p.keyList || []) if (k.rest) out.push({ p, name: k.name || k.masked || k.id || t("Key"), kind: "key", rest: k.rest });
    for (const l of p.account?.logins || []) if (l.rest) out.push({ p, name: l.alias || l.user || t("account"), kind: "account", rest: l.rest });
    return out;
  };

  // failsOf: the failed requests of the ledger, by provider and why, the
  // most frequent first, each with its last one
  const failsOf = (rows) => {
    const by = new Map();
    for (const r of rows) {
      const name = r.providerName || r.provider || "?";
      const why = r.err_type || (r.status ? String(r.status) : t("Failed"));
      const at = name + "\u0000" + why;
      const g = by.get(at) || { name, why, n: 0, last: null, said: "" };
      g.n++;
      if (!g.last || new Date(r.t) > new Date(g.last.t)) { g.last = r; g.said = r.err || ""; }
      by.set(at, g);
    }
    return [...by.values()].sort((a, b) => b.n - a.n || a.name.localeCompare(b.name));
  };

  const card = (n, what) => {
    const c = el("div", "dr-card");
    c.append(el("b", "", String(n)), el("span", "", what));
    return c;
  };

  function draw(providers, rows) {
    const rest = providers.flatMap(restLines);
    const fails = failsOf(rows);
    const on = providers.filter((p) => !p.off);
    page.querySelector("#drCards").replaceChildren(
      card(on.length, t("providers")),
      card(rest.length, t("resting")),
      card(rows.length, t("failed lately")),
    );
    page.querySelector("#drRestCount").textContent = rest.length ? t("{n} keys and accounts", { n: rest.length }) : "";
    page.querySelector("#drFailCount").textContent = rows.length ? t("{n} requests", { n: rows.length }) : "";
    const none = page.querySelector("#drNoRests"), noFail = page.querySelector("#drNoFails");
    none.hidden = !!rest.length;
    noFail.hidden = !!fails.length;

    const list = page.querySelector("#drRests");
    list.replaceChildren();
    for (const x of rest) {
      const row = el("div", "row dr-rest" + (x.rest.why === "auth" || x.rest.why === "credit" ? " bad" : ""));
      const who = el("div", "who");
      who.append(el("div", "name", x.p.name), el("div", "sub", x.name + " · " + t(x.kind === "key" ? "Key" : "account")));
      const why = el("div", "dr-why", (x.rest.status ? x.rest.status + " · " : "") + keyRestWords(x.rest));
      const when = el("div", "dr-when", restBack(x.rest.until));
      const acts = el("div", "dr-acts");
      const go = el("button", "text action dr-try", t("Try now"));
      go.title = t("Ends the rest: the next request asks it again. Refused again, it rests again.");
      go.onclick = async () => {
        go.disabled = true;
        try { await api("gateway/unrest", { key: x.rest.key }); status(t("It is asked again now"), "ok"); await load(true); }
        catch (e) { statusErr(e); go.disabled = false; }
      };
      const open = el("button", "text action dr-open", t("Open provider"));
      open.onclick = () => openProvider(x.p.id);
      acts.append(go, open);
      row.append(who, why, when, acts);
      list.append(row);
    }

    const flist = page.querySelector("#drFails");
    flist.replaceChildren();
    for (const f of fails) {
      const row = el("div", "row dr-fail");
      const who = el("div", "who");
      who.append(el("div", "name", f.name), el("div", "sub", (f.last?.model ? f.last.model + " · " : "") + (f.said || "")));
      const why = el("div", "dr-why", f.why);
      const n = el("div", "dr-count", f.n + "×");
      const when = el("div", "dr-when", f.last ? new Date(f.last.t).toLocaleString(intlLang()) : "");
      const acts = el("div", "dr-acts");
      const open = el("button", "text action dr-open", t("Open provider"));
      open.onclick = () => openProvider(f.last?.provider || f.name);
      acts.append(open);
      row.append(who, why, n, when, acts);
      flist.append(row);
    }
  }

  async function load(quiet) {
    if (!page) return reading;
    if (reading) return reading;
    reading = (async () => {
      const say = page.querySelector("#drRead");
      if (!quiet) say.textContent = t("Reading…");
      try {
        const [pv, led] = await Promise.all([api("providers"), api("usage/requests?failed=1&limit=200")]);
        draw(pv.providers || [], led.rows || []);
        say.textContent = t("Read {time}", { time: new Date().toLocaleTimeString(intlLang()) });
      } catch (e) {
        say.textContent = e.message || String(e);
      } finally { reading = null; }
    })();
    return reading;
  }
});
