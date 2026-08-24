// Notifies the opener window to refresh once a "subclass tracker popup"
// (subclass_tracker_popup.go — Titan Slots, SNB Upgrades, EIP/W.o.W, Spyware
// Programs, and every other subclass's own cap+catalog pick tracker) loads
// or reloads. Every add/remove in that pattern is a plain POST-and-redirect
// back to this same popup URL (see that file's own header doc on why a real
// window.open() popup has no other way back into the opener's DOM) — so
// "this page just (re)loaded" already covers "a pick was just added or
// removed", with no separate save event to hook the way companion-fields.js
// hooks a field's own focusout/fetch.
//
// RefreshOpenerBlocks (subclass_tracker_popup.go) is read off the page's own
// <h1> (subclass_tracker_popup_header, partials/subclass_tracker_popup.html)
// as a space-separated list of character_sheet.html element ids, and handed
// straight to window.opener.n5eRefreshBlocks — the exact mechanism
// companion-fields.js's own notifyOpenerToRefresh already uses for a
// companion popup's field saves. Guarded in a try/catch since window.opener
// can be closed, cross-origin, or have navigated away by the time this
// fires — none of those are failures worth surfacing.
(function () {
  const marker = document.querySelector("[data-refresh-opener-blocks]");
  const blocks = marker && marker.dataset.refreshOpenerBlocks;
  if (!blocks) return;
  try {
    if (window.opener && !window.opener.closed && window.opener.n5eRefreshBlocks) {
      window.opener.n5eRefreshBlocks(blocks);
    }
  } catch (err) {
    console.warn("could not refresh the main sheet tab:", err);
  }
})();
