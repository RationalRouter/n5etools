// Character sheet item/jutsu click popup: clicking an inventory or known-
// jutsu row opens its existing /items/{slug} or /jutsu/{slug} detail card
// in a modal instead of navigating away from the sheet. Reuses the exact
// ?fragment=1 fetch creation-picker.js's selectRow already established
// (items.go's handleItemDetail / jutsu.go's handleJutsuDetail both already
// serve just the card markup for this), just targeting a <dialog> instead
// of a fixed pane.
//
// Real <a href> is kept on every trigger (see character_sheet.html) so
// plain navigation still works with JS disabled — same progressive-
// enhancement precedent creation-picker.js's own doc comment states.
(function () {
  const dialog = document.getElementById("sheet-item-popup");
  const body = document.getElementById("sheet-popup-body");
  if (!dialog || !body) return;
  const closeBtn = dialog.querySelector(".sheet-popup-close");

  document.addEventListener("click", (e) => {
    const trigger = e.target.closest(".sheet-popup-trigger");
    if (!trigger) return;
    e.preventDefault();
    fetch(trigger.href + "?fragment=1")
      .then((r) => r.text())
      .then((html) => {
        body.innerHTML = html;
        dialog.showModal();
      })
      .catch((err) => console.warn("item/jutsu popup fetch failed:", err));
  });

  if (closeBtn) closeBtn.addEventListener("click", () => dialog.close());
  // Clicking the ::backdrop area delivers its click event to the <dialog>
  // element itself (not a descendant) — this is what distinguishes an
  // outside click from a click inside the popup body.
  dialog.addEventListener("click", (e) => {
    if (e.target === dialog) dialog.close();
  });
})();
