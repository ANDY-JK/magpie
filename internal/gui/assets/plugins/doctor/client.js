// The Doctor page: who is resting, why, and what failed lately — and what to
// do about it. A row for each resting key or account names it and its why,
// says what to do (an appeal, a top-up, a new key), and offers Try now (the
// same gateway/unrest a provider row's button posts) and Probe and recover,
// which asks the vendor first and lifts the rest only once it answers
// (gateway/probe). Try all ends every rest in turn; the page reads itself
// again every 45 seconds while it is in sight. A second list says which keys
// and accounts are signed out, paused or off, so every one's state is here.
// A page the Doctor plugin adds to the window (kernel.js), its words in
// i18n.js beside this file. Nothing here changes a provider by itself.
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
  <div class="row-head"><span class="label" id="drRestHead"></span><span class="grow"></span><button class="text action dr-all" id="drTryAll" type="button" hidden></button><span class="dr-count" id="drRestCount"></span></div>
  <div class="list" id="drRests"></div>
  <p class="hint dr-none" id="drNoRests" hidden></p>
  <div class="row-head" id="drIdleHeadRow"><span class="label" id="drIdleHead"></span><span class="grow"></span><span class="dr-count" id="drIdleCount"></span></div>
  <div class="list" id="drIdle"></div>
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
      main.querySelector("#drTryAll").textContent = t("Try all");
      main.querySelector("#drIdleHead").textContent = t("Not in use");
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

  const kindWords = (kind) => t(kind === "key" ? "Key" : kind === "account" ? "account" : "Provider");

  // restLines: every key and account of a provider the gateway is passing
  // over, with the rest it rests — and the provider itself, when its vendor
  // refused it whole (a firewall, its web page where an API should answer),
  // which no key's or account's own row carries
  const restLines = (p) => {
    const out = [];
    if (p.rest) out.push({ p, name: "", kind: "provider", rest: p.rest });
    for (const k of p.keyList || []) if (k.rest) out.push({ p, name: k.name || k.masked || k.id || t("Key"), kind: "key", rest: k.rest });
    for (const l of p.account?.logins || []) if (l.rest) out.push({ p, name: l.alias || l.user || t("account"), kind: "account", rest: l.rest });
    // one model of an account rests alone, the account's others still
    // answering (#1960): said by accountRests, not by a login's own rest
    const said = new Set(out.map((x) => x.rest.key));
    for (const [user, rests] of Object.entries(p.accountRests || {})) {
      const l = (p.account?.logins || []).find((x) => String(x.user).toLowerCase() === user);
      for (const r of rests) {
        if (said.has(r.key)) continue;
        said.add(r.key);
        out.push({ p, name: l?.alias || l?.user || user, kind: "account", rest: r });
      }
    }
    return out;
  };

  // idleLines: the keys and accounts that take no request for another
  // reason than a rest — the account's sign-in lapsed, the account paused,
  // the key or provider switched off — so every one's state is on the page
  const idleLines = (providers) => {
    const out = [];
    for (const p of providers) {
      if (p.off) out.push({ p, name: "", kind: "provider", state: "turned off" });
      for (const k of p.keyList || []) {
        if (!k.on && !k.rest) out.push({ p, name: k.name || k.masked || k.id || t("Key"), kind: "key", state: "turned off" });
      }
      for (const l of p.account?.logins || []) {
        if (l.rest) continue;
        const name = l.alias || l.user || t("account");
        if (l.lapsed) out.push({ p, name, kind: "account", state: "Signed out" });
        else if (l.paused) out.push({ p, name, kind: "account", state: "Paused" });
        else if (!l.on && !l.active) out.push({ p, name, kind: "account", state: "turned off" });
      }
    }
    return out;
  };

  // failsOf: the failed requests of the ledger, by provider, why and the
  // key or account that answered them, the most frequent first, each with
  // its last one — so the page names which key failed, not only where
  const failsOf = (rows) => {
    const by = new Map();
    for (const r of rows) {
      const name = r.providerName || r.provider || "?";
      const why = r.err_type || (r.status ? String(r.status) : t("Failed"));
      const who = r.providerKeyName || r.providerAccount || "";
      const at = name + "\u0000" + why + "\u0000" + who;
      const g = by.get(at) || { name, why, who, n: 0, last: null, said: "" };
      g.n++;
      if (!g.last || new Date(r.t) > new Date(g.last.t)) { g.last = r; g.said = r.err || ""; }
      by.set(at, g);
    }
    return [...by.values()].sort((a, b) => b.n - a.n || a.name.localeCompare(b.name) || a.who.localeCompare(b.who));
  };

  // restBad: a rest a key or account doesn't come out of by waiting — its
  // sign-in refused, its credit gone, or its vendor refusing the key
  // itself (by "key"). A restricted or unverified account is not this: it
  // rests amber, with its word, as the Providers page marks it.
  const restBad = (r) => r.why === "auth" || r.why === "credit" || r.by === "key";

  // adviceFor: what to do about a rest, copied in short from the Routing
  // page's restHow (639-679 there) — a plugin's words are its own, so the
  // two don't import each other. A rate limit or an outage says nothing:
  // it lifts by itself.
  const adviceFor = (r) => {
    switch (r.by === "key" ? "key" : r.why) {
      case "restricted": return t("Appeal with the vendor, then try it again here");
      case "verify": return t("Verify the account with the vendor, then try it again");
      case "credit": case "quota": return t("Top up the account, then try it again here");
      case "auth": return t("Sign in again, then try it again here");
      case "key": return t("Replace the key, then try it again here");
      default: return "";
    }
  };

  // restName: the why a rest is said with — its status, its word, and the
  // one model it is for when it is only one model's
  const restName = (r) => (r.status ? r.status + " · " : "") + keyRestWords(r) + (r.model ? " · " + r.model : "");

  const card = (n, what) => {
    const c = el("div", "dr-card");
    c.append(el("b", "", String(n)), el("span", "", what));
    return c;
  };

  const idleCount = (lines) => ["Signed out", "Paused", "turned off"]
    .map((w) => [t(w), lines.filter((x) => x.state === w).length])
    .filter(([, n]) => n)
    .map(([w, n]) => w + " " + n)
    .join(" · ");

  function draw(providers, rows) {
    const rest = providers.flatMap(restLines);
    const idle = idleLines(providers);
    const fails = failsOf(rows);
    const on = providers.filter((p) => !p.off);
    page.querySelector("#drCards").replaceChildren(
      card(on.length, t("providers")),
      card(rest.length, t("resting")),
      card(rows.length, t("failed lately")),
      card(idle.length, t("not taking requests")),
    );
    page.querySelector("#drRestCount").textContent = rest.length ? t("{n} keys and accounts", { n: rest.length }) : "";
    page.querySelector("#drIdleCount").textContent = idle.length ? idleCount(idle) : "";
    page.querySelector("#drFailCount").textContent = rows.length ? t("{n} requests", { n: rows.length }) : "";
    page.querySelector("#drIdleHeadRow").hidden = !idle.length;
    page.querySelector("#drIdle").hidden = !idle.length;
    const none = page.querySelector("#drNoRests"), noFail = page.querySelector("#drNoFails");
    none.hidden = !!rest.length;
    noFail.hidden = !!fails.length;

    // Try all: every rest ended in turn, a fifth of a second apart. Each
    // one is the Try now a row has; asking the vendor for all of them at
    // once is what this is not (that's a probe, and a burst at a vendor).
    const all = page.querySelector("#drTryAll");
    all.hidden = !rest.length;
    all.disabled = false;
    all.onclick = async () => {
      all.disabled = true;
      try {
        for (let i = 0; i < rest.length; i++) {
          if (i) await new Promise((go) => setTimeout(go, 200));
          await api("gateway/unrest", { key: rest[i].rest.key });
        }
        status(t("Every rest was tried"), "ok");
        await fresh();
      } catch (e) { statusErr(e); all.disabled = false; }
    };

    const list = page.querySelector("#drRests");
    list.replaceChildren();
    for (const x of rest) {
      const row = el("div", "row dr-rest" + (restBad(x.rest) ? " bad" : ""));
      const who = el("div", "who");
      who.append(el("div", "name", x.p.name), el("div", "sub", x.kind === "provider" ? kindWords(x.kind) : x.name + " · " + kindWords(x.kind)));
      const advice = adviceFor(x.rest);
      if (advice) who.append(el("div", "hint dr-advice", advice));
      const why = el("div", "dr-why", restName(x.rest));
      if (x.rest.error) why.title = x.rest.error;
      const when = el("div", "dr-when", restBack(x.rest.until));
      const acts = el("div", "dr-acts");
      const go = el("button", "text action dr-try", t("Try now"));
      go.title = t("Ends the rest: the next request asks it again. Refused again, it rests again.");
      go.onclick = async () => {
        go.disabled = true;
        try { await api("gateway/unrest", { key: x.rest.key }); status(t("It is asked again now"), "ok"); await fresh(); }
        catch (e) { statusErr(e); go.disabled = false; }
      };
      // Probe and recover: the vendor is asked first (its model list, which
      // costs no allowance), and only an answer lifts the rest. One rest at
      // a time, never all of them: a burst is the vendor's to refuse.
      const probe = el("button", "text action dr-probe", t("Probe and recover"));
      probe.onclick = async () => {
        probe.disabled = true;
        try {
          const a = await api("gateway/probe", { key: x.rest.key });
          if (a.lifted) status(t("It answered: the rest is lifted"), "ok");
          else status(t("Its rest stands"));
          await fresh();
        } catch (e) { statusErr(e); probe.disabled = false; }
      };
      const open = el("button", "text action dr-open", t("Open provider"));
      open.onclick = () => openProvider(x.p.id);
      acts.append(go, probe, open);
      row.append(who, why, when, acts);
      list.append(row);
    }

    const ilist = page.querySelector("#drIdle");
    ilist.replaceChildren();
    for (const x of idle) {
      const row = el("div", "row dr-rest dr-idle");
      const who = el("div", "who");
      who.append(el("div", "name", x.p.name), el("div", "sub", x.kind === "provider" ? kindWords(x.kind) : x.name + " · " + kindWords(x.kind)));
      const state = el("div", "dr-why", t(x.state));
      const acts = el("div", "dr-acts");
      const open = el("button", "text action dr-open", t("Open provider"));
      open.onclick = () => openProvider(x.p.id);
      acts.append(open);
      row.append(who, state, acts);
      ilist.append(row);
    }

    const flist = page.querySelector("#drFails");
    flist.replaceChildren();
    for (const f of fails) {
      const row = el("div", "row dr-fail");
      const who = el("div", "who");
      const said = [f.last?.model, f.who ? kindWords(f.last?.providerKeyName ? "key" : "account") + " " + f.who : "", f.said]
        .filter(Boolean).join(" · ");
      who.append(el("div", "name", f.name), el("div", "sub", said));
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

  // fresh: read again after any read already under way is done — a click's
  // own answer (a lifted rest) isn't left to a read that started before it
  async function fresh() {
    if (reading) await reading;
    return load(true);
  }

  // The page reads itself again while it is the one in sight: every 45
  // seconds quietly, and when the window comes back (a window hidden for a
  // while has kept what it read then). Loaded once, at the plugin's setup,
  // so a mount again after the error boundary's Try again adds no second.
  const beat = setInterval(() => { if (page && !page.hidden && !document.hidden) load(true); }, 45000);
  ctx.onDispose(() => clearInterval(beat));
  document.addEventListener("visibilitychange", () => { if (!document.hidden && page && !page.hidden) load(true); });
});