// The Metrics page: what the gateway and this process hold and spend right
// now. Three reads, asked of nowhere else in the window: what is in flight
// at the gateway and each of those requests (/api/busy), this process's
// memory, CPU and goroutines with the last four hours of them
// (/api/process), and calls by hour with their failures (/api/usage/hourly).
// What other pages already show — Usage's standings, the routing totals,
// who rests — it links to instead of drawing again. A page the Metrics
// plugin adds to the window (kernel.js), its words in i18n.js beside this
// file. Nothing here changes anything by itself: every read is a GET, and a
// read that fails keeps what was drawn, saying why where the numbers were.
magpie.plugin("metrics", (ctx) => {
  let page = null, reading = null, ticks = 0;
  // what was read last, drawn again on a language change and kept when a
  // later read fails: a failed read is unknown, never empty
  const kept = {};
  const errs = {};
  // where a request in flight stands (gateway/active.go), said by its phase
  const PHASES = { slot: "queued for a key or account", rpm: "waiting for the minute", upstream: "asking a vendor", redact: "masking its words" };

  ctx.slots.add("page", {
    key: "metrics",
    title: "Metrics",
    html: `
  <div class="row-head">
    <div class="who"><b id="mtTitle"></b><span class="hint" id="mtSub"></span></div>
    <span class="grow"></span>
    <span class="hint" id="mtRead"></span>
    <button class="text action" id="mtReload" type="button"></button>
  </div>
  <div class="row-head"><span class="label" id="mtLiveHead"></span><span class="grow"></span><span class="hint" id="mtLiveNote"></span></div>
  <div class="stats" id="mtLive"></div>
  <div class="list" id="mtActive" hidden></div>
  <p class="hint" id="mtPhase"></p>
  <div class="row-head"><span class="label" id="mtProcHead"></span><span class="grow"></span><span class="hint" id="mtProcMeta"></span></div>
  <div class="stats" id="mtProc"></div>
  <span class="label" id="mtMemHead"></span>
  <canvas id="mtMem" style="display:block;width:100%;max-width:860px;height:72px"></canvas>
  <span class="label" id="mtCpuHead"></span>
  <canvas id="mtCpu" style="display:block;width:100%;max-width:860px;height:72px"></canvas>
  <p class="hint" id="mtIdle"></p>
  <div class="row-head"><span class="label" id="mtHourHead"></span><span class="grow"></span><span class="hint" id="mtHourAvg"></span></div>
  <canvas id="mtHour" style="display:block;width:100%;max-width:860px;height:96px"></canvas>
  <p class="hint" id="mtHourLegend"></p>
  <p class="hint" id="mtHourNote"></p>
  <div class="row-head"><span class="label" id="mtMoreHead"></span></div>
  <div class="list" id="mtMore"></div>`,
    mount(main) {
      page = main;
      page.querySelector("#mtReload").onclick = () => load(false, true);
      say();
    },
    open() { load(false, true); },
    // the window came back: read again, quietly (an answer that hasn't
    // changed draws the same page)
    refresh() { if (page && !document.hidden) load(true); },
    relang() { say(); },
  });

  // say: the words that don't wait for a read, and the pages linked to —
  // mount, and a language picked after the page opened
  function say() {
    const q = (id) => page.querySelector(id);
    q("#mtTitle").textContent = t("Metrics");
    q("#mtSub").textContent = t("Requests in flight, this process's memory and CPU, and calls by hour");
    q("#mtReload").textContent = t("Read now");
    q("#mtLiveHead").textContent = t("Running now");
    q("#mtPhase").textContent = t("Waiting ones say where they wait: a key's queue, or room in the minute.");
    q("#mtProcHead").textContent = t("The process itself");
    q("#mtMemHead").textContent = t("Memory, the last four hours");
    q("#mtCpuHead").textContent = t("CPU, the last four hours");
    q("#mtIdle").textContent = t("Idle past half a minute, the gateway hands its memory back.");
    q("#mtHourHead").textContent = t("Calls by hour");
    q("#mtHourNote").textContent = t("Waiting requests aren't counted by hour yet.");
    q("#mtMoreHead").textContent = t("Shown elsewhere");
    const more = q("#mtMore");
    more.replaceChildren();
    for (const x of [
      { feature: "usage", name: "Usage", sub: "Which provider answered, and what it cost", go: standings },
      { feature: "routing", name: "Routing", sub: "Which route served what, and its totals", go: () => show("routing") },
      { feature: "doctor", name: "Doctor", sub: "Who rests, and what failed lately", go: () => show("doctor") },
    ]) {
      const row = el("div", "row");
      row.dataset.feature = x.feature;
      const who = el("div", "who");
      who.append(el("div", "name", t(x.name)), el("div", "sub", t(x.sub)));
      const open = el("button", "text action", t("Open"));
      open.onclick = x.go;
      row.append(who, open);
      more.append(row);
    }
    if (!reading) draw();
  }

  // the Usage page's standings: open the page, then pick its fifth tab — the
  // page's own strip, in the order the page keeps (overview, requests,
  // sessions, context, standings). Nothing of that page's words is said
  // here, so the pick is by the strip's order; a strip not there yet leaves
  // the page at the tab it had.
  async function standings() {
    if (!await show("usage")) return;
    for (let i = 0; i < 20; i++) {
      const b = document.querySelector("#usageTab")?.querySelectorAll("button")[4];
      if (b) { b.click(); return; }
      await new Promise((r) => setTimeout(r, 100));
    }
  }

  const cs = (name) => getComputedStyle(document.documentElement).getPropertyValue(name).trim() || "#888";
  const ms = (v) => (v >= 1000 ? (v / 1000).toFixed(1) + " s" : Math.round(v) + " ms");
  const round1 = (v) => (v >= 10 || Math.round(v) === v ? String(Math.round(v)) : v.toFixed(1));
  const kpi = (b, label) => { const c = el("div", "kpi"); c.append(el("b", "", b), el("span", "", label)); return c; };
  const swatch = (c) => {
    const i = el("i");
    i.style.cssText = "display:inline-block;width:8px;height:8px;border-radius:2px;background:" + c + ";margin-right:3px";
    return i;
  };

  // line: a small polyline chart, no library and no axes — one hairline at
  // the foot, one line per series, scaled to the tallest point anywhere
  function line(cv, list, h) {
    const w = cv.clientWidth;
    if (!w) return;
    const dpr = devicePixelRatio || 1;
    cv.width = Math.round(w * dpr);
    cv.height = Math.round(h * dpr);
    const g = cv.getContext("2d");
    g.scale(dpr, dpr);
    g.strokeStyle = cs("--line-2");
    g.lineWidth = 1;
    g.beginPath();
    g.moveTo(0, h - 4);
    g.lineTo(w, h - 4);
    g.stroke();
    const top = Math.max(1e-9, ...list.flatMap((s) => s.v));
    for (const s of list) {
      g.beginPath();
      s.v.forEach((v, i) => {
        const x = (i / Math.max(1, s.v.length - 1)) * (w - 4) + 2;
        const y = h - 4 - (v / top) * (h - 10);
        if (i) g.lineTo(x, y); else g.moveTo(x, y);
      });
      g.strokeStyle = s.c;
      g.lineWidth = 1.5;
      g.stroke();
    }
  }

  function draw() {
    if (!page) return;
    busyDraw();
    procDraw();
    hourDraw();
  }

  // busyDraw: what a restart would cut short, and each request in flight.
  // owner false when another magpie on this computer serves the gateway —
  // the endpoint then says nothing else, and an empty table must not read
  // as a quiet gateway.
  function busyDraw() {
    const live = page.querySelector("#mtLive"), act = page.querySelector("#mtActive"), note = page.querySelector("#mtLiveNote");
    live.classList.remove("empty");
    act.replaceChildren();
    act.hidden = true;
    note.textContent = "";
    const b = kept.busy;
    if (!b) {
      live.classList.add("empty");
      live.replaceChildren(el("div", "none", errs.busy ? String(errs.busy.message || errs.busy) : t("Reading…")));
      return;
    }
    if (b.owner === false) {
      const said = t("The gateway is served by another magpie on this computer.");
      live.classList.add("empty");
      live.replaceChildren(el("div", "none", said));
      return;
    }
    note.textContent = errs.busy ? String(errs.busy.message || errs.busy) : "";
    const active = b.active || [];
    live.replaceChildren(
      kpi(String(b.busy?.requests ?? 0), t("Requests")),
      kpi(String(b.busy?.tools ?? 0), t("Turns waiting on tools")),
    );
    for (const a of active) {
      const row = el("div", "row");
      const who = el("div", "who");
      who.append(el("div", "name", a.agent || "?"));
      const bits = [a.model, a.provider, PHASES[a.phase] ? t(PHASES[a.phase]) : a.phase, ms(a.ms || 0)].filter(Boolean);
      who.append(el("div", "sub", bits.join(" · ")));
      row.append(who);
      act.append(row);
    }
    act.hidden = !active.length;
  }

  // procDraw: what this process holds now, and the sampler's last four
  // hours of its memory and CPU (15 seconds a reading).
  function procDraw() {
    const box = page.querySelector("#mtProc"), meta = page.querySelector("#mtProcMeta");
    box.classList.remove("empty");
    meta.textContent = "";
    const st = kept.proc;
    if (!st) {
      box.classList.add("empty");
      box.replaceChildren(el("div", "none", errs.proc ? String(errs.proc.message || errs.proc) : t("Reading…")));
      line(page.querySelector("#mtMem"), [], 72);
      line(page.querySelector("#mtCpu"), [], 72);
      return;
    }
    const hist = st.history || [];
    const cpu = cpuSeries(hist, st.numCPU);
    meta.textContent = [st.program, st.pid ? "pid " + st.pid : ""].filter(Boolean).join(" · ");
    box.replaceChildren(
      kpi(fmtBytes(st.heapAlloc), t("live memory")),
      kpi(fmtBytes(st.sys), t("taken from the system")),
      kpi(fmtBytes(st.heapReleased), t("returned so far")),
      kpi(String(st.goroutines), t("Go routines")),
      kpi(cpu.now === null ? "—" : Math.round(cpu.now) + "%", t("CPU share")),
      kpi(st.uptime || "—", t("running since")),
      // K-02: usage records the ledger couldn't write, so a Usage total
      // short of the calls made is explained, not a mystery. None is none.
      ...(st.usageDrops ? [kpi(String(st.usageDrops), t("usage records not written"))] : []),
    );
    line(page.querySelector("#mtMem"), [{ v: hist.map((s) => s.heapAlloc / 1e6), c: cs("--accent") }], 72);
    line(page.querySelector("#mtCpu"), [{ v: cpu.series, c: cs("--accent") }], 72);
  }

  // cpuSeries: the share of the machine's cores the process used between two
  // readings — cpuTotal is seconds of CPU since the start, so a difference
  // over the time between readings, over the core count, is the share. The
  // first reading has nothing to differ from, so the series is one shorter;
  // a system that says no CPU (every reading 0) is a dash, not a flat zero.
  function cpuSeries(hist, cores) {
    const out = [];
    for (let i = 1; i < hist.length; i++) {
      const dt = (new Date(hist[i].t) - new Date(hist[i - 1].t)) / 1000;
      if (dt > 0 && hist[i].cpuTotal > 0) {
        out.push(Math.min(100, ((hist[i].cpuTotal - hist[i - 1].cpuTotal) / dt) / (cores || 1) * 100));
      } else out.push(0);
    }
    return { series: out, now: out.length && hist.some((s) => s.cpuTotal > 0) ? out[out.length - 1] : null };
  }

  // hourDraw: calls and failures by hour, with their averages over the
  // window. What a request waited in a queue is not counted by hour (the
  // counts have no column for it), so no curve of it is drawn.
  function hourDraw() {
    const avg = page.querySelector("#mtHourAvg"), legend = page.querySelector("#mtHourLegend");
    const h = kept.hour;
    if (!h) {
      avg.textContent = errs.hour ? String(errs.hour.message || errs.hour) : "";
      legend.replaceChildren();
      line(page.querySelector("#mtHour"), [], 96);
      return;
    }
    const pts = h.points || [];
    const calls = pts.map((p) => p.calls), failed = pts.map((p) => p.errors);
    const hours = Math.max(1, pts.length);
    const ac = calls.reduce((a, b) => a + b, 0) / hours, af = failed.reduce((a, b) => a + b, 0) / hours;
    avg.textContent = t("An average of {n} calls an hour, {m} failed", { n: round1(ac), m: round1(af) });
    legend.replaceChildren(
      swatch(cs("--accent")), document.createTextNode(" " + t("Calls an hour") + "   ·   "),
      swatch(cs("--red")), document.createTextNode(" " + t("Failed an hour")),
    );
    line(page.querySelector("#mtHour"), [{ v: calls, c: cs("--accent") }, { v: failed, c: cs("--red") }], 96);
  }

  async function load(quiet, hours) {
    if (!page) return reading;
    if (reading) return reading;
    reading = (async () => {
      const at = page.querySelector("#mtRead");
      if (!quiet) at.textContent = t("Reading…");
      const one = async (path) => { try { return { v: await api(path) }; } catch (e) { return { e }; } };
      const jobs = [one("busy"), one("process?history=1")];
      if (hours) jobs.push(one("usage/hourly"));
      const [b, p, h] = await Promise.all(jobs);
      kept.busy = b.v ?? kept.busy; errs.busy = b.e || null;
      kept.proc = p.v ?? kept.proc; errs.proc = p.e || null;
      if (h) { kept.hour = h.v ?? kept.hour; errs.hour = h.e || null; }
      draw();
      at.textContent = t("Read at {time}", { time: new Date().toLocaleTimeString(intlLang()) });
    })();
    return reading.finally(() => { reading = null; });
  }

  // The page reads itself again while it is the one in sight: the gateway
  // and the process every 15 seconds — the sampler's own step, so a CPU
  // difference is one reading long — and the hours once a minute. Loaded
  // once, at the plugin's setup, so a mount again after the error boundary's
  // Try again adds no second.
  const beat = setInterval(() => {
    if (!page || page.hidden || document.hidden) return;
    ticks++;
    load(true, ticks % 4 === 0);
  }, 15000);
  ctx.onDispose(() => clearInterval(beat));
  document.addEventListener("visibilitychange", () => { if (!document.hidden && page && !page.hidden) load(true); });
});
