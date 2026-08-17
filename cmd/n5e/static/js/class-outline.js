// Classes page: builds a left-hand "Outline" sidebar (5etools-style) that
// mirrors every visible h2/h3 heading in plain reading order — h3's nest
// under whichever h2 precedes them on the page, exactly like the on-page
// content itself (general and subclass features interleave by level under
// one "Class Features" heading; each subclass's own name/description and
// option lists appear wherever they sit). Subclass-scoped headings render
// in the same blue used everywhere else on the page (see .subclass-scope
// in app.css) so they're visually distinct without needing their own
// grouping — the on-page heading text already disambiguates which
// subclass a feature belongs to when more than one is selected (see
// "{{.SubclassName}}: Level N: {{.Name}}" in class_detail.html).
//
// Rebuilds whenever a subclass tile toggles content visibility (via a
// MutationObserver watching `hidden` attribute changes — see
// subclass-tabs.js), so newly-revealed headings appear immediately and
// deselected ones disappear.
//
// Purely progressive enhancement: with JS disabled there's no outline
// (the empty <nav> stays empty), and the page still reads top to bottom
// normally. Harmless no-op on every other page (no .class-detail there).
(function () {
  const detail = document.querySelector(".class-detail");
  const list = document.getElementById("class-outline-list");
  if (!detail || !list) return;

  let idCounter = 0;

  function isVisible(el) {
    return el.offsetParent !== null;
  }

  function ensureId(h) {
    if (!h.id) {
      idCounter += 1;
      h.id = "outline-" + idCounter;
    }
    return h.id;
  }

  function scrollToHeading(h) {
    h.scrollIntoView({ behavior: "smooth", block: "start" });
    history.replaceState(null, "", "#" + h.id);
  }

  function makeLink(h) {
    const li = document.createElement("li");
    const a = document.createElement("a");
    a.href = "#" + ensureId(h);
    a.textContent = h.textContent;
    if (h.closest(".subclass-scope")) a.classList.add("outline-subclass");
    a.addEventListener("click", (e) => {
      e.preventDefault();
      scrollToHeading(h);
    });
    li.appendChild(a);
    return li;
  }

  function rebuild() {
    list.innerHTML = "";
    let currentUl = null;

    detail.querySelectorAll("h2, h3").forEach((h) => {
      if (!isVisible(h)) return;

      if (h.tagName === "H2") {
        const li = makeLink(h);
        list.appendChild(li);
        const ul = document.createElement("ul");
        ul.className = "outline-sub";
        li.appendChild(ul);
        currentUl = ul;
        return;
      }
      (currentUl || list).appendChild(makeLink(h));
    });
  }

  rebuild();

  new MutationObserver(rebuild).observe(detail, {
    attributes: true,
    attributeFilter: ["hidden"],
    subtree: true,
  });
})();
