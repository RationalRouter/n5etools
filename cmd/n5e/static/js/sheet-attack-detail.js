// Click-to-detail-card for companion attack names: any
// ".companion-attack-name" that carries a description (companion_fields.html
// wraps it in .tooltip for a hover preview, and its parent [data-detail-name]
// row carries a full <template class="detail-body"> sibling) also opens a
// small modal with the same text on click, everywhere that markup renders —
// the Companions tab (every non-puppet companion kind) and the standalone
// companion popup page alike.
//
// The Puppets tab is deliberately excluded: it already has a persistent,
// always-visible right-hand panel for this exact data
// (sheet-puppet-detail.js, keyed off the same data-detail-name/detail-body
// markup), so opening a modal there too would fire both at once for the
// same click. sheet-puppet-detail.js's own listeners are separately gated to
// do nothing off that tab, so the reverse case (this file's dialog assuming
// puppet-detail.js's job elsewhere) can't happen.
//
// Delegated from document, not bound per-row, so it survives any
// outerHTML swap of the containing fragment without a rewire call — same
// reasoning as every other sheet-wide delegated listener (see
// sheet-puppet-detail.js's own header comment).
(function () {
  let dialog = null;

  function ensureDialog() {
    if (dialog) return dialog;
    dialog = document.createElement("dialog");
    dialog.className = "companion-attack-detail-dialog";
    dialog.innerHTML =
      '<form method="dialog"><h3></h3><div class="companion-attack-detail-body"></div>' +
      '<button type="submit" class="companion-attack-detail-close">Close</button></form>';
    document.body.appendChild(dialog);
    return dialog;
  }

  function open(name, bodyHTML) {
    const d = ensureDialog();
    d.querySelector("h3").textContent = name;
    d.querySelector(".companion-attack-detail-body").innerHTML = bodyHTML;
    d.showModal();
  }

  document.addEventListener("click", (e) => {
    if (e.target.closest("form")) return;
    const nameEl = e.target.closest(".companion-attack-name");
    if (!nameEl) return;
    if (nameEl.closest("#sheet-puppet-tab")) return; // owned by sheet-puppet-detail.js there
    const row = nameEl.closest("[data-detail-name]");
    const template = row ? row.querySelector(":scope > template.detail-body") : null;
    const bodyHTML = template ? template.innerHTML : "";
    if (!bodyHTML) return;
    open(row.dataset.detailName, bodyHTML);
  });
})();
