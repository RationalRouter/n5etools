// Confirmation gate for lowering a class's level in the "Your Classes"
// panel (class_picker.html) — the level <select> has no onchange of its
// own; this delegates the change from document so it survives arriving via
// a fresh page load either the creation flow's Class step or the sheet's
// Add a Class page renders it into (same "delegate from document" rule
// most of this project's other listeners already follow, even though this
// particular panel is never fragment-swapped in — consistency, and it
// costs nothing).
//
// Only a DECREASE asks for confirmation. Raising a level is always safe
// (nothing it grants goes away); lowering one can silently invalidate a
// subclass pick that's only valid at the level it was chosen at, or drop
// proficiencies gained at a level no longer held — neither of which the
// server currently detects or cleans up on its own, so the confirm here is
// the only warning the player gets before that happens.
(function () {
  document.addEventListener("change", (e) => {
    const select = e.target.closest(".your-class-level-select");
    if (!select) return;

    // The server-rendered `selected` attribute (not the live `.value`,
    // which the browser has already updated to the new pick by the time
    // `change` fires) is what "before this edit" actually means here.
    const originalOption = select.querySelector("option[selected]");
    const previous = originalOption ? Number(originalOption.value) : null;
    const next = Number(select.value);

    if (previous !== null && next < previous) {
      const className = select.closest(".your-class-row")?.querySelector(".your-class-name")?.textContent.trim() || "This class";
      const ok = window.confirm(
        `Lower ${className} from level ${previous} to level ${next}? ` +
        "A subclass pick or proficiencies gained at the levels above this one may no longer apply, and nothing here undoes them automatically."
      );
      if (!ok) {
        select.value = String(previous);
        return;
      }
    }
    select.form.submit();
  });
})();
