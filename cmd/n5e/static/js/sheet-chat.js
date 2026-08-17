// Character sheet chat/dice-log panel: a plain message form (posts
// straight to /sheet/chat) plus a listener for dice-roller.js's
// "n5e:roll-result" event, dispatched from showResult() for every roll
// made anywhere on the page (the floating dice tray, or a .rollable
// ability/save/skill/initiative click). Formatting/persisting a roll never
// recomputes the math — dice-roller.js's showResult is the one place that
// knows the real total and per-die values; this file only reads them back
// off the event.
(function () {
  const panel = document.querySelector(".sheet-chat-panel");
  const form = document.getElementById("sheet-chat-form");
  const input = document.getElementById("sheet-chat-input");
  if (!panel || !form || !input) return;

  const characterID = panel.dataset.characterId;
  const chatURL = "/characters/" + characterID + "/sheet/chat";

  function postChat(fields) {
    fetch(chatURL, { method: "POST", body: new URLSearchParams(fields) })
      .then((r) => r.text())
      .then((html) => {
        const log = document.getElementById("sheet-chat-log");
        if (log) log.outerHTML = html;
      })
      .catch((err) => console.warn("chat post failed:", err));
  }

  form.addEventListener("submit", (e) => {
    e.preventDefault();
    const text = input.value.trim();
    if (!text) return;
    postChat({ kind: "message", text });
    input.value = "";
  });

  // Roll20-style notation: "1d20(14) + 2 = 16". modifier is folded into one
  // trailing +N/-N term rather than shown per-die, matching how every
  // .rollable row on this sheet already presents its own modifier. Factored
  // out (rather than left inline in the listener below) so a companion
  // popup window — a separate document with no dice log of its own, see
  // companion-fields.js's forwarding listener — can hand its own
  // n5e:roll-result detail to this same window's log via window.n5eLogRoll,
  // instead of duplicating this formatting there.
  function logRoll(detail) {
    const { label, total, modifier, dice, critRange } = detail;
    if (!dice || dice.length === 0) return;

    const parts = dice.map((d) => `${d.values.length}d${d.sides}(${d.values.join(",")})`);
    let text = parts.join(" + ");
    if (modifier) text += (modifier > 0 ? " + " : " - ") + Math.abs(modifier);
    text += " = " + total;
    if (label) text = label + ": " + text;

    // "nat20" is reused as the general "this roll crit" flag/style even
    // when the qualifying value isn't a literal 20 — a companion attack
    // button can widen its own crit range (Puppet Roles' Lurker,
    // companion_fields.html's data-crit-range), and a wider-range crit
    // still deserves the same highlight a natural 20 gets. Undefined
    // critRange (every non-attack roll) falls back to 20, unchanged from
    // before.
    const threshold = critRange || 20;
    let crit = "none";
    for (const d of dice) {
      if (d.sides !== 20) continue;
      if (d.values.some((v) => v >= threshold)) crit = "nat20";
      else if (d.values.includes(1) && crit === "none") crit = "nat1";
    }

    postChat({ kind: "roll", text, crit });
  }
  window.n5eLogRoll = logRoll;

  document.addEventListener("n5e:roll-result", (e) => logRoll(e.detail));
})();
