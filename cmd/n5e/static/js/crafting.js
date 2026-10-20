// Seal Crafting editor (crafting_editor.html): every change posts the form
// to its data-preview-url, and each [data-crafting-live] element in the
// answer replaces the page's element with the same id -- the seal picker
// and the results panel. Focus and each rank group's open/closed state
// carry over the swap, and an answer to an older request is dropped.
// Delegated from document, since the swapped elements hold inputs of their
// own.
(function () {
  let seq = 0;
  let timer = null;

  function editorForm(el) {
    return el instanceof Element ? el.closest("form#crafting-form") : null;
  }

  function refresh(form) {
    const mine = ++seq;
    fetch(form.dataset.previewUrl, {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded", "X-Requested-With": "fetch" },
      body: new URLSearchParams(new FormData(form)).toString(),
    })
      .then((r) => (r.ok ? r.text() : Promise.reject(new Error("request failed (" + r.status + ")"))))
      .then((html) => {
        if (mine !== seq) return;
        const active = document.activeElement;
        const focusID = active && form.contains(active) ? active.id : "";
        const groups = new Map([...form.querySelectorAll("details[id]")].map((d) => [d.id, d.open]));

        const doc = new DOMParser().parseFromString(html, "text/html");
        doc.querySelectorAll("[data-crafting-live][id]").forEach((fresh) => {
          const current = document.getElementById(fresh.id);
          if (current) current.replaceWith(document.importNode(fresh, true));
        });

        groups.forEach((open, id) => {
          const d = document.getElementById(id);
          if (d) d.open = open;
        });
        if (focusID) {
          const el = document.getElementById(focusID);
          if (el && el !== document.activeElement) el.focus({ preventScroll: true });
        }
      })
      .catch((err) => console.warn("seal crafting preview failed:", err));
  }

  document.addEventListener("change", (e) => {
    const form = editorForm(e.target);
    if (!form) return;
    clearTimeout(timer);
    timer = setTimeout(() => refresh(form), 120);
  });

  // Enter in a number or text field previews instead of saving; Save is
  // its own button.
  document.addEventListener("keydown", (e) => {
    if (e.key !== "Enter" || !(e.target instanceof HTMLInputElement)) return;
    const form = editorForm(e.target);
    if (!form) return;
    e.preventDefault();
    e.target.dispatchEvent(new Event("change", { bubbles: true }));
  });
})();
