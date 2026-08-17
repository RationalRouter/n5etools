// Free-text autosave, shared by the Bio tab's five textareas and the Core
// tab's Notes box. Every field is a .sheet-bio-field inside a real <form>;
// blurring one resubmits its whole form. Simple and idempotent, and a
// handful of short text columns is cheap enough that there's no real cost
// to sending all of a form's fields each time.
//
// Delegated from the document rather than bound to each field at load:
// several parts of the sheet are re-rendered as fragment swaps, and a
// listener attached to an element that gets replaced is silently inert
// afterwards. "focusout" rather than "blur" because blur does not bubble.
(function () {
  // Must be application/x-www-form-urlencoded, not a raw FormData body
  // (which fetch sends as multipart/form-data) — the Go handlers only
  // parse urlencoded bodies via r.ParseForm(), so a multipart body's
  // fields all silently read back as empty strings. That meant every
  // autosave here was successfully "saving" blank text over whatever the
  // player had actually typed — no error, no visible failure, just quiet
  // data loss on the very next navigation. See sheet-vitals.js's postForm
  // for the same fix applied to the other sheet-side fetch forms.
  function save(form) {
    fetch(form.action, {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded", "X-Requested-With": "fetch" },
      body: new URLSearchParams(new FormData(form)).toString(),
    }).then((r) => {
      if (!r.ok) console.warn("text autosave rejected:", r.status);
    }).catch((err) => {
      console.warn("text autosave failed:", err);
    });
  }

  document.addEventListener("focusout", (e) => {
    const field = e.target;
    if (!(field instanceof Element) || !field.classList.contains("sheet-bio-field")) return;
    if (field.form) save(field.form);
  });
})();
