// The sourcebook reader page (templates/sourcebook_view.html). The PDF.js
// viewer announces itself to its embedding page with a "webviewerloaded"
// event on this document, before it reads any of its options — so the
// listener has to exist before the iframe starts loading, which is why the
// iframe carries its URL in data-src instead of src.
(function () {
  const frame = document.getElementById("sourcebook-frame");
  if (!frame) return;

  document.addEventListener("webviewerloaded", (e) => {
    const opts = e.detail && e.detail.source && e.detail.source.PDFViewerApplicationOptions;
    if (!opts) return;
    // Saved viewer preferences would otherwise override everything below.
    opts.set("disablePreferences", true);
    opts.set("localeProperties", { lang: frame.dataset.locale });
    opts.set("enableScripting", false); // the scripting sandbox isn't vendored
    opts.set("viewerCssTheme", 2); // dark, matching the app
    // Drawing/highlighting tools off: nothing they make is ever saved.
    opts.set("annotationEditorMode", -1);
  });

  // Fill the window below the page header. Measured rather than a fixed
  // calc() because the nav wraps to more rows on narrow windows.
  const bottomGap = 8;
  function fit() {
    const top = frame.getBoundingClientRect().top + window.scrollY;
    frame.style.height = Math.max(240, window.innerHeight - top - bottomGap) + "px";
  }
  fit();
  window.addEventListener("resize", fit);

  frame.src = frame.dataset.src;
})();
