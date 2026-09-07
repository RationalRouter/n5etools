// Sitewide language switcher (layout.html's #lang-switcher, right under the
// nav on every page). The base markup is a plain, always-expanded row of
// language forms with no JS required — this script is purely a progressive
// enhancement that collapses it down to a single 🌐 button and expands it
// back into the rounded option box on click, so a language pick still works
// with JS disabled, just without the collapse animation. Selecting a
// language submits a normal form POST to /settings/locale (see
// handleSetLocale), which sets the cookie and redirects back to the current
// page — the resulting full page load is what "instantly translates" the
// page, not anything this script does.
(function () {
  // Picking a language is a real POST/redirect/GET (see handleSetLocale's
  // doc comment) that lands back on the same page, not a fragment swap or a
  // JS reload() — so unlike sheet-inventory.js's reload-scroll-restore
  // (which browsers handle for free), nothing here preserves scroll
  // position across the navigation on its own. Same sessionStorage
  // save-before-navigate/restore-on-load pattern as sheet-tabs.js's tab
  // memory: sessionStorage survives a same-tab cross-page POST navigation
  // fine (this is not the localStorage-per-port pitfall in CLAUDE.md, which
  // is specifically about surviving an app *restart*).
  const SCROLL_KEY = "n5e:lang-switch-scroll";
  const savedScroll = sessionStorage.getItem(SCROLL_KEY);
  if (savedScroll !== null) {
    sessionStorage.removeItem(SCROLL_KEY);
    window.scrollTo(0, parseInt(savedScroll, 10) || 0);
  }

  const switcher = document.getElementById("lang-switcher");
  const toggle = document.getElementById("lang-toggle");
  const panel = document.getElementById("lang-panel");
  if (!switcher || !toggle || !panel) return;

  for (const form of panel.querySelectorAll("form")) {
    form.addEventListener("submit", () => {
      sessionStorage.setItem(SCROLL_KEY, String(window.scrollY));
    });
  }

  panel.classList.add("lang-panel-collapsible");

  function setOpen(open) {
    panel.classList.toggle("lang-panel-open", open);
    toggle.setAttribute("aria-expanded", open ? "true" : "false");
  }

  toggle.addEventListener("click", () => {
    setOpen(!panel.classList.contains("lang-panel-open"));
  });

  document.addEventListener("click", (e) => {
    if (!panel.classList.contains("lang-panel-open")) return;
    if (switcher.contains(e.target)) return;
    setOpen(false);
  });

  document.addEventListener("keydown", (e) => {
    if (e.key !== "Escape" || !panel.classList.contains("lang-panel-open")) return;
    setOpen(false);
    toggle.focus();
  });
})();
