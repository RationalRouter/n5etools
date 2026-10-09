// Nav bar dropdown menus (layout.html's "Player References" / "Dungeon
// Master Tools" — see .nav-dropdown/.nav-dropdown-toggle/.nav-dropdown-panel
// in app.css). Same click-to-toggle/outside-click/Escape-to-close shape as
// lang-switcher.js's #lang-switcher, generalized over every .nav-dropdown
// on the page instead of one fixed element, so a future third menu needs no
// script changes.
//
// The nav itself is never swapped via outerHTML (it's outside every
// fragment-swappable container on every page), so plain load-time listeners
// are enough here — no delegation or window.n5eRewireControls() call
// needed, unlike controls living inside the character sheet's fragments.
(function () {
  const dropdowns = document.querySelectorAll(".nav-dropdown");
  if (!dropdowns.length) return;

  function setOpen(dropdown, open) {
    const toggle = dropdown.querySelector(".nav-dropdown-toggle");
    dropdown.classList.toggle("nav-dropdown-open", open);
    if (toggle) toggle.setAttribute("aria-expanded", open ? "true" : "false");
  }

  function closeAll(except) {
    for (const d of dropdowns) {
      if (d !== except) setOpen(d, false);
    }
  }

  for (const dropdown of dropdowns) {
    const toggle = dropdown.querySelector(".nav-dropdown-toggle");
    if (!toggle) continue;
    toggle.addEventListener("click", () => {
      const willOpen = !dropdown.classList.contains("nav-dropdown-open");
      closeAll(dropdown);
      setOpen(dropdown, willOpen);
    });
  }

  document.addEventListener("click", (e) => {
    for (const dropdown of dropdowns) {
      if (dropdown.classList.contains("nav-dropdown-open") && !dropdown.contains(e.target)) {
        setOpen(dropdown, false);
      }
    }
  });

  document.addEventListener("keydown", (e) => {
    if (e.key !== "Escape") return;
    for (const dropdown of dropdowns) {
      if (dropdown.classList.contains("nav-dropdown-open")) {
        setOpen(dropdown, false);
        dropdown.querySelector(".nav-dropdown-toggle").focus();
      }
    }
  });
})();
