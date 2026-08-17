// Character sheet on/off toggles (skill proficiency bullets, the
// inspiration star) — small forms that used to be plain POST-and-redirect,
// which correctly persisted the toggle but reloaded the whole page, so the
// browser reset scroll position to the top on every single click. These
// intercept the submit instead: flip the visual state immediately
// (optimistic — the click itself already tells us the next state, no need
// to wait on a round trip), POST the change in the background, and only
// reload/revert if the server actually rejects it.
//
// Every toggle form shares this shape: a hidden input named "on" holding
// the value this click should send (server-rendered as the correct next
// state each time), and a submit <button> that both IS the visual toggle
// and carries data-toggle-class naming which CSS class marks it "on".
// data-also-refresh works the same way it does on sheet-vitals.js's
// .sheet-fetch-form handling — a space-separated list of extra element ids
// to re-fetch via window.n5eRefreshBlocks after the primary swap lands.
(function () {
  document.addEventListener("submit", (e) => {
    const form = e.target;
    if (!(form instanceof HTMLFormElement) || !form.classList.contains("sheet-toggle-form")) return;
    e.preventDefault();

    const btn = form.querySelector("button[type=submit]");
    const onField = form.querySelector("input[name=on]");
    if (!btn || !onField) return;

    const turningOn = onField.value === "1";
    const toggleClass = btn.dataset.toggleClass;
    if (toggleClass) btn.classList.toggle(toggleClass, turningOn);

    fetch(form.action, {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded", "X-Requested-With": "fetch" },
      body: new URLSearchParams(new FormData(form)).toString(),
    })
      .then((r) => {
        if (!r.ok) throw new Error("server rejected the request (" + r.status + ")");
        return r.text();
      })
      .then((html) => {
        // A toggle that only changes its own appearance (inspiration) has
        // no data-target and is already done — the optimistic flip above
        // was the whole update, and all that's left is to line the hidden
        // field up for the next click.
        //
        // A toggle that changes numbers elsewhere (a skill proficiency
        // bullet, which moves that skill's modifier and possibly passive
        // Perception) names the block to replace, and the server sends
        // that block back freshly rendered. Swapping it is what makes the
        // bonus actually change rather than just the bullet's colour.
        const targetId = form.dataset.target;
        if (!targetId) {
          onField.value = turningOn ? "0" : "1";
          return;
        }
        const current = document.getElementById(targetId);
        if (current) current.outerHTML = html;
        // Blocks this toggle changes that live outside the swapped
        // fragment — Clash Checks after a Ninshou/Illusions/Martial Arts
        // proficiency flip. See sheet-refresh.js.
        if (window.n5eRefreshBlocks) window.n5eRefreshBlocks(form.dataset.alsoRefresh);
      })
      .catch((err) => {
        console.warn("toggle failed, reverting:", err);
        if (toggleClass) btn.classList.toggle(toggleClass, !turningOn);
      });
  });
})();
