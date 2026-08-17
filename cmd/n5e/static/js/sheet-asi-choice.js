// Progressive disclosure for the Pending Choices "Ability Score Improvement
// or Feat" picker (character_sheet.html's sheet_feature_choices fragment,
// one .pending-asi-form per unresolved breakpoint). Every field for both
// branches is rendered server-side up front; this only shows/hides and
// enables/disables whichever branch matches the current "Choice"/"Method"
// selects, so the form only ever submits the fields that are actually
// relevant — a disabled field is excluded from submission entirely and is
// also exempt from HTML's own `required` validation, which matters since
// only one branch's fields are ever really required.
//
// Document-delegated rather than bound at load: this fragment is swapped in
// place after every successful pick (data-target="sheet-feature-choices"),
// so a directly-bound listener would go dead after the first submit, the
// same page-reset-bug shape documented elsewhere in this codebase for
// every other swappable block.
(function () {
  function setBranchActive(branch, active) {
    if (!branch) return;
    branch.hidden = !active;
    branch.querySelectorAll("select, input").forEach((el) => {
      el.disabled = !active;
    });
  }

  function applyMode(form) {
    const modeSelect = form.querySelector(".pending-asi-mode-select");
    if (!modeSelect) return;
    const isFeat = modeSelect.value === "feat";
    setBranchActive(form.querySelector(".pending-asi-branch-asi"), !isFeat);
    setBranchActive(form.querySelector(".pending-asi-branch-feat"), isFeat);
  }

  function applyMethod(form) {
    const methodSelect = form.querySelector(".pending-asi-method-select");
    const ability2Field = form.querySelector(".pending-asi-ability2-field");
    const ability1Label = form.querySelector(".pending-asi-ability1-label");
    if (!methodSelect || !ability2Field) return;
    const isDouble = methodSelect.value === "double";
    setBranchActive(ability2Field, !isDouble);
    if (ability1Label) {
      ability1Label.textContent = isDouble ? "Ability to raise by +2" : "First ability (+1)";
    }
  }

  // Some feats (Clone Specialization and the like) run long enough that the
  // full text blows the tooltip box off the top and bottom of the viewport
  // — the fixed-position hover box has no scroll of its own. Past this
  // length, show a truncated preview and point at the ⓘ popup (a real
  // /feats/{slug} page, via sheet-popup.js's existing dialog) for the rest,
  // rather than growing the box without bound.
  const FEAT_TOOLTIP_CHAR_LIMIT = 500;

  function updateFeatDescription(select) {
    const form = select.closest(".pending-asi-form");
    const content = form && form.querySelector(".pending-asi-feat-description");
    const popupLink = form && form.querySelector(".pending-asi-feat-popup");
    if (!content) return;
    const opt = select.selectedOptions[0];
    const description = (opt && opt.dataset.description) || "";
    const hasFeat = !!(opt && opt.value);
    if (!hasFeat) {
      content.textContent = "Choose a feat above to see its description.";
    } else if (description.length > FEAT_TOOLTIP_CHAR_LIMIT) {
      content.textContent = description.slice(0, FEAT_TOOLTIP_CHAR_LIMIT).trimEnd() + "… Click ⓘ for the full text.";
    } else {
      content.textContent = description;
    }
    if (popupLink) {
      if (hasFeat) {
        popupLink.href = "/feats/" + opt.value;
        popupLink.classList.add("sheet-popup-trigger");
      } else {
        popupLink.removeAttribute("href");
        popupLink.classList.remove("sheet-popup-trigger");
      }
    }
  }

  document.addEventListener("change", (e) => {
    if (e.target.classList.contains("pending-asi-mode-select")) applyMode(e.target.closest(".pending-asi-form"));
    if (e.target.classList.contains("pending-asi-method-select")) applyMethod(e.target.closest(".pending-asi-form"));
    if (e.target.classList.contains("pending-asi-feat-select")) updateFeatDescription(e.target);
  });
})();
