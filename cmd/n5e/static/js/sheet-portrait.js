// Character portrait upload. The form works without any of this — it is a
// real <form method=post enctype=multipart/form-data> with a real submit
// button, and posting it normally stores the image and lands back on the
// sheet. What this adds is the two conveniences that make it feel like the
// rest of the sheet: picking a file uploads it immediately (no second
// click on a submit button), and the response replaces just the portrait
// box instead of reloading the page and dropping the player back at the
// top of it.
//
// Unlike every other fetch on this sheet, these bodies are multipart —
// that is required here, because urlencoded bodies cannot carry a file,
// and the portrait handler is the one endpoint that parses with
// ParseMultipartForm to match. Passing the FormData object straight to
// fetch with NO Content-Type header is deliberate: the browser then sets
// the header itself including the multipart boundary, which we could not
// generate by hand.
(function () {
  function submitForm(form) {
    fetch(form.action, {
      method: "POST",
      // No Content-Type: the browser must set it itself so the multipart
      // boundary is correct. X-Requested-With is what tells the handler
      // to answer with a fragment rather than a redirect (see
      // wantsFragment in server.go).
      headers: { "X-Requested-With": "fetch" },
      body: new FormData(form),
    })
      .then((r) => {
        if (!r.ok) {
          return r.text().then((text) => {
            throw new Error(text || "upload failed (" + r.status + ")");
          });
        }
        return r.text();
      })
      .then((html) => {
        const current = document.getElementById(form.dataset.target);
        if (!current) return;
        current.outerHTML = html;
        wire();
      })
      .catch((err) => {
        console.warn("portrait update failed:", err);
        // Size and file-type rejections are the only realistic failures
        // here, and both are the player's to act on — silently doing
        // nothing would look like a broken button.
        window.alert("Could not set the portrait: " + err.message);
      });
  }

  function wire() {
    const uploadForm = document.querySelector("form.sheet-portrait-form");
    if (uploadForm && !uploadForm.dataset.wired) {
      uploadForm.dataset.wired = "1";
      // Only now is the separate submit button redundant — see the
      // .js-wired rule in app.css. Without this script it stays visible,
      // because a file picker with no way to submit is not an upload.
      uploadForm.classList.add("js-wired");
      uploadForm.addEventListener("submit", (e) => {
        e.preventDefault();
        submitForm(uploadForm);
      });
      const file = uploadForm.querySelector("input[type=file]");
      if (file) {
        file.addEventListener("change", () => {
          if (file.files && file.files.length) submitForm(uploadForm);
        });
      }
    }

    const removeForm = document.querySelector("form.sheet-portrait-remove-form");
    if (removeForm && !removeForm.dataset.wired) {
      removeForm.dataset.wired = "1";
      removeForm.addEventListener("submit", (e) => {
        e.preventDefault();
        submitForm(removeForm);
      });
    }
  }

  wire();
})();
