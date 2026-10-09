// Adversary Builder (encounters_builder.html): every change posts the form
// to its data-preview-url, and each [data-builder-live] element in the
// answer replaces the page's element with the same id -- the stat block and
// the steps that depend on other choices (ability array, trait pool,
// passives, jutsu, Solo attachments). Steps the GM is not touching keep
// their state. Focus and any trait group the GM opened carry over the swap,
// and an answer to an older request is dropped. Delegated from document,
// since the swapped elements hold inputs of their own.
(function () {
  let seq = 0;
  let timer = null;

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
        const openGroups = [...form.querySelectorAll("details[open][id]")].map((d) => d.id);

        const doc = new DOMParser().parseFromString(html, "text/html");
        doc.querySelectorAll("[data-builder-live][id]").forEach((fresh) => {
          const current = document.getElementById(fresh.id);
          if (current) current.replaceWith(document.importNode(fresh, true));
        });

        markOpenJutsu();
        openGroups.forEach((id) => {
          const d = document.getElementById(id);
          if (d) d.open = true;
        });
        if (focusID) {
          const el = document.getElementById(focusID);
          if (el && el !== document.activeElement) el.focus({ preventScroll: true });
        }
      })
      .catch((err) => console.warn("adversary builder preview failed:", err));
  }

  // Jutsu detail panel: a jutsu tile's name opens its full card
  // (/jutsu/{slug}?fragment=1) under the stat block. Only the tile's
  // checkbox adds the jutsu. The open tile stays marked across live swaps.
  let openJutsu = "";
  let detailSeq = 0;

  function markOpenJutsu() {
    document.querySelectorAll("[data-jutsu-detail]").forEach((b) => {
      b.setAttribute("aria-pressed", b.dataset.jutsuDetail === openJutsu ? "true" : "false");
    });
  }

  function showJutsu(slug) {
    const panel = document.getElementById("builder-jutsu-detail");
    if (!panel) return;
    const body = panel.querySelector(".encounter-builder-jutsu-detail-body");
    const mine = ++detailSeq;
    openJutsu = slug;
    markOpenJutsu();
    fetch("/jutsu/" + slug + "?fragment=1", { headers: { "X-Requested-With": "fetch" } })
      .then((r) => (r.ok ? r.text() : Promise.reject(new Error("request failed (" + r.status + ")"))))
      .then((html) => {
        if (mine !== detailSeq) return;
        body.innerHTML = html;
        panel.hidden = false;
        panel.scrollTop = 0;
      })
      .catch((err) => {
        if (mine !== detailSeq) return;
        console.warn("jutsu detail failed:", err);
        body.textContent = panel.dataset.error || "";
        panel.hidden = false;
      });
  }

  document.addEventListener("click", (e) => {
    if (!(e.target instanceof Element)) return;
    const tile = e.target.closest("[data-jutsu-detail]");
    if (tile) {
      showJutsu(tile.dataset.jutsuDetail);
      return;
    }
    if (e.target.closest("[data-jutsu-detail-close]")) {
      const panel = document.getElementById("builder-jutsu-detail");
      if (panel) panel.hidden = true;
      detailSeq++;
      openJutsu = "";
      markOpenJutsu();
    }
  });

  // Generate name: fills the name field with a random name for the chosen
  // clan, then previews it like a typed one.
  document.addEventListener("click", (e) => {
    if (!(e.target instanceof Element)) return;
    const btn = e.target.closest("[data-builder-name-roll]");
    const form = btn && builderForm(btn);
    if (!form) return;
    const clan = form.querySelector('[name="clan_tag"]');
    fetch("/encounters/builder/name?clan=" + encodeURIComponent(clan ? clan.value : ""))
      .then((r) => (r.ok ? r.text() : Promise.reject(new Error("request failed (" + r.status + ")"))))
      .then((name) => {
        const input = form.querySelector('[name="name"]');
        input.value = name;
        input.dispatchEvent(new Event("change", { bubbles: true }));
      })
      .catch((err) => console.warn("adversary name failed:", err));
  });

  function builderForm(el) {
    return el instanceof Element ? el.closest("form#encounter-builder-form") : null;
  }

  document.addEventListener("change", (e) => {
    const form = builderForm(e.target);
    if (!form) return;
    clearTimeout(timer);
    timer = setTimeout(() => refresh(form), 120);
  });

  // A search field previews as the GM types.
  document.addEventListener("input", (e) => {
    if (!(e.target instanceof Element) || !e.target.matches("[data-builder-search]")) return;
    const form = builderForm(e.target);
    if (!form) return;
    clearTimeout(timer);
    timer = setTimeout(() => refresh(form), 250);
  });

  // Enter in a number or text field previews instead of saving; Save is
  // its own button.
  document.addEventListener("keydown", (e) => {
    if (e.key !== "Enter" || !(e.target instanceof HTMLInputElement)) return;
    const form = builderForm(e.target);
    if (!form) return;
    e.preventDefault();
    e.target.dispatchEvent(new Event("change", { bubbles: true }));
  });
})();
