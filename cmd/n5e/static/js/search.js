// Site-wide search: fetches the name index once (lazily, on first use) and
// filters it client-side as the user types. Names-only over ~1800 rows is
// small enough that this stays instant without hitting the server per
// keystroke — see the design note in cmd/n5e/search.go.
(function () {
  const input = document.getElementById("site-search");
  const results = document.getElementById("site-search-results");
  const tooltip = document.getElementById("search-tooltip");
  if (!input || !results) return;

  const MAX_RESULTS = 15;

  let index = null;
  let indexPromise = null;
  let activeIndex = -1;

  // A single shared floating preview element, positioned via JS rather than
  // the usual CSS :hover chain — .search-results scrolls (overflow-y: auto),
  // which would clip a nested absolutely-positioned tooltip. See the CSS
  // comment on #search-tooltip in app.css.
  function showTooltip(li) {
    const text = li && li.dataset.preview;
    if (!tooltip || !text) return;
    tooltip.textContent = text;
    tooltip.hidden = false;
    const dropdownRect = results.getBoundingClientRect();
    const rowRect = li.getBoundingClientRect();
    const tw = tooltip.offsetWidth;
    // Prefer to the left of the dropdown (it's right-aligned under the nav
    // search box); fall back to the right if that would run off-screen.
    let left = dropdownRect.left - tw - 8;
    if (left < 8) left = Math.min(dropdownRect.right + 8, window.innerWidth - tw - 8);
    tooltip.style.left = `${Math.max(8, left)}px`;
    tooltip.style.top = `${rowRect.top}px`;
  }

  function hideTooltip() {
    if (tooltip) tooltip.hidden = true;
  }

  function loadIndex() {
    if (!indexPromise) {
      indexPromise = fetch("/search/index.json")
        .then((r) => r.json())
        .then((data) => {
          index = data;
          return data;
        });
    }
    return indexPromise;
  }

  function render(matches) {
    results.innerHTML = "";
    activeIndex = -1;
    hideTooltip();
    if (matches.length === 0) {
      results.hidden = true;
      return;
    }
    for (const m of matches) {
      const li = document.createElement("li");
      if (m.preview) li.dataset.preview = m.preview;
      const a = document.createElement("a");
      a.href = m.url;
      a.textContent = m.name;
      const type = document.createElement("span");
      type.className = "search-result-type";
      type.textContent = m.type;
      a.appendChild(type);
      li.appendChild(a);
      results.appendChild(li);
    }
    results.hidden = false;
  }

  function search(query) {
    const q = query.trim().toLowerCase();
    if (q === "" || !index) {
      render([]);
      return;
    }
    const matches = [];
    for (const entry of index) {
      if (entry.name.toLowerCase().includes(q)) {
        matches.push(entry);
        if (matches.length >= MAX_RESULTS) break;
      }
    }
    render(matches);
  }

  input.addEventListener("focus", () => {
    loadIndex().then(() => search(input.value));
  });

  input.addEventListener("input", () => {
    if (index) {
      search(input.value);
    } else {
      loadIndex().then(() => search(input.value));
    }
  });

  input.addEventListener("keydown", (e) => {
    const items = results.querySelectorAll("li");
    if (items.length === 0) return;

    if (e.key === "ArrowDown") {
      e.preventDefault();
      activeIndex = Math.min(activeIndex + 1, items.length - 1);
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      activeIndex = Math.max(activeIndex - 1, 0);
    } else if (e.key === "Enter") {
      if (activeIndex >= 0) {
        e.preventDefault();
        items[activeIndex].querySelector("a").click();
      }
      return;
    } else if (e.key === "Escape") {
      render([]);
      input.blur();
      return;
    } else {
      return;
    }

    items.forEach((li, i) => li.classList.toggle("active", i === activeIndex));
    items[activeIndex].scrollIntoView({ block: "nearest" });
    showTooltip(items[activeIndex]);
  });

  // Rollover preview on hover/keyboard focus — click-to-navigate is
  // untouched, it's still a plain <a href> inside each row.
  //
  // The tooltip is a DOM sibling of #site-search-results, not a child, so
  // "still hovering the search UI" has to mean "inside results OR inside the
  // tooltip itself" — checking only `results.contains()` hides the tooltip
  // the moment the cursor crosses from a row onto the rendered tooltip.
  function insideSearchUI(el) {
    return !!el && (results.contains(el) || (tooltip && tooltip.contains(el)));
  }
  const maybeHideTooltip = (e) => {
    if (!insideSearchUI(e.relatedTarget)) hideTooltip();
  };
  results.addEventListener("mouseover", (e) => {
    const li = e.target.closest("li");
    if (li) showTooltip(li);
  });
  results.addEventListener("mouseout", maybeHideTooltip);
  if (tooltip) tooltip.addEventListener("mouseout", maybeHideTooltip);
  results.addEventListener("focusin", (e) => {
    const li = e.target.closest("li");
    if (li) showTooltip(li);
  });
  results.addEventListener("focusout", hideTooltip);

  document.addEventListener("click", (e) => {
    if (!e.target.closest(".nav-search")) {
      render([]);
    }
  });
})();
