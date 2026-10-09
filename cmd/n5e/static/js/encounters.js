// Encounter Generator (Dungeon Master Tools): rerolling, adding, or deleting
// an adversary swaps only the roster table/card (#encounter-roster), and
// every turn-order tracker action swaps only #encounter-turn-order, in
// place instead of a full page reload/redirect, the same "known, bounded
// region" reasoning sheet-jutsu-fetch.js already documents for jutsu
// learn/forget. Delegated from document (not bound per-form at load) so any
// one of these actions replacing #encounter-roster keeps every OTHER
// roster form inside it working with no separate rewiring pass needed —
// same pattern sheet-jutsu-fetch.js and confirm-submit.js already use for
// this exact reason. A delete form's own data-confirm is handled separately
// by confirm-submit.js, which runs in the capture phase before this.
(function () {
  document.addEventListener("submit", (e) => {
    const form = e.target;
    if (!(form instanceof HTMLFormElement)) return;
    if (!form.classList.contains("encounter-roster-form")) return;
    e.preventDefault();

    const body = new URLSearchParams(new FormData(form));

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
        const target = document.getElementById(form.dataset.target);
        if (target) target.outerHTML = html;
        if (form.dataset.target === "encounter-roster") refreshTurnOrder();
      })
      .catch((err) => {
        console.warn("encounter action failed:", err);
        window.alert("Could not save that change: " + err.message);
      });
  });

  // The turn-order tracker's "add adversary" picker lists the saved roster,
  // so any roster change re-fetches the tracker too. Its open/closed state
  // is carried over so a collapsed tracker stays collapsed.
  function refreshTurnOrder() {
    const tracker = document.getElementById("encounter-turn-order");
    if (!tracker || !tracker.dataset.refreshUrl) return;
    const wasOpen = tracker.open;
    fetch(tracker.dataset.refreshUrl, { headers: { "X-Requested-With": "fetch" } })
      .then((r) => (r.ok ? r.text() : Promise.reject(new Error("request failed (" + r.status + ")"))))
      .then((html) => {
        const current = document.getElementById("encounter-turn-order");
        if (!current) return;
        current.outerHTML = html;
        const fresh = document.getElementById("encounter-turn-order");
        if (fresh) fresh.open = wasOpen;
      })
      .catch((err) => console.warn("turn order refresh failed:", err));
  }

  // A jutsu on the turn-order unit card loads its /jutsu card the first
  // time it is expanded. "toggle" does not bubble, so this listens in the
  // capture phase.
  document.addEventListener("toggle", (e) => {
    const item = e.target;
    if (!(item instanceof HTMLDetailsElement) || !item.classList.contains("encounter-unit-jutsu")) return;
    const body = item.querySelector(".encounter-unit-jutsu-body");
    if (!item.open || !body || body.dataset.loaded) return;
    body.dataset.loaded = "1";
    fetch("/jutsu/" + item.dataset.jutsuSlug + "?fragment=1", { headers: { "X-Requested-With": "fetch" } })
      .then((r) => (r.ok ? r.text() : Promise.reject(new Error("request failed (" + r.status + ")"))))
      .then((html) => { body.innerHTML = html; })
      .catch((err) => {
        console.warn("jutsu detail failed:", err);
        delete body.dataset.loaded;
        body.textContent = item.closest("[data-jutsu-error]")?.dataset.jutsuError || "";
      });
  }, true);

  // Initiative and max HP re-sort or refill the tracker, so a changed value
  // submits its fragment-swapping form straight away.
  document.addEventListener("change", (e) => {
    const field = e.target;
    if (!(field instanceof HTMLInputElement)) return;
    if (!field.classList.contains("encounter-turn-autosubmit") || !field.form) return;
    field.form.requestSubmit();
  });

  // Current HP and the note save on blur without swapping anything: the
  // server answers with the resolved value ("-7" becomes the new total),
  // written back so a leftover delta is never applied twice.
  document.addEventListener("focusout", (e) => {
    const field = e.target;
    if (!(field instanceof HTMLInputElement)) return;
    if (!field.classList.contains("encounter-turn-plain-field") || !field.form) return;
    if (field.value === field.defaultValue) return;
    fetch(field.form.action, {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded", "X-Requested-With": "fetch" },
      body: new URLSearchParams(new FormData(field.form)).toString(),
    })
      .then((r) => (r.ok ? r.text() : Promise.reject(new Error("request failed (" + r.status + ")"))))
      .then((text) => {
        field.value = text;
        field.defaultValue = text;
      })
      .catch((err) => console.warn("turn order field save failed:", err));
  });

  // Turn-order reordering, same gesture as the character sheet's tiles
  // (sheet-subgrid.js): pressing the ⠿ handle arms its row as draggable,
  // dragging over another row moves it above or below by midpoint, and the
  // drop posts the whole top-to-bottom id list. Arrow Up/Down on a focused
  // handle moves one step, for keyboard use.
  let draggingRow = null;
  let orderBeforeDrag = "";

  function rowOrder(tbody) {
    return [...tbody.querySelectorAll("tr[data-turn-id]")].map((r) => r.dataset.turnId).join(",");
  }

  function saveOrder(tbody, focusTurnID) {
    const body = new URLSearchParams();
    body.set("order", rowOrder(tbody));
    fetch(tbody.dataset.turnReorderUrl, {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded", "X-Requested-With": "fetch" },
      body: body.toString(),
    })
      .then((r) => {
        // 409: the tracker changed elsewhere; reload it rather than guess.
        if (r.status === 409) { refreshTurnOrder(); return null; }
        return r.ok ? r.text() : Promise.reject(new Error("request failed (" + r.status + ")"));
      })
      .then((html) => {
        if (html === null) return;
        const tracker = document.getElementById("encounter-turn-order");
        if (tracker) tracker.outerHTML = html;
        if (focusTurnID) {
          const handle = document.querySelector('#encounter-turn-order tr[data-turn-id="' + focusTurnID + '"] .encounter-turn-handle');
          if (handle) handle.focus();
        }
      })
      .catch((err) => {
        console.warn("turn reorder failed:", err);
        refreshTurnOrder();
      });
  }

  document.addEventListener("mousedown", (e) => {
    const handle = e.target.closest(".encounter-turn-handle");
    if (!handle) return;
    const row = handle.closest("tr[data-turn-id]");
    if (row) row.draggable = true;
  });
  // The gesture can end anywhere; a row left draggable would turn a later
  // text selection inside its inputs into a drag.
  document.addEventListener("mouseup", () => {
    document.querySelectorAll('#encounter-turn-order tr[draggable="true"]').forEach((r) => {
      if (r !== draggingRow) r.draggable = false;
    });
  });
  document.addEventListener("dragstart", (e) => {
    const row = e.target instanceof Element && e.target.closest("tr[data-turn-id]");
    if (!row || !row.draggable) return;
    draggingRow = row;
    orderBeforeDrag = rowOrder(row.parentElement);
    row.classList.add("encounter-turn-dragging");
    e.dataTransfer.effectAllowed = "move";
    e.dataTransfer.setData("text/plain", "");
  });
  document.addEventListener("dragover", (e) => {
    if (!draggingRow) return;
    const tbody = draggingRow.parentElement;
    if (!(e.target instanceof Element) || !tbody.contains(e.target)) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = "move";
    const before = [...tbody.querySelectorAll("tr[data-turn-id]")]
      .filter((r) => r !== draggingRow)
      .find((r) => {
        const rect = r.getBoundingClientRect();
        return e.clientY < rect.top + rect.height / 2;
      });
    tbody.insertBefore(draggingRow, before || null);
  });
  document.addEventListener("drop", (e) => {
    if (draggingRow && e.target instanceof Element && draggingRow.parentElement.contains(e.target)) e.preventDefault();
  });
  document.addEventListener("dragend", () => {
    if (!draggingRow) return;
    const row = draggingRow;
    const tbody = row.parentElement;
    draggingRow = null;
    row.draggable = false;
    row.classList.remove("encounter-turn-dragging");
    if (tbody && rowOrder(tbody) !== orderBeforeDrag) saveOrder(tbody);
  });
  document.addEventListener("keydown", (e) => {
    const handle = e.target instanceof Element && e.target.closest(".encounter-turn-handle");
    if (!handle || (e.key !== "ArrowUp" && e.key !== "ArrowDown")) return;
    const row = handle.closest("tr[data-turn-id]");
    const tbody = row && row.parentElement;
    if (!tbody) return;
    e.preventDefault();
    const sibling = e.key === "ArrowUp" ? row.previousElementSibling : row.nextElementSibling;
    if (!sibling) return;
    if (e.key === "ArrowUp") tbody.insertBefore(row, sibling);
    else tbody.insertBefore(sibling, row);
    saveOrder(tbody, row.dataset.turnId);
  });

  // Enter in a plain field confirms it the same way clicking away does,
  // instead of navigating to the endpoint's bare-text response.
  document.addEventListener("submit", (e) => {
    const form = e.target;
    if (!(form instanceof HTMLFormElement)) return;
    if (!form.classList.contains("encounter-turn-plain-form")) return;
    e.preventDefault();
    if (document.activeElement instanceof HTMLElement) document.activeElement.blur();
  });
})();
