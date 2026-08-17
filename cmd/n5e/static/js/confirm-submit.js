// Confirmation gate for destructive form submissions.
//
// Any <form data-confirm="..."> asks before it submits. Delegated from the
// document so it covers markup that arrives later via a fragment swap (the
// sheet does this constantly), and registered in the capture phase so it
// runs before the fetch-and-swap submit handlers in sheet-*.js — those call
// preventDefault() and fetch immediately, so a bubble-phase confirm here
// would arrive after the request was already on its way.
//
// Progressive enhancement in the honest direction: with JS off the form
// still submits, i.e. the delete still works, it just doesn't ask first.
// The server treats the POST as authoritative either way.
(function () {
  document.addEventListener(
    "submit",
    (e) => {
      const form = e.target.closest("form[data-confirm]");
      if (!form) return;
      if (!window.confirm(form.dataset.confirm)) {
        e.preventDefault();
        e.stopPropagation();
      }
    },
    true,
  );
})();
