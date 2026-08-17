// Short rest: the player says how many hit dice and chakra dice to spend,
// and the app rolls them.
//
// The rolls go through window.n5eRoll, the same entry point every other
// click-to-roll on the sheet uses, rather than a private Math.random(). Two
// reasons that matters: the dice physically appear in the tray so the
// player watches the roll happen, and the result flows into the persisted
// chat/dice log for free, because sheet-chat.js is already listening for
// the n5e:roll-result event that every roll dispatches.
//
// n5eRoll has no return value — it dispatches its result as an event — so
// this waits for that one event to come back before posting the totals.
// Rolls are user-initiated one at a time, so there is no realistic way for
// two to be in flight at once; the timeout exists only so a roll that never
// reports back leaves the form usable instead of permanently stuck.
//
// Bound by delegation from the document, NOT to the form at load. The
// short-rest form lives inside #sheet-vitals, which every vitals fetch
// (an HP edit, a Temp HP edit, the Max HP/Max Chakra pin, and a rest
// itself) replaces wholesale — a listener bound to the original element
// dies with it, and sheet-vitals.js's re-wiring pass deliberately skips
// this form, so nothing would ever put one back. The symptom was the
// classic one for this bug: short rest works exactly once per page load,
// then silently reverts to a full-page POST with no dice rolled.
//
// The same swap is why "can this browser roll?" is recorded as a class on
// <body> rather than on the form. .js-wired was the old marker and it is
// re-rendered away every time; <body> is never swapped.
(function () {
  // sheet-vitals.js deliberately skips this form (its selector excludes
  // .sheet-short-rest-form) so there is exactly one submit handler here
  // and no ordering question about which of two runs first.
  //
  // The dice roller is the only optional part: without window.n5eRoll the
  // plain HP/Chakra number fields stay visible and the player types the
  // totals themselves, which is what .n5e-can-roll hides in app.css.
  const canRoll = typeof window.n5eRoll === "function";
  if (canRoll) document.body.classList.add("n5e-can-roll");

  function rollDice(count, sides, modifier, label) {
    return new Promise((resolve) => {
      if (!count || !sides) {
        resolve(0);
        return;
      }
      let settled = false;
      const done = (value) => {
        if (settled) return;
        settled = true;
        document.removeEventListener("n5e:roll-result", onResult);
        clearTimeout(timer);
        resolve(value);
      };
      const onResult = (e) => done(e.detail.total);
      const timer = setTimeout(() => {
        console.warn("short rest: no roll result came back, treating as 0");
        done(0);
      }, 15000);

      document.addEventListener("n5e:roll-result", onResult);
      window.n5eRoll({ sides, count, modifier: modifier * count, label });
    });
  }

  document.addEventListener("submit", (e) => {
    const form = e.target;
    if (!(form instanceof HTMLFormElement) || !form.classList.contains("sheet-short-rest-form")) return;
    e.preventDefault();

    if (!canRoll) {
      post(form, new URLSearchParams(new FormData(form)));
      return;
    }

    const hitDice = Number(form.elements.hit_dice_delta.value) || 0;
    const chakraDice = Number(form.elements.chakra_dice_delta.value) || 0;
    if (hitDice <= 0 && chakraDice <= 0) return;

    const hitDie = Number(form.dataset.hitDie) || 0;
    const chakraDie = Number(form.dataset.chakraDie) || 0;
    const conMod = Number(form.dataset.conMod) || 0;

    rollDice(hitDice, hitDie, conMod, "Short Rest — Hit Dice")
      .then((hp) => rollDice(chakraDice, chakraDie, conMod, "Short Rest — Chakra Dice")
        .then((chakra) => ({ hp, chakra })))
      .then(({ hp, chakra }) => {
        const params = new URLSearchParams(new FormData(form));
        // Negative totals are possible with a bad Constitution modifier;
        // a rest must never take HP away. The server clamps the upper end
        // against the character's maximum.
        params.set("hp", String(Math.max(0, hp)));
        params.set("chakra", String(Math.max(0, chakra)));
        post(form, params);
      });
  });

  function post(form, params) {
    fetch(form.action, {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded", "X-Requested-With": "fetch" },
      body: params.toString(),
    })
      .then((r) => {
        if (!r.ok) throw new Error("server rejected the rest (" + r.status + ")");
        return r.text();
      })
      .then((html) => {
        const current = document.getElementById(form.dataset.target);
        if (current) current.outerHTML = html;
        // #sheet-vitals holds every other vitals control too (HP/THP/Chakra
        // edit-boxes, Base Temp HP, Max HP/Chakra, the Cast buttons, and the
        // Long Rest form) — none of those are this form's to know about, but
        // they just got replaced along with it and go dead without this.
        // Skipping it once meant Long Rest silently reverted to a real form
        // submission (full-page reload, scroll reset to the top) the moment
        // any short rest happened first on the page.
        if (window.n5eRewireControls) window.n5eRewireControls();
        // Spending hit dice also changes the hit-dice counter in the square
        // row at the top of the left column, which is outside this
        // response's fragment — see sheet-refresh.js.
        if (window.n5eRefreshBlocks) window.n5eRefreshBlocks(form.dataset.alsoRefresh);
      })
      .catch((err) => console.warn("short rest failed:", err));
  }
})();
