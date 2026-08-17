// Floating dice roller (see templates/partials/dice_roller.html), present
// on every page. A D&D Beyond-style dice tray: click die faces to build up
// a pool (left-click +1, right-click -1), Roll it, see real physics-based
// 3D dice tumble across the screen plus a total + per-die breakdown.
// Selected counts persist across page loads via localStorage (this app is
// a plain multi-page site, not an SPA, so without that the tray would
// silently forget your pool on every navigation) — rolled results
// deliberately do NOT persist, a fresh page load always shows a clean
// roll state.
//
// The panel only closes via the FAB toggle or its own close button, never
// on an outside click (unlike the nav search / jutsu category dropdowns)
// — the point of a dice tray is to stay open while you browse the rest of
// the page for what to roll next.
//
// 3D rolling is powered by static/js/dice3d.js (geometry in
// static/js/dice3d-solids.js) — our own WebGL renderer and rigid-body
// physics, which replaced a vendored @3d-dice/dice-box. The vendored
// library could not be made to stop launching every die from roughly one
// point, and its die size and per-collision events were equally out of
// reach; all three are ours now.
//
// That layer stays entirely optional: if WebGL is unavailable or the roll
// rejects for any reason, rollFallback below still gives a fully working
// dice roller with a lightweight CSS "slot machine" animation instead of
// real tumbling dice — nothing about the core roll-dice-and-get-a-total
// feature depends on the 3D layer working.
//
// The 3D layer never decides a number: dice3d.js draws every value up
// front and animates towards it, and returns exactly the same
// [{sides, values}] shape rollFallback builds. So showResult and the
// n5e:roll-result event below are identical on both paths.
(function () {
  const fab = document.getElementById("dice-fab");
  const panel = document.getElementById("dice-panel");
  const closeBtn = document.getElementById("dice-close");
  const dice = document.querySelectorAll(".dice-die");
  const resetBtn = document.getElementById("dice-reset");
  const rollBtn = document.getElementById("dice-roll");
  const clearBtn = document.getElementById("dice-clear");
  const results = document.getElementById("dice-results");
  const resultsTotal = document.getElementById("dice-results-total");
  const resultsBreakdown = document.getElementById("dice-results-breakdown");
  const resultsLabel = document.getElementById("dice-results-label");
  const modeBtns = document.querySelectorAll(".dice-mode-btn");
  const elementalYouSelect = document.getElementById("dice-elemental-you");
  const elementalFoeSelect = document.getElementById("dice-elemental-foe");
  const elementalHint = document.getElementById("dice-elemental-hint");
  if (!fab || !panel || dice.length === 0) return;

  const STORAGE_KEY = "n5e-dice-selection";
  const MODE_KEY = "n5e-dice-roll-mode";
  const ELEMENTAL_KEY = "n5e-dice-elemental-matchup";

  // --- Advantage/Disadvantage + Elemental Advantage -----------------------
  // rollMode is the player's own manual Normal/Advantage/Disadvantage
  // choice; the Elemental Matchup selects layer the book's Nature Release
  // circle (Fire > Wind > Lightning > Earth > Water > Fire — the superior
  // side rolls its attack/Clash at Advantage) on top of it. Both are
  // sessionStorage-scoped: a combat-scoped modifier, not a saved character
  // preference, so it's fine (arguably correct) for it to reset on a fresh
  // tab, matching this file's own existing "manual counts" persistence
  // reasoning for what's safe to lose.
  function loadSessionJSON(key, fallback) {
    try {
      const raw = sessionStorage.getItem(key);
      return raw ? JSON.parse(raw) : fallback;
    } catch {
      return fallback;
    }
  }
  function saveSessionJSON(key, value) {
    try {
      sessionStorage.setItem(key, JSON.stringify(value));
    } catch {
      // sessionStorage unavailable — the toggle still works for this page
      // load, it just won't survive a navigation.
    }
  }

  let rollMode = loadSessionJSON(MODE_KEY, "normal");
  if (rollMode !== "advantage" && rollMode !== "disadvantage") rollMode = "normal";
  const elementalMatchup = loadSessionJSON(ELEMENTAL_KEY, { you: "", foe: "" });

  const ELEMENT_ORDER = ["Fire", "Wind", "Lightning", "Earth", "Water"];
  // The book's own circle: the element at index i beats the element at
  // index i+1 (wrapping) — Fire beats Wind, Wind beats Lightning, ...,
  // Water beats Fire.
  function elementalGivesAdvantage(you, foe) {
    if (!you || !foe) return false;
    const i = ELEMENT_ORDER.indexOf(you);
    if (i === -1) return false;
    return ELEMENT_ORDER[(i + 1) % ELEMENT_ORDER.length] === foe;
  }

  // Combines the manual Roll Mode with Elemental Advantage using the
  // standard advantage/disadvantage-cancel rule: having both at once rolls
  // normally, rather than one silently overriding the other.
  function effectiveMode() {
    const elemental = elementalGivesAdvantage(elementalMatchup.you, elementalMatchup.foe);
    if (elemental && rollMode === "disadvantage") return "normal";
    if (elemental) return "advantage";
    return rollMode;
  }

  function renderRollMode() {
    modeBtns.forEach((btn) => btn.classList.toggle("active", btn.dataset.mode === rollMode));
    if (elementalHint) elementalHint.hidden = !elementalGivesAdvantage(elementalMatchup.you, elementalMatchup.foe);
  }
  renderRollMode();

  modeBtns.forEach((btn) => {
    btn.addEventListener("click", () => {
      rollMode = btn.dataset.mode;
      saveSessionJSON(MODE_KEY, rollMode);
      renderRollMode();
    });
  });

  if (elementalYouSelect && elementalFoeSelect) {
    elementalYouSelect.value = elementalMatchup.you;
    elementalFoeSelect.value = elementalMatchup.foe;
    function onElementalChange() {
      elementalMatchup.you = elementalYouSelect.value;
      elementalMatchup.foe = elementalFoeSelect.value;
      saveSessionJSON(ELEMENTAL_KEY, elementalMatchup);
      renderRollMode();
    }
    elementalYouSelect.addEventListener("change", onElementalChange);
    elementalFoeSelect.addEventListener("change", onElementalChange);
  }

  function loadCounts() {
    try {
      const raw = localStorage.getItem(STORAGE_KEY);
      return raw ? JSON.parse(raw) : {};
    } catch {
      return {};
    }
  }
  function saveCounts(counts) {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(counts));
    } catch {
      // localStorage unavailable (private browsing, etc.) — the tray
      // still works for the current page load, it just won't persist.
    }
  }

  const counts = loadCounts();

  function renderCounts() {
    dice.forEach((btn) => {
      const sides = btn.dataset.sides;
      const n = counts[sides] || 0;
      const badge = btn.querySelector(".dice-die-count");
      badge.textContent = String(n);
      badge.hidden = n === 0;
      btn.classList.toggle("active", n > 0);
    });
  }
  renderCounts();

  function setOpen(open) {
    panel.hidden = !open;
    fab.setAttribute("aria-expanded", String(open));
  }

  fab.addEventListener("click", () => setOpen(panel.hidden));
  closeBtn.addEventListener("click", () => setOpen(false));

  dice.forEach((btn) => {
    const sides = btn.dataset.sides;
    btn.addEventListener("click", () => {
      counts[sides] = (counts[sides] || 0) + 1;
      saveCounts(counts);
      renderCounts();
    });
    // Right-click to decrement — a plain click-only tray means one
    // mis-click means clearing everything via Reset just to fix it.
    btn.addEventListener("contextmenu", (e) => {
      e.preventDefault();
      if (!counts[sides]) return;
      counts[sides] -= 1;
      if (counts[sides] <= 0) delete counts[sides];
      saveCounts(counts);
      renderCounts();
    });
  });

  resetBtn.addEventListener("click", () => {
    for (const key of Object.keys(counts)) delete counts[key];
    saveCounts(counts);
    renderCounts();
  });

  clearBtn.addEventListener("click", () => {
    for (const key of Object.keys(counts)) delete counts[key];
    saveCounts(counts);
    renderCounts();
    results.hidden = true;
    if (window.n5eDice3D) window.n5eDice3D.clear();
  });

  // --- Procedural dice-clack sound ---------------------------------------
  // No sound assets vendored — a few short bursts of filtered noise with a
  // fast decay convincingly reads as dice-on-table clacks without needing
  // to source/license/vendor an actual audio file for something this
  // small. Lazily created on first roll since browsers require a user
  // gesture before audio will play, and the Roll click itself is that
  // gesture.
  //
  // strength (0-1) scales the volume so a hard first landing is louder than
  // a die nudging its neighbour on the way to a stop; the 3D layer passes
  // the real contact impulse through, which is exactly what the vendored
  // library could not expose.
  let audioCtx = null;
  function playClack(delaySeconds, strength) {
    if (!audioCtx) {
      const Ctx = window.AudioContext || window.webkitAudioContext;
      if (!Ctx) return;
      audioCtx = new Ctx();
    }
    if (audioCtx.state === "suspended") audioCtx.resume();

    const duration = 0.06;
    const bufferSize = Math.max(1, Math.round(audioCtx.sampleRate * duration));
    const buffer = audioCtx.createBuffer(1, bufferSize, audioCtx.sampleRate);
    const data = buffer.getChannelData(0);
    for (let i = 0; i < bufferSize; i++) {
      data[i] = (Math.random() * 2 - 1) * (1 - i / bufferSize) ** 3;
    }

    const source = audioCtx.createBufferSource();
    source.buffer = buffer;
    const filter = audioCtx.createBiquadFilter();
    filter.type = "bandpass";
    filter.frequency.value = 900 + Math.random() * 900;
    filter.Q.value = 1.2;
    const gain = audioCtx.createGain();
    const scale = typeof strength === "number" ? Math.max(0.25, Math.min(1, strength)) : 1;
    gain.gain.value = (0.25 + Math.random() * 0.15) * scale;

    source.connect(filter).connect(gain).connect(audioCtx.destination);
    source.start(audioCtx.currentTime + delaySeconds);
  }

  // Fallback-only: a handful of staggered clacks timed to roughly cover a
  // typical roll's settle time. The 3D path doesn't need this — it calls
  // playClack directly from real contacts (see rollWith3D).
  function playRollSounds(dieCount) {
    const n = Math.min(10, Math.max(3, dieCount * 2));
    for (let i = 0; i < n; i++) {
      playClack(Math.random() * 1.1);
    }
  }

  // The Roll click is the user gesture browsers require before audio will
  // play, but the first real contact is ~200ms later — by then the gesture
  // is over on some browsers. Creating/resuming the context on the click
  // itself keeps the contact-driven clacks audible.
  function primeAudio() {
    try {
      if (!audioCtx) {
        const Ctx = window.AudioContext || window.webkitAudioContext;
        if (!Ctx) return;
        audioCtx = new Ctx();
      }
      if (audioCtx.state === "suspended") audioCtx.resume();
    } catch {
      audioCtx = null;
    }
  }

  function rollDie(sides) {
    return Math.floor(Math.random() * sides) + 1;
  }

  // Lightweight CSS "slot machine" fallback for when the 3D layer isn't
  // available: flash each selected die face and cycle the total through
  // random plausible values for ~600ms, then settle on the real result.
  //
  // countsSource replaces a direct closure read of `counts` (the manual
  // tray's own persisted pool) — window.n5eRoll below passes its own
  // throwaway {sides: count} object here instead, so a programmatic roll
  // (e.g. a sheet's click-to-roll) can never read from or corrupt the
  // tray's real selection. The manual Roll button below still passes
  // `counts` itself, so its own behavior is unchanged.
  //
  // advMode ("advantage"/"disadvantage"/null) is set only by startRoll,
  // only when sidesWithCounts/countsSource together already describe a
  // lone 2d20 (see startRoll's own isLoneD20 check) — the d20 group's
  // subtotal becomes the kept die (max or min) instead of a sum, described
  // in its own breakdown line rather than the plain "NdM: ... (total)" one.
  function rollFallback(sidesWithCounts, countsSource, modifier, label, advMode, critRange, onResult) {
    const breakdown = [];
    const diceDetail = [];
    let total = 0;
    let maxPossible = 0;
    let dieCount = 0;
    for (const sides of sidesWithCounts) {
      const n = countsSource[sides];
      dieCount += n;
      const rolls = [];
      for (let i = 0; i < n; i++) rolls.push(rollDie(sides));
      let subtotal;
      if (advMode && sides === 20) {
        subtotal = advMode === "disadvantage" ? Math.min(...rolls) : Math.max(...rolls);
        breakdown.push(`d20 (${advMode}): ${rolls.join(", ")} — kept ${subtotal}`);
      } else {
        subtotal = rolls.reduce((a, b) => a + b, 0);
        breakdown.push(`${n}d${sides}: ${rolls.join(", ")} (${subtotal})`);
      }
      total += subtotal;
      maxPossible += n * sides;
      diceDetail.push({ sides, values: rolls });
    }
    total += modifier;
    playRollSounds(dieCount);

    dice.forEach((btn) => {
      if (countsSource[btn.dataset.sides] > 0) btn.classList.add("rolling");
    });
    resultsTotal.textContent = "…";
    results.hidden = false;

    const start = performance.now();
    const duration = 600;
    function tick(now) {
      const elapsed = now - start;
      if (elapsed < duration) {
        resultsTotal.textContent = String(1 + Math.floor(Math.random() * maxPossible));
        requestAnimationFrame(tick);
      } else {
        dice.forEach((btn) => btn.classList.remove("rolling"));
        showResult(total, breakdown, label, diceDetail, modifier, critRange, onResult);
      }
    }
    requestAnimationFrame(tick);
  }

  // Real physics roll via dice3d.js. Groups are handed over in the tray's
  // own descending-sides order and come back in that same order, so the
  // breakdown reads the way the tray is laid out.
  //
  // n5eDice3D.roll always resolves — it draws the values before the first
  // frame and has its own hard cap on how long a roll may take — so the
  // catch below is for a genuinely broken 3D layer (no WebGL, shader
  // failure), not for a slow roll. See rollFallback's doc for why
  // countsSource replaces a `counts` closure, and for what advMode means.
  async function rollWith3D(sidesWithCounts, countsSource, modifier, label, advMode, critRange, onResult) {
    dice.forEach((btn) => {
      if (countsSource[btn.dataset.sides] > 0) btn.classList.add("rolling");
    });
    resultsTotal.textContent = "…";
    results.hidden = false;
    primeAudio();

    const groups = sidesWithCounts.map((sides) => ({ sides, count: countsSource[sides] }));
    let rolled;
    try {
      rolled = await window.n5eDice3D.roll(groups, {
        onImpact: (strength) => playClack(0, strength),
      });
    } finally {
      dice.forEach((btn) => btn.classList.remove("rolling"));
    }

    let total = modifier;
    const breakdown = [];
    const diceDetail = [];
    for (const group of rolled) {
      const values = group.values;
      let subtotal;
      if (advMode && group.sides === 20) {
        subtotal = advMode === "disadvantage" ? Math.min(...values) : Math.max(...values);
        breakdown.push(`d20 (${advMode}): ${values.join(", ")} — kept ${subtotal}`);
      } else {
        subtotal = values.reduce((a, b) => a + b, 0);
        breakdown.push(`${values.length}d${group.sides}: ${values.join(", ")} (${subtotal})`);
      }
      total += subtotal;
      diceDetail.push({ sides: group.sides, values });
    }
    showResult(total, breakdown, label, diceDetail, modifier, critRange, onResult);
  }

  // A lone d20 roll is exactly what Advantage/Disadvantage/Elemental
  // Advantage apply to — every skill/save/ability/attack/Clash check tile
  // on the sheet (window.n5eRoll always passes count=1 for those), plus a
  // manual tray roll with nothing selected but a single d20. A damage roll
  // or any mixed pool is never affected.
  function isLoneD20(sidesWithCounts, countsSource) {
    return sidesWithCounts.length === 1 && sidesWithCounts[0] === 20 && countsSource[20] === 1;
  }

  // Both entry points below go through this: try the 3D layer, fall back to
  // the CSS animation if it isn't there or blows up. Keeping it in one place
  // means the manual Roll button and window.n5eRoll can't drift apart — and
  // now also means Advantage/Disadvantage/Elemental Advantage apply
  // identically no matter which of the two rolled the dice.
  function startRoll(sidesWithCounts, countsSource, modifier, label, critRange, onResult) {
    let advMode = null;
    if (isLoneD20(sidesWithCounts, countsSource)) {
      const mode = effectiveMode();
      if (mode !== "normal") {
        advMode = mode;
        // A fresh object, never the real countsSource (the manual tray's
        // own persisted pool, in the Roll button's case) — this roll needs
        // 2 dice, but the player's selected pool must stay exactly 1.
        countsSource = { 20: 2 };
      }
    }
    if (window.n5eDice3D && window.n5eDice3D.available) {
      rollWith3D(sidesWithCounts, countsSource, modifier, label, advMode, critRange, onResult).catch((err) => {
        console.warn("3D roll failed, falling back to a plain roll:", err);
        rollFallback(sidesWithCounts, countsSource, modifier, label, advMode, critRange, onResult);
      });
      return;
    }
    rollFallback(sidesWithCounts, countsSource, modifier, label, advMode, critRange, onResult);
  }

  // Every roll — the manual tray's own Roll button and window.n5eRoll
  // callers alike — ends up here with a full diceDetail array, so the
  // n5e:roll-result event below fires for any roll made anywhere on a page
  // that has this dice tray. sheet-chat.js (character sheet only) is the
  // one listener today; elsewhere the event just goes unheard.
  function showResult(total, breakdown, label, diceDetail, modifier, critRange, onResult) {
    resultsTotal.textContent = String(total);
    resultsTotal.classList.remove("dice-pulse");
    // Force a reflow so re-adding the class retriggers the animation on
    // consecutive rolls (CSS animations don't restart on a class that's
    // already applied).
    void resultsTotal.offsetWidth;
    resultsTotal.classList.add("dice-pulse");
    // A flat modifier (a skill/attack/save bonus) is folded into `total`
    // by both callers but was never added to `breakdown` itself — the tray
    // showed the raw die result (e.g. "1d20: 8 (8)") right next to a final
    // total that silently included a +11 nowhere displayed in between,
    // reading as if the roll and the total disagreed. Appended here, once,
    // rather than in both rollFallback and rollWith3D, so the two roll
    // paths can't drift apart on this.
    const parts = breakdown.slice();
    if (modifier) parts.push((modifier > 0 ? "+" : "") + modifier + " modifier");
    resultsBreakdown.textContent = parts.join("  •  ");
    if (resultsLabel) {
      resultsLabel.textContent = label || "";
      resultsLabel.hidden = !label;
    }
    // critRange: the lowest d20 result that counts as a critical hit for
    // THIS roll — 20 unless the caller (a companion attack button whose
    // own data-crit-range says otherwise, e.g. Puppet Roles' Lurker) asked
    // for a wider range. sheet-chat.js reads this instead of hardcoding 20
    // so its own crit flag matches the roll it's labeling.
    const detail = { label: label || "", total, modifier: modifier || 0, dice: diceDetail, critRange: critRange || 20 };
    document.dispatchEvent(new CustomEvent("n5e:roll-result", { detail }));
    // onResult: an optional per-call callback (see window.n5eRoll), rather
    // than every caller having to install and tear down its own
    // document-level "n5e:roll-result" listener just to correlate ONE
    // specific roll — Lurker's crit-damage-bonus flow (character-sheet.js)
    // is the first real user, needing to know THIS row's own Attack roll
    // crit, not merely that some roll somewhere on the page crit.
    if (onResult) onResult(detail);
  }

  rollBtn.addEventListener("click", () => {
    const sidesWithCounts = Object.keys(counts)
      .map(Number)
      .filter((sides) => counts[sides] > 0)
      .sort((a, b) => b - a);
    if (sidesWithCounts.length === 0) return;
    startRoll(sidesWithCounts, counts, 0, "");
  });

  // Public entry point for other pages' click-to-roll (e.g. the character
  // sheet's skill/save/ability rows) to trigger a roll without faking
  // clicks on the manual tray. Builds its own local {sides: count} object
  // — see rollFallback's/rollWith3D's countsSource doc — so it never
  // touches the tray's real, localStorage-persisted pool. Opens the panel
  // if it's closed, since the caller isn't the FAB and the result would
  // otherwise be invisible.
  window.n5eRoll = function ({ sides, count = 1, modifier = 0, label = "", critRange = 20, onResult }) {
    setOpen(true);
    startRoll([sides], { [sides]: count }, modifier, label, critRange, onResult);
  };
})();
