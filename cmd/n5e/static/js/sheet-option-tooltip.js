// Generic rollover-tooltip content updater for a <select class="sheet-
// option-tooltip-select"> whose chosen option should show its description
// in a sibling .tooltip-content box — keeps new dropdown pickers (Hand
// Wraps of Passion, and any future one) from each needing their own
// bespoke change-listener, the way sheet-asi-choice.js's pending-asi-
// feat-select case already does for the Pending Choices feat picker (kept
// separate since that one also drives a ⓘ popup link and long-text
// truncation neither of which apply here).
//
// Document-delegated, not bound at load: every one of these selects lives
// inside a sheet-fetch-form fragment that gets replaced via outerHTML on
// submit, the same page-reset-bug shape documented elsewhere in this codebase
// for every other swappable control.
//
// Markup contract: each <option data-description="..."> lives in a
// .sheet-option-tooltip-select whose enclosing <form> also contains a
// .tooltip-content element to write into.
//
// A select with no real enclosing <form> in the DOM (e.g. a companion
// popup/card field, which associates to its actual form via the HTML
// `form="..."` attribute rather than nesting — see
// templates/partials/companion_fields.html's own header comment for why)
// instead points straight at its tooltip with data-tooltip-for="<id>",
// read here before falling back to the closest-form lookup. Both markup
// shapes are handled by the same delegated listener rather than each
// caller needing its own bespoke change handler.
(function () {
  function updateTooltip(select) {
    let content;
    if (select.dataset.tooltipFor) {
      content = document.getElementById(select.dataset.tooltipFor);
    } else {
      const form = select.closest("form");
      content = form && form.querySelector(".tooltip-content");
    }
    if (!content) return;
    const opt = select.selectedOptions[0];
    if (opt && opt.dataset.description) {
      content.textContent = opt.dataset.description;
    }
  }

  document.addEventListener("change", (e) => {
    if (e.target.classList.contains("sheet-option-tooltip-select")) updateTooltip(e.target);
  });
})();
