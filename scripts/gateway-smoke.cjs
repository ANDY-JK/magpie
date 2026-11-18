// A live gateway smoke run: one minimal request on each API magpie serves
// to clients, against a running magpie, so a release or a proxy change is
// checked end to end rather than by the unit tests alone (N-05, the review).
//
//   MAGPIE_URL=http://127.0.0.1:3425 node scripts/gateway-smoke.cjs
//   MAGPIE_URL=... MAGPIE_KEY=sk-magpie-... MAGPIE_MODEL=relay/m1 node scripts/gateway-smoke.cjs
//
// MAGPIE_KEY is a gateway key; the default is the local token "magpie",
// which only a gateway not shared on the network takes. MAGPIE_MODEL is the
// model every request asks; with none, the first model /v1/models lists is
// used. The script talks to the gateway as an agent does and changes
// nothing about it; it exits 1 if any API fails, and names each failure.
const base = (process.env.MAGPIE_URL || "http://127.0.0.1:3425").replace(/\/+$/, "");
const key = process.env.MAGPIE_KEY || "magpie";
const timeoutMs = Number(process.env.MAGPIE_SMOKE_TIMEOUT_MS || 60000);

const ask = "Reply with the single word: pong";
const bodies = {
  chat: (model) => ({
    path: "/v1/chat/completions",
    headers: { "Content-Type": "application/json", Authorization: `Bearer ${key}` },
    body: { model, messages: [{ role: "user", content: ask }], max_tokens: 8, stream: false },
  }),
  responses: (model) => ({
    path: "/v1/responses",
    headers: { "Content-Type": "application/json", Authorization: `Bearer ${key}` },
    body: { model, input: [{ role: "user", content: [{ type: "input_text", text: ask }] }], max_output_tokens: 8, stream: false },
  }),
  anthropic: (model) => ({
    path: "/v1/messages",
    headers: { "Content-Type": "application/json", "x-api-key": key, "anthropic-version": "2023-06-01" },
    body: { model, max_tokens: 8, messages: [{ role: "user", content: ask }] },
  }),
  gemini: (model) => ({
    path: `/v1beta/models/${encodeURIComponent(model)}:generateContent`,
    headers: { "Content-Type": "application/json", "x-goog-api-key": key },
    body: { contents: [{ role: "user", parts: [{ text: ask }] }] },
  }),
  ollama: (model) => ({
    path: "/api/chat",
    headers: { "Content-Type": "application/json", Authorization: `Bearer ${key}` },
    body: { model, messages: [{ role: "user", content: ask }], stream: false },
  }),
};

async function call(path, headers, body, method = "POST") {
  const ctl = new AbortController();
  const t = setTimeout(() => ctl.abort(), timeoutMs);
  try {
    const res = await fetch(base + path, {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
      signal: ctl.signal,
    });
    const text = await res.text();
    return { status: res.status, ok: res.ok, text };
  } finally {
    clearTimeout(t);
  }
}

function firstModel(listing) {
  let parsed;
  try {
    parsed = JSON.parse(listing.text);
  } catch {
    return "";
  }
  const models = parsed?.data || parsed?.models || [];
  for (const m of models) {
    const id = typeof m === "string" ? m : m?.id;
    if (id) return String(id);
  }
  return "";
}

(async () => {
  const results = [];
  const record = (name, ok, detail) => {
    results.push({ name, ok, detail });
    console.log(`${ok ? "ok  " : "FAIL"}  ${name.padEnd(10)} ${detail}`);
  };

  // the gateway is magpie, and can serve at all
  try {
    const hello = await call("/api/hello", {}, undefined, "GET");
    record("hello", hello.ok, `${hello.status} ${hello.text.slice(0, 80)}`);
    const ready = await call("/readyz", {}, undefined, "GET");
    record("readyz", ready.ok, `${ready.status} ${ready.text.slice(0, 80)}`);
  } catch (e) {
    record("hello", false, String(e));
  }

  let model = process.env.MAGPIE_MODEL || "";
  if (!model) {
    try {
      const listing = await call("/v1/models", { Authorization: `Bearer ${key}` }, undefined, "GET");
      model = firstModel(listing);
      if (!model) record("models", false, `${listing.status} ${listing.text.slice(0, 120)}`);
    } catch (e) {
      record("models", false, String(e));
    }
  }
  if (!model) {
    console.error("no model: set MAGPIE_MODEL, or have a provider serve one");
    process.exit(1);
  }
  console.log(`model: ${model}\n`);

  for (const [name, build] of Object.entries(bodies)) {
    const { path, headers, body } = build(model);
    try {
      const r = await call(path, headers, body);
      // a reply that is an error inside a 200 is a failure too
      const said = r.text.slice(0, 120).replace(/\s+/g, " ");
      record(name, r.ok && !/"error"\s*:/.test(r.text.slice(0, 4000)), `${r.status} ${said}`);
    } catch (e) {
      record(name, false, String(e));
    }
  }

  const failed = results.filter((r) => !r.ok);
  console.log(`\n${results.length - failed.length}/${results.length} answered`);
  if (failed.length) {
    console.log("failed: " + failed.map((f) => f.name).join(", "));
    process.exit(1);
  }
})().catch((e) => {
  console.error(e);
  process.exit(1);
});
