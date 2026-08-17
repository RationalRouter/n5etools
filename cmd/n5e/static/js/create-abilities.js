// Ability-score step interactivity: method-choice landing (exclusive
// show/hide, single active panel — unlike subclass-tabs.js's multi-select
// Set, "last clicked wins" here), a live Point Buy remaining-points counter,
// and a Manual Entry dice-roll-and-assign convenience. All three pieces are
// independent and each no-ops safely if their markup isn't on the page.
(function () {
  // --- Method-choice landing ----------------------------------------------
  const tiles = document.querySelectorAll(".method-tile");
  const panels = document.querySelectorAll("[data-method-panel]");
  if (tiles.length > 0 && panels.length > 0) {
    // Nothing is shown until a tile is clicked — with JS disabled, no
    // `hidden` attribute is ever applied (it's set here, not baked into the
    // server-rendered HTML), so all 3 panels stay visible as a safe
    // fallback instead.
    panels.forEach((p) => { p.hidden = true; });
    tiles.forEach((tile) => {
      tile.addEventListener("click", () => {
        const method = tile.dataset.method;
        tiles.forEach((t) => t.setAttribute("aria-pressed", String(t === tile)));
        panels.forEach((p) => { p.hidden = p.dataset.methodPanel !== method; });
      });
    });
  }

  // --- Point Buy live counter ----------------------------------------------
  const pointBuyForm = document.getElementById("pointbuy-form");
  const remainingEl = document.getElementById("pointbuy-remaining");
  const costDataEl = document.getElementById("pointbuy-cost-data");
  if (pointBuyForm && remainingEl && costDataEl) {
    let costs = {};
    try {
      costs = JSON.parse(costDataEl.textContent);
    } catch (e) {
      console.error("ability scores: bad point-buy cost data", e);
    }
    const inputs = pointBuyForm.querySelectorAll('input[type="number"]');
    function recompute() {
      let total = 0;
      for (const input of inputs) {
        total += costs[input.value] || 0;
      }
      const remaining = 30 - total;
      remainingEl.textContent = String(remaining);
      remainingEl.classList.toggle("creation-form-error-inline", remaining < 0);
    }
    inputs.forEach((input) => input.addEventListener("input", recompute));
    recompute();
  }

  // --- Manual Entry: roll-and-assign convenience ---------------------------
  const rollBtn = document.getElementById("manual-roll-btn");
  const rollResults = document.getElementById("manual-roll-results");
  const rollChips = document.getElementById("manual-roll-chips");
  const rollAssign = document.getElementById("manual-roll-assign");
  const applyBtn = document.getElementById("manual-roll-apply");
  if (rollBtn && rollResults && rollChips && rollAssign && applyBtn) {
    const abilities = [
      { key: "str", label: "Strength" },
      { key: "dex", label: "Dexterity" },
      { key: "con", label: "Constitution" },
      { key: "int", label: "Intelligence" },
      { key: "wis", label: "Wisdom" },
      { key: "cha", label: "Charisma" },
    ];

    function roll4d6DropLowest() {
      const rolls = [1, 2, 3, 4].map(() => 1 + Math.floor(Math.random() * 6));
      rolls.sort((a, b) => a - b);
      rolls.shift(); // drop the lowest
      return rolls.reduce((a, b) => a + b, 0);
    }

    // Disables a rolled value in every OTHER dropdown once one dropdown has
    // picked it — each of the 6 rolls can only be assigned to one ability.
    // A select's own currently-chosen option is never disabled by this
    // (only checked against the OTHER selects' values), so re-opening the
    // same dropdown still shows its current pick as selectable.
    function refreshDisabledOptions(selects) {
      const chosenElsewhere = selects.map((s) => s.value);
      selects.forEach((select, i) => {
        for (const opt of select.options) {
          if (opt.value === "") continue;
          opt.disabled = chosenElsewhere.some((v, j) => j !== i && v === opt.value);
        }
      });
    }

    rollBtn.addEventListener("click", () => {
      const results = [1, 2, 3, 4, 5, 6].map(() => roll4d6DropLowest());
      rollChips.innerHTML = results.map((r, i) => `<span class="manual-roll-chip" data-roll-index="${i}">${r}</span>`).join("");
      rollAssign.innerHTML = abilities
        .map(
          (ab) => `
        <div class="ability-score-row">
          <label for="roll-assign-${ab.key}">${ab.label}</label>
          <select id="roll-assign-${ab.key}" data-ability="${ab.key}">
            <option value="">—</option>
            ${results.map((r, i) => `<option value="${i}">${r}</option>`).join("")}
          </select>
        </div>`
        )
        .join("");
      rollResults.hidden = false;

      const selects = Array.from(rollAssign.querySelectorAll("select"));
      selects.forEach((select) => select.addEventListener("change", () => refreshDisabledOptions(selects)));
      refreshDisabledOptions(selects);
    });

    applyBtn.addEventListener("click", () => {
      const results = Array.from(rollChips.querySelectorAll(".manual-roll-chip")).map((c) => c.textContent);
      for (const ab of abilities) {
        const select = document.getElementById(`roll-assign-${ab.key}`);
        const input = document.getElementById(`man-${ab.key}`);
        if (!select || !input || select.value === "") continue;
        input.value = results[Number(select.value)];
      }
    });
  }
})();
