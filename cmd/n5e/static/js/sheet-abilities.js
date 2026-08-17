// Editing ability scores from the sheet. Each ability box is a click-to-
// roll target, and the score inside it is a click-to-edit button — so
// every click on the score has to be stopped from bubbling up to the roll
// handler on the box around it, or changing a score would also throw dice.
//
// The value being edited is the BASE score (data-base), not the total
// shown: clan, background and ASI bonuses are added on top of the base by
// charsheet.Compute, so writing the displayed total back would fold those
// bonuses into the base and apply them a second time on the next render.
// The button therefore shows the total while the input holds the base.
//
// On success the page reloads rather than patching the DOM. An ability
// score feeds its own modifier, the matching save, every skill under it,
// passive perception, initiative, AC and every weapon attack line — there
// is no partial update that leaves the sheet consistent. Browsers restore
// scroll position across a reload, so this doesn't jump to the top.
(function () {
  const row = document.querySelector(".sheet-ability-row[data-ability-endpoint]");
  if (!row) return;
  const endpoint = row.dataset.abilityEndpoint;

  row.querySelectorAll(".sheet-ability-box").forEach((box) => {
    const button = box.querySelector(".sheet-ability-score");
    const input = box.querySelector(".sheet-ability-input");
    if (!button || !input) return;

    function close() {
      button.hidden = false;
      input.hidden = true;
    }

    button.addEventListener("click", (e) => {
      e.stopPropagation(); // don't also roll an ability check
      button.hidden = true;
      input.hidden = false;
      input.value = button.dataset.base;
      input.focus();
      input.select();
    });

    // Same guard for the input: a click or keypress inside it must not
    // reach the surrounding box's roll handler either.
    input.addEventListener("click", (e) => e.stopPropagation());

    input.addEventListener("keydown", (e) => {
      e.stopPropagation();
      if (e.key === "Escape") {
        close();
        return;
      }
      if (e.key !== "Enter") return;
      e.preventDefault();
      const value = parseInt(input.value, 10);
      if (Number.isNaN(value) || value < 1 || value > 30) {
        close();
        return;
      }
      const params = new URLSearchParams();
      params.set("ability", button.dataset.ability);
      params.set("value", String(value));
      fetch(endpoint, {
        method: "POST",
        headers: { "Content-Type": "application/x-www-form-urlencoded", "X-Requested-With": "fetch" },
        body: params.toString(),
      })
        .then((r) => {
          if (!r.ok) throw new Error("server rejected the change (" + r.status + ")");
          window.location.reload();
        })
        .catch((err) => {
          console.warn("ability update failed:", err);
          close();
        });
    });

    input.addEventListener("blur", close);
  });
})();
