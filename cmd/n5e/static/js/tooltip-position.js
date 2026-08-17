// Tooltip chips (.tooltip/.tooltip-content, partials/tooltip.html) anchor
// their popup box to the trigger's own left edge (see app.css) with no
// awareness of what actually bounds them — a trigger sitting near the right
// edge of a wide row (e.g. the last chip in a Puppet Upgrades tier, or a
// Seals chip inside a Companions card) pushes its popup box's right edge
// past whatever clips it, cutting off whatever text doesn't fit instead of
// wrapping or scrolling into view. app.css's own max-width only keeps the
// box from being WIDER than the viewport; it does nothing about an anchor
// point that's already too close to an edge.
//
// The clipping edge is NOT always the browser window: every sheet box
// (.sheet-box, app.css) is `overflow: auto` so its own draggable/resizable
// grid cell can scroll — a scrollable ancestor clips an absolutely-
// positioned descendant's visual overflow at ITS OWN boundary regardless of
// how much further the viewport extends, the same way any `overflow:auto`
// container clips content, position:absolute or not. Checking only
// window.innerWidth (the original version of this file) missed this
// entirely: a tooltip well inside the browser window but pushed past its
// own narrow panel's right edge never triggered the flip, and the panel's
// own overflow clipped it anyway (see the seals-chip tooltip, cut off mid-
// sentence at the panel edge, well short of the actual window edge).
// clipRect() finds the nearest ancestor(s) with non-visible overflow on
// either axis and intersects their boxes down to one effective clip
// rectangle, falling back to the viewport itself if there are none.
//
// Delegated from document (mouseover/focusin bubble), not bound per-tooltip
// at load, so it covers every .tooltip added later via a fragment swap with
// no rewire pass needed. The box is only measurable once it's actually
// shown — app.css hides it via opacity/visibility, not display:none, so its
// layout box (and thus getBoundingClientRect) is already correct even
// before the hover-triggered opacity transition finishes.
(function () {
  function clipRect(tooltip) {
    let rect = { top: 0, left: 0, right: window.innerWidth, bottom: window.innerHeight };
    let el = tooltip.parentElement;
    while (el && el !== document.body) {
      const style = getComputedStyle(el);
      if (style.overflowX !== "visible" || style.overflowY !== "visible") {
        // getBoundingClientRect() is the BORDER box — for a box that's
        // itself scrollable (every .sheet-box has its own overflow-y:auto
        // for the panel's vertical scroll), that includes whatever gutter
        // the browser reserves for ITS OWN scrollbar. clientWidth/
        // clientHeight already exclude that gutter (and the border), so
        // deriving the clip edges from those instead of the raw rect is
        // what actually keeps a clamped tooltip from landing ON TOP of
        // the scrollbar and nudging a second, horizontal one into
        // existence right as it's shown — confirmed live: without this,
        // a tooltip clamped to the border-box edge still triggered a
        // ~15px (one scrollbar's width) horizontal overflow the instant
        // .tooltip:hover set overflow:visible.
        const r = el.getBoundingClientRect();
        const left = r.left + el.clientLeft;
        const top = r.top + el.clientTop;
        rect = {
          top: Math.max(rect.top, top),
          left: Math.max(rect.left, left),
          right: Math.min(rect.right, left + el.clientWidth),
          bottom: Math.min(rect.bottom, top + el.clientHeight),
        };
      }
      el = el.parentElement;
    }
    return rect;
  }

  function positionTooltip(tooltip) {
    const content = tooltip.querySelector(".tooltip-content");
    if (!content) return;
    content.classList.remove("tooltip-content-below");
    const bounds = clipRect(tooltip);

    // Horizontal: clamp within bounds via an explicit pixel offset instead
    // of a binary left/right class flip. A flip alone assumes the OPPOSITE
    // anchor always fits — true on a wide page, false on a narrow sheet
    // box: flipping a wide tooltip to right-anchor can just as easily push
    // its now-leftward-growing box past the container's LEFT edge instead
    // (confirmed live: a Hunters Patterns tooltip inside a resized-narrow
    // Hunter Techniques box rendered right-anchored, correctly clear of the
    // right edge, but with its first several characters of every line
    // clipped off the LEFT by the same .sheet-box overflow:auto that
    // clipRect() accounts for). Left is reset to 0 first so repeated hovers
    // (a resize between them) always start from the CSS default rather than
    // compounding an old offset.
    content.style.left = "0px";
    const rect = content.getBoundingClientRect();
    let shift = 0;
    if (rect.right > bounds.right) shift = bounds.right - rect.right;
    if (rect.left + shift < bounds.left) shift = bounds.left - rect.left;
    content.style.left = shift + "px";

    // Vertical: still a binary flip (below instead of above) — a real
    // clamp would mean shrinking or scrolling the box's own content, out
    // of scope here. Measured after the horizontal shift above, since
    // shifting left/right can change the box's own top position if it
    // wraps to more lines at the narrower effective width.
    if (content.getBoundingClientRect().top < bounds.top) {
      content.classList.add("tooltip-content-below");
    }
  }
  document.addEventListener("mouseover", (e) => {
    const tooltip = e.target.closest(".tooltip");
    if (tooltip) positionTooltip(tooltip);
  });
  document.addEventListener("focusin", (e) => {
    const tooltip = e.target.closest(".tooltip");
    if (tooltip) positionTooltip(tooltip);
  });
})();
