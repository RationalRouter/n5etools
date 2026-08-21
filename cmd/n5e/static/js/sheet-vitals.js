// Character sheet click-to-edit boxes and their supporting forms: HP, Temp
// HP, Base Temp HP, the rest buttons, and Ryo. Everything here posts to one
// of the /characters/{id}/sheet/{hp,base-temp-hp,rest,ryo} endpoints and
// gets back a re-rendered fragment (see character_sheet.html's
// "sheet_vitals"/"sheet_ryo" blocks and characters.go's
// renderSheetFragment), which replaces its container in place.
//
// Two reasons everything answers with a fragment instead of redirecting:
// HP/THP/Chakra/Hit-Dice move together (a rest or a negative HP delta can
// change several at once, so re-rendering them as a group is what keeps
// them from drifting out of sync client-side), and a redirect reloads the
// page, which throws the player back to the top of a long sheet on every
// single edit.
//
// Elements are wired by data attribute rather than by id so one routine
// covers every box:
//   .sheet-edit-box  data-endpoint  where to POST
//                    data-field     the form field name to send
//                    data-target    id of the container to swap on success
//     [data-role=display]  the button you click to start editing
//     [data-role=edit]     the input that replaces it
//   form.sheet-fetch-form  data-target  same, for plain submit-and-swap forms
//   form.sheet-attack-mod-form select[name=ability]  auto-submits its own
//                    form on "change" — the Attack/Clash ability pickers
//                    have no Set button, so picking a new ability is the
//                    only action needed to save and apply it
//   .sheet-cast-btn  data-endpoint  where to POST
//                    data-field     the form field name to send
//                    data-value     the fixed value to send (no typing — one
//                                   click spends a jutsu's known chakra cost)
//                    data-target    id of the container to swap on success
//                    data-slug      (optional) jutsu slug, forwarded as "slug"
//                    data-rank      (optional) rank cast at, forwarded as "rank"
(function () {
  // Every POST here MUST be application/x-www-form-urlencoded, matching
  // what the Go handlers parse via r.ParseForm() — fetch's automatic
  // Content-Type for a raw FormData body is multipart/form-data instead,
  // which r.ParseForm() silently ignores (it only reads the body for
  // urlencoded requests), so form values arrive empty server-side. Real
  // bug this caused: a 400 response's plain-text error body got swapped
  // straight into #sheet-vitals via outerHTML, replacing the element
  // (and its id) with a plain text node — no id left to find, wireAll()
  // had nothing to re-wire, and the whole block was gone until a full
  // page reload. postForm() below is the one place every fetch in this
  // file goes through, so both the encoding and the error handling only
  // need to be correct once.
  function postForm(url, params) {
    return fetch(url, {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded", "X-Requested-With": "fetch" },
      body: params.toString(),
    }).then((r) => {
      if (!r.ok) {
        return r.text().then((text) => {
          throw new Error("server rejected the request (" + r.status + "): " + text);
        });
      }
      return r.text();
    });
  }

  // Swap a fragment response into its container. Bails out rather than
  // touching the DOM if the container is missing or the response doesn't
  // look like markup, so a bad response can never destroy the element it
  // was supposed to update.
  function swap(targetId, html) {
    const current = document.getElementById(targetId);
    if (!current) return;
    current.outerHTML = html;
    wireAll();
    flashSaveToasts(targetId);
  }

  // A .sheet-save-toast inside a swapped fragment is a brief on-screen
  // confirmation for actions whose own re-render looks identical before
  // and after (e.g. Hand Wraps of Passion's Set button re-shows the same
  // selection it already displayed) — without one, a successful save
  // reads as if nothing happened. Hidden by default in the fragment's own
  // markup and only flashed here, so every other swap (the vast majority,
  // which already show a visible change and include no such element) is
  // an unaffected no-op.
  function flashSaveToasts(targetId) {
    const container = document.getElementById(targetId);
    if (!container) return;
    container.querySelectorAll(".sheet-save-toast").forEach((toast) => {
      toast.hidden = false;
      toast.classList.add("sheet-save-toast-visible");
      toast.addEventListener(
        "animationend",
        () => {
          toast.hidden = true;
          toast.classList.remove("sheet-save-toast-visible");
        },
        { once: true }
      );
    });
  }

  function wireFetchForms() {
    // The short-rest form is excluded on purpose: sheet-rest.js owns it,
    // because it has to roll the hit dice and fill in the resulting
    // totals before the form can be posted at all. Two submit handlers on
    // one form would come down to script load order.
    document.querySelectorAll("form.sheet-fetch-form:not(.sheet-short-rest-form)").forEach((form) => {
      if (form.dataset.wired) return;
      form.dataset.wired = "1";
      const targetId = form.dataset.target;
      form.addEventListener("submit", (e) => {
        e.preventDefault();
        postForm(form.action, new URLSearchParams(new FormData(form)))
          .then((html) => {
            swap(targetId, html);
            // Blocks this action changes that live outside the swapped
            // fragment — the hit-dice squares after a rest. See
            // sheet-refresh.js.
            if (window.n5eRefreshBlocks) window.n5eRefreshBlocks(form.dataset.alsoRefresh);
          })
          .catch((err) => console.warn("sheet update failed:", err));
      });
    });
  }

  // The Attack/Clash ability pickers and the class/subclass line's level
  // and subclass pickers have no Set button (removed/never had one — a
  // bare dropdown is the whole control): selecting a new value submits the
  // form itself. Delegated from the document rather than wired per-select
  // like wireFetchForms' forms are, so it survives every sheet_attack_mods/
  // sheet_level_row swap (including its own) with no re-wiring call needed.
  document.addEventListener("change", (e) => {
    const field = e.target;
    if (!(field instanceof Element) || !field.matches(".sheet-attack-mod-form select[name=ability], .sheet-subclass-select, .sheet-class-level-select")) return;
    if (field.form) field.form.requestSubmit();
  });

  // Click the display to turn it into a text input; Enter submits whatever
  // was typed to the box's own endpoint, Escape or clicking away cancels
  // back to the plain display. The value is sent as typed, with only commas
  // stripped: the HP boxes want a signed delta ("+2"/"-5"), while Ryo also
  // accepts a bare number meaning "set the total to exactly this", and only
  // the server-side handler knows which of its inputs mean what. Sending
  // the raw string keeps that decision in one place instead of splitting it
  // across two languages.
  function wireEditBoxes() {
    document.querySelectorAll(".sheet-edit-box").forEach((box) => {
      if (box.dataset.wired) return;
      box.dataset.wired = "1";

      const display = box.querySelector("[data-role=display]");
      const input = box.querySelector("[data-role=edit]");
      if (!display || !input) return;
      const field = box.dataset.field || "delta";
      const targetId = box.dataset.target;

      function closeEdit() {
        display.hidden = false;
        input.hidden = true;
      }

      function openEdit() {
        if (!input.hidden) return; // already editing
        display.hidden = true;
        input.hidden = false;
        input.value = "";
        input.focus();
      }

      display.addEventListener("click", openEdit);
      // Clicking anywhere else in the box (the padding around the display
      // button, not just the digits themselves) should also open it — Ryo's
      // box is wide enough that most of its area used to be dead space that
      // looked clickable but wasn't. Only while still showing the display,
      // though — once editing, a click anywhere in the box (including the
      // input itself, to place the caret) must not re-trigger openEdit and
      // wipe out whatever was already typed.
      box.addEventListener("click", () => {
        if (input.hidden) openEdit();
      });

      input.addEventListener("keydown", (e) => {
        if (e.key === "Escape") {
          closeEdit();
          return;
        }
        if (e.key !== "Enter") return;
        e.preventDefault();
        const value = input.value.replace(/,/g, "").trim();
        if (value === "" || Number.isNaN(Number(value))) {
          closeEdit();
          return;
        }
        const params = new URLSearchParams();
        params.set(field, value);
        postForm(box.dataset.endpoint, params)
          .then((html) => swap(targetId, html))
          .catch((err) => {
            console.warn("sheet edit failed:", err);
            closeEdit();
          });
      });

      input.addEventListener("blur", closeEdit);
    });
  }

  // One-click fixed-value spends (e.g. "Cast" on a jutsu row) — no
  // display/edit-input toggle, just a POST of the button's own data-value.
  // A jutsu Cast button also carries data-slug/data-rank (see
  // character_sheet.html and sheet-jutsu-upcast.js) so the cast endpoint
  // knows which jutsu was cast, at what rank — needed to start concentration
  // tracking when that jutsu requires it. A "Cast via <Pool>" button
  // (jutsuFreeCastGrant, characters.go) also carries data-resource-key and,
  // for a grant that spends more than one pool use per cast (Wolves
  // Legacy's Wolf Techniques), data-resource-uses. Forwarded only when
  // present, so this stays a no-op for any other .sheet-cast-btn that
  // doesn't set them.
  function wireCastButtons() {
    document.querySelectorAll(".sheet-cast-btn").forEach((btn) => {
      if (btn.dataset.wired) return;
      btn.dataset.wired = "1";
      btn.addEventListener("click", () => {
        const params = new URLSearchParams();
        params.set(btn.dataset.field || "delta", btn.dataset.value);
        if (btn.dataset.slug) params.set("slug", btn.dataset.slug);
        if (btn.dataset.rank) params.set("rank", btn.dataset.rank);
        if (btn.dataset.resourceKey) params.set("resource_key", btn.dataset.resourceKey);
        if (btn.dataset.resourceUses) params.set("resource_uses", btn.dataset.resourceUses);
        postForm(btn.dataset.endpoint, params)
          .then((html) => swap(btn.dataset.target, html))
          .catch((err) => console.warn("cast failed:", err));
      });
    });
  }

  // Re-runs after every swap: the fragment response is a brand new element
  // with brand new children, so none of the old listeners carry over. The
  // data-wired flag is what keeps this from stacking a second listener on
  // the boxes that were NOT part of the swap.
  function wireAll() {
    wireFetchForms();
    wireEditBoxes();
    wireCastButtons();
  }

  // Exposed so any OTHER script that replaces DOM via outerHTML (sheet-rest.js
  // for the short-rest form it owns directly; sheet-refresh.js for blocks
  // like sheet-squares that live outside #sheet-vitals but can still contain
  // a .sheet-edit-box, e.g. Speed) can re-run this pass afterward instead of
  // leaving whatever it just swapped in permanently unwired. This is the
  // general form of the exact bug already fixed once for the short-rest
  // form's own listener (see sheet-rest.js's header comment) — it recurs
  // anywhere a swap doesn't call this, because the swapped-in markup then has
  // zero JS listeners until the next full page load, and clicking a
  // .sheet-fetch-form inside it falls through to a real, unhandled form
  // submission: a full-page POST/redirect that reloads the page and resets
  // scroll to the top. Any new swap/refresh call site must call this
  // rewiring pass afterward, or delegate its listener from document instead.
  window.n5eRewireControls = wireAll;

  wireAll();
})();
