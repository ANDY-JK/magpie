// The heavy built-in pages, declared before their scripts (kernel.js's
// magpie.lazy): each is in the window from the start, and its script and
// markup come the first time the reader opens it. One declared here must be
// self-contained, or the part the window needs before then a small eager
// file of its own: Routing's is plugins/routing/shared.js (the words its
// rows say of a request, the Usage page's Requests and the context card
// say too, and the calls that bring the page in). The scripts are named in
// index.html's lazyPages template, so the server asks for them by their
// content's hash (api.go, versionedPage).
const lazyScript = (key) => [...document.getElementById("lazyPages").content.querySelectorAll(`script[data-page="${key}"]`)].map((s) => s.getAttribute("src"));
magpie.lazy("sessions", { plugin: "sessions", title: "Sessions", scripts: lazyScript("sessions") });
magpie.lazy("library", { plugin: "library", title: "Library", scripts: lazyScript("library") });
magpie.lazy("routing", { plugin: "routing", title: "Routing", scripts: lazyScript("routing") });
