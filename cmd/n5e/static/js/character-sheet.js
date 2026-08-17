// Character sheet click-to-roll: skill, save and ability checks, plus
// equipped-weapon attack and damage rolls. Jutsu rows still carry no
// .rollable class — a jutsu's roll needs the casting-economy formula,
// which this app does not model yet, and a plausible-looking wrong number
// is worse than no button.
//
// Handled by delegation from the document rather than by binding to each
// .rollable at load time. Half the sheet is re-rendered in place after an
// edit (the skills block after a proficiency toggle, the vitals block
// after a rest), and directly-bound listeners do not survive having their
// element replaced — the rows would silently stop rolling after the first
// toggle, with nothing to suggest why. Delegation binds once, to something
// that is never swapped.
//
// data-count is what makes damage rolls work (2d4, 3d6); it defaults to 1
// so every d20 row can keep leaving it off. Requires window.n5eRoll
// (dice-roller.js) to already be defined — this script is loaded after it
// in layout.html. Entirely inert on every other page, since .rollable only
// exists here.
(function () {
  if (typeof window.n5eRoll !== "function") return;

  // toggleDamagePending disables/enables a companion attack row's own
  // Damage button while its Attack roll is still resolving — see roll()'s
  // own crit-damage-bonus handling below. Scoped to rows that actually
  // carry a crit damage bonus (Puppet Roles' Lurker); every other
  // companion attack row's Damage button is left alone, since there's no
  // bonus a click-before-Attack-resolves could silently miss there.
  function toggleDamagePending(row, pending) {
    const damageBtn = row.querySelector(".sheet-attack-roll[data-crit-damage-bonus]");
    if (!damageBtn || !Number(damageBtn.dataset.critDamageBonus)) return;
    damageBtn.disabled = pending;
  }

  function roll(el) {
    // A companion attack row's own Attack button always carries
    // data-crit-range (see companion_fields.html); its Damage button never
    // does — this is enough to tell the two apart without a class of its
    // own for each.
    const row = el.closest(".companion-attack-row");
    const isAttackRoll = row && el.hasAttribute("data-crit-range");
    let modifier = Number(el.dataset.modifier);

    // Puppet Roles' Lurker: "on a critical hit, add your Proficiency Bonus
    // to the damage dealt." A Damage roll folds in the bonus only when the
    // immediately-preceding Attack roll on the SAME row crit — consumed
    // once so a second Damage click (re-rolling, or just clicking again)
    // doesn't double-apply it.
    if (row && !isAttackRoll) {
      const critDamageBonus = Number(el.dataset.critDamageBonus) || 0;
      if (critDamageBonus && row.dataset.lastAttackCrit === "1") {
        modifier += critDamageBonus;
        delete row.dataset.lastAttackCrit;
      }
    }

    window.n5eRoll({
      sides: Number(el.dataset.sides),
      count: Number(el.dataset.count) || 1,
      modifier,
      label: el.dataset.label || "",
      // Only a companion's own attack-roll button ever sets this (a wider
      // crit range from Puppet Roles' Lurker) — every other .rollable row
      // has no data-crit-range at all, and Number(undefined) below is NaN,
      // so n5eRoll's own default (20) applies exactly like before.
      critRange: Number(el.dataset.critRange) || 20,
      onResult: isAttackRoll
        ? (detail) => {
            const threshold = detail.critRange || 20;
            const crit = detail.dice.some((d) => d.sides === 20 && d.values.some((v) => v >= threshold));
            row.dataset.lastAttackCrit = crit ? "1" : "0";
            delete row.dataset.attackPending;
            toggleDamagePending(row, false);
          }
        : undefined,
    });

    if (isAttackRoll) {
      row.dataset.attackPending = "1";
      toggleDamagePending(row, true);
    }
  }

  document.addEventListener("click", (e) => {
    const el = e.target.closest(".rollable");
    if (el) roll(el);
  });

  document.addEventListener("keydown", (e) => {
    if (e.key !== "Enter" && e.key !== " ") return;
    // Only the row itself, never a control that happens to sit inside one
    // (the ability-score editor's input lives inside a .rollable box, and
    // Enter there means "save this score", not "roll").
    const el = e.target.closest(".rollable");
    if (!el || el !== e.target) return;
    // A real <button> (the attack/damage rolls) already turns Enter and
    // Space into a click by itself; handling the keydown too would roll
    // twice. This branch exists for the rows that are divs with
    // role="button", where the browser does no such thing.
    if (el.tagName === "BUTTON") return;
    e.preventDefault();
    roll(el);
  });
})();
