// The heavy built-in pages, declared before their scripts (kernel.js's
// magpie.lazy): each is in the window from the start, and its script and
// markup come the first time the reader opens it. One declared here must be
// self-contained: nothing else may call what its script defines. The scripts
// are named in index.html's lazyPages template, so the server asks for them
// by their content's hash (api.go, versionedPage).
//
// Routing is not here: its script hands the tray panel, Usage's Requests and
// the context card what they draw with (panelRoutingShown, kindName,
// swapWhy, routedWhy, purposeOptions, openRoute), so it loads with the
// window.
const lazyScript = (key) => [...document.getElementById("lazyPages").content.querySelectorAll(`script[data-page="${key}"]`)].map((s) => s.getAttribute("src"));
magpie.lazy("sessions", { plugin: "sessions", title: "Sessions", scripts: lazyScript("sessions") });
magpie.lazy("library", { plugin: "library", title: "Library", scripts: lazyScript("library") });
