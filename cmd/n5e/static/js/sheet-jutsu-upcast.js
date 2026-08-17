// Rank selector next to a jutsu's "Cast" button (character_sheet.html's
// "Attacks & Jutsu" table). All the rank/cost math already happened in Go
// (cmd/n5e/characters.go's buildUpcastOptions) — each <option>'s value is
// already the exact chakra cost for that rank, so this file only has to
// copy the selected option's value onto the paired .sheet-cast-btn's
// data-value and label, plus its data-rank (each <option> also carries
// data-rank, so the cast endpoint knows which rank was actually cast at —
// needed to start concentration tracking at the right rank). The actual
// spend still goes through sheet-vitals.js's existing
// wireCastButtons/.sheet-cast-btn path unchanged — no new endpoint, no new
// fetch.
//
// A single document-level delegated listener, not a per-element one bound
// during a wireAll() pass: any control living inside a container another
// sheet action can swap via outerHTML must survive that swap, and
// delegation from document is the simplest way to guarantee that without
// having to remember to call window.n5eRewireControls() from yet another
// call site.
(function () {
  document.addEventListener("change", (e) => {
    if (!e.target.matches(".sheet-upcast-rank")) return;
    const btn = document.getElementById(e.target.dataset.castTarget);
    if (!btn) return;

    const cost = e.target.value;
    const name = btn.dataset.name || "";
    const rank = e.target.selectedOptions[0] ? e.target.selectedOptions[0].dataset.rank : undefined;
    btn.dataset.value = "-" + cost;
    if (rank) btn.dataset.rank = rank;
    btn.textContent = "Cast " + cost + "⚡";
    btn.setAttribute("aria-label", "Cast " + name + ", spending " + cost + " chakra");
    btn.setAttribute("title", "Spend " + cost + " chakra");
  });
})();
