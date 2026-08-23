// Persists which companion cards are minimized on the Companions tab
// (sheet_summon_tab's own <details class="companion-card">, [open] driven
// on render by SummonsTab.Collapsed) — server-side, under
// character_sheet_ui_state's "summons:collapsed" key (a JSON array of
// companion ids, decoded by parseCollapsedCompanionIDs), not localStorage:
// the binary binds a fresh OS-assigned port on every launch, and
// localStorage is scoped per-origin (port included), so it would silently
// forget every minimized card on the very next launch — see
// sheet-layout.js's own header comment for the identical gotcha already
// hit once with the box-grid layout.
//
// The native `toggle` event does not bubble, so the usual bubble-phase
// document.addEventListener delegation every other control on this page
// relies on (confirm-submit.js, .rollable, .sheet-toggle-form) can't reach
// it. Listening on `document` in the CAPTURE phase instead is the one way
// to delegate a non-bubbling event without binding a fresh listener to
// every card — a binding that would go dead the instant an unrelated save
// elsewhere on this same tab re-renders #sheet-summon-tab wholesale
// (loadSummonsTabData re-renders every companion's card on any change, not
// just the one that changed, per that fragment's own doc comment).
//
// The Puppets tab's own companion-card <details> (sheet_puppet_tab)
// hardcodes `open` and has no saved state of its own, so this only ever
// reacts to a toggle whose target lives inside #sheet-summon-tab.
(function () {
  document.addEventListener(
    "toggle",
    (e) => {
      const details = e.target;
      if (!(details instanceof HTMLDetailsElement) || !details.classList.contains("companion-card")) return;
      const tab = document.getElementById("sheet-summon-tab");
      if (!tab || !tab.contains(details)) return;

      const idPanel = document.querySelector("[data-character-id]");
      const characterID = idPanel ? idPanel.dataset.characterId : null;
      if (!characterID) return;

      // Recomputed from the whole tab's own current DOM state rather than
      // tracked incrementally — cheap (at most a handful of companions) and
      // immune to ever drifting from what's actually on screen, the same
      // "read the live DOM, don't shadow it in a separate running total"
      // approach sheet-layout.js's own saveLayout() uses for the box grid.
      const collapsed = [];
      tab.querySelectorAll(".companion-card[data-companion-id]").forEach((card) => {
        if (!card.open) collapsed.push(Number(card.dataset.companionId));
      });

      const body = new URLSearchParams();
      body.set("key", "summons:collapsed");
      body.set("data", JSON.stringify(collapsed));
      fetch("/characters/" + characterID + "/sheet/ui-state", {
        method: "POST",
        headers: { "Content-Type": "application/x-www-form-urlencoded", "X-Requested-With": "fetch" },
        body: body.toString(),
      }).catch((err) => console.warn("save companion collapse state failed:", err));
    },
    true
  );
})();
