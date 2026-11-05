// The window drawn again in a language picked after it opened: i18n.js's
// setLocale holds the new language back until its words are here
// (loadLocaleWords), and imports this then. It draws what a load that comes
// in another language draws (app.js's relang), from the words that are in by
// now, so those pages read in the language picked, not the one they were
// drawn in.
window.relangWords = () => {
  if (!load.done) return; // the first read draws the page in its language
  if (mode === "window") { if (!sideNav()) slide($("#nav"), "nav"); navSubs(); renderSideGw(); renderSideToggle(); }
  if (view === "plugins") window.loadPlugins?.();
  magpie.call(view, "relang");
  if (prefs) renderSettings();
  renderAgents();
  providers = null;
  usage = null;
};
