// Character sheet Core/Bio/Jutsu/Inventory tabs: pure client-side
// show/hide over server-rendered <section data-tab-panel="..."> blocks,
// same "toggle a hidden attribute on a data-tagged block" idea
// subclass-tabs.js already uses elsewhere — kept as its own small file
// since the DOM shape/purpose here differs (top-level page tabs, not an
// inline subclass picker).
//
// The open tab is remembered per character in sessionStorage. Some sheet
// edits (equipping an item, changing an ability score) have effects spread
// across the whole page and can only be applied by reloading it; without
// this, every one of those would dump the player back on the Core tab,
// which feels like the app losing their place. sessionStorage rather than
// localStorage so it survives the session's navigation without permanently
// pinning which tab a character opens on.
(function () {
  const buttons = document.querySelectorAll(".sheet-tab-btn");
  const panels = document.querySelectorAll("[data-tab-panel]");
  if (!buttons.length || !panels.length) return;

  const chat = document.querySelector(".sheet-chat-panel");
  const storageKey = "n5e:sheet-tab:" + (chat ? chat.dataset.characterId : "unknown");

  function show(tab, remember) {
    let matched = false;
    panels.forEach((p) => {
      const isTab = p.dataset.tabPanel === tab;
      p.hidden = !isTab;
      if (isTab) matched = true;
    });
    if (!matched) return false;
    buttons.forEach((b) => b.classList.toggle("active", b.dataset.tab === tab));
    if (remember) {
      try {
        sessionStorage.setItem(storageKey, tab);
      } catch (err) {
        // Private-browsing modes can refuse to write. Forgetting which tab
        // was open is not worth breaking the tabs over.
      }
    }
    return true;
  }

  buttons.forEach((btn) => {
    btn.addEventListener("click", () => show(btn.dataset.tab, true));
  });

  let saved = null;
  try {
    saved = sessionStorage.getItem(storageKey);
  } catch (err) {
    saved = null;
  }
  // A remembered tab that no longer exists (renamed or removed between
  // sessions) leaves show() returning false and the server-rendered
  // default standing.
  if (saved) show(saved, false);
})();
