// Learning/forgetting a jutsu used to fall into sheet-inventory.js's
// generic reload-based handling, same as every other library-pane action —
// but that specifically "flashes" the whole page, disruptively, on every
// add. A full reload is genuinely the right call for
// most of sheet-inventory.js's forms (equipping armor moves AC, equipping a
// weapon adds an attack row — effects scattered across the sheet that no
// one fragment could contain), but jutsu is different: learning or
// forgetting one only ever changes two known, bounded regions — the Known
// Jutsu list (Jutsu tab, "sheet_jutsu_known") and the Attacks & Jutsu
// table's Jutsu section (Core tab, "sheet_attack_jutsu_table") — so those
// two get swapped in place instead.
//
// Marked by the sheet-jutsu-fetch-form class (carried ALONGSIDE
// sheet-inventory-form/sheet-library-form for their shared CSS — see
// sheet-inventory.js's own exclusion check for that class).
(function () {
  // Toggling the "already known" indicator on the clicked library row
  // directly, rather than waiting for a server round trip, needs no fetch
  // of its own — the row is already in the DOM, and the class this reads
  // (sheet-lib-row-owned) is the exact one character_sheet.html's server
  // render sets from OwnedJutsu at initial page load.
  function updateLibraryRowOwned(slug, owned) {
    const row = document.querySelector('.sheet-lib-row[data-slug="' + CSS.escape(slug) + '"]');
    if (!row) return;
    row.classList.toggle("sheet-lib-row-owned", owned);
    const btn = row.querySelector(".sheet-lib-add");
    const nameEl = row.querySelector(".browse-row-name");
    const name = nameEl ? nameEl.textContent : "this jutsu";
    if (btn) {
      btn.title = owned ? "Already known — adding again does nothing" : "Learn " + name;
      btn.setAttribute("aria-label", "Learn " + name);
    }
  }

  document.addEventListener("submit", (e) => {
    const form = e.target;
    if (!(form instanceof HTMLFormElement)) return;
    if (!form.classList.contains("sheet-jutsu-fetch-form")) return;
    e.preventDefault();

    const body = new URLSearchParams(new FormData(form));
    // Same reasoning as sheet-inventory.js's identical line: the add form
    // is one <form> with hundreds of per-row submit buttons, and FormData
    // never includes the button that actually submitted it.
    if (e.submitter && e.submitter.name) body.append(e.submitter.name, e.submitter.value);
    const slug = body.get("slug");
    const learning = !form.action.endsWith("/delete");

    fetch(form.action, {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded", "X-Requested-With": "fetch" },
      body: body.toString(),
    })
      .then((r) => {
        if (!r.ok) {
          return r.text().then((text) => {
            throw new Error(text || "request failed (" + r.status + ")");
          });
        }
        return r.text();
      })
      .then((html) => {
        const targetId = form.dataset.target;
        const target = targetId && document.getElementById(targetId);
        if (target) target.outerHTML = html;
        if (window.n5eRewireControls) window.n5eRewireControls();
        if (window.n5eRefreshBlocks) window.n5eRefreshBlocks(form.dataset.alsoRefresh);
        if (slug) updateLibraryRowOwned(slug, learning);
      })
      .catch((err) => {
        console.warn("jutsu update failed:", err);
        window.alert("Could not save that change: " + err.message);
      });
  });
})();
