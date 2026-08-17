// Re-fetches live blocks of the character sheet that a mutation changed but
// whose reply could not contain.
//
// Why this exists: the sheet's endpoints each answer with ONE re-rendered
// fragment, which works as long as every action changes exactly one block.
// A rest broke that. It posts to /sheet/rest and gets "sheet_vitals" back
// (HP, chakra, the rest forms), but it also spends or restores hit dice —
// and the hit-dice counter now lives in the square row at the top of the
// LEFT column, a different element in a different column that one response
// body cannot include. Rather than have every endpoint learn to return
// several fragments at once, the form names the extra blocks it affects:
//
//   <form data-target="sheet-vitals" data-also-refresh="sheet-squares">
//
// and this fetches each of those from the sheet's read-only fragment
// endpoint after the primary swap lands.
//
// Each refreshable block carries its own template name in data-fragment
// (see character_sheet.html's {{define "sheet_squares"}}), so the element id
// in data-also-refresh is all a caller needs to know — the mapping from id
// to template lives on the element itself and can't drift out of sync in a
// second place here.
(function () {
  // The chat panel is the one element on the sheet that already carries the
  // character id, and it is present on every character sheet render.
  function characterID() {
    const panel = document.querySelector("[data-character-id]");
    return panel ? panel.dataset.characterId : null;
  }

  // Space-separated element ids -> refreshed in place. Silent no-op when the
  // attribute is absent, the page isn't a character sheet, or a named
  // element isn't on this page: a cosmetic refresh must never be able to
  // break the action that triggered it.
  window.n5eRefreshBlocks = function (list) {
    if (!list) return;
    const id = characterID();
    if (!id) return;

    list.trim().split(/\s+/).forEach((elementID) => {
      const el = document.getElementById(elementID);
      if (!el) return;
      const fragment = el.dataset.fragment;
      if (!fragment) return;

      fetch("/characters/" + encodeURIComponent(id) + "/sheet/fragment/" + encodeURIComponent(fragment), {
        headers: { "X-Requested-With": "fetch" },
      })
        .then((r) => {
          if (!r.ok) throw new Error("server rejected the refresh (" + r.status + ")");
          return r.text();
        })
        .then((html) => {
          // Re-read the element rather than reusing `el`: the primary swap
          // may have replaced it between the fetch starting and finishing.
          const current = document.getElementById(elementID);
          if (current) current.outerHTML = html;
          // sheet-squares carries its own .sheet-edit-box (Speed) — without
          // this, Speed's listener dies the moment any rest refreshes this
          // block and never comes back until a full page reload.
          if (window.n5eRewireControls) window.n5eRewireControls();
        })
        .catch((err) => console.warn("refreshing " + elementID + " failed:", err));
    });
  };
})();
