// Sitewide "back to top" button — layout.html (right after the footer)
// and layout_bare.html (the companion/Class Reference/Clan Reference popup
// layout) each declare their own #back-to-top button, both wired up by this
// one script. Several pages here get long enough that getting back to the
// top means several seconds of scrolling — the Classes page with two or
// three subclasses selected at once, a character sheet tab with a lot of
// features, a big Feats/Items list, the Class Reference popup's own full
// class+subclass catalog. Rather than deciding page-by-page which ones
// qualify, this just fades in past a scroll threshold on every page — a
// short page never crosses the threshold, so it never appears there at
// all.
(function () {
  const button = document.getElementById("back-to-top");
  if (!button) return;

  const SHOW_AFTER_PX = 600;

  function updateVisibility() {
    button.classList.toggle("back-to-top-visible", window.scrollY > SHOW_AFTER_PX);
  }
  window.addEventListener("scroll", updateVisibility, { passive: true });
  updateVisibility();

  button.addEventListener("click", () => {
    window.scrollTo({ top: 0, behavior: "smooth" });
  });
})();
