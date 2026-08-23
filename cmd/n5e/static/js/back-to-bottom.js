// Sitewide "scroll to bottom" button — sits next to #back-to-top
// (back-to-top.js's own doc comment explains why that one is sitewide
// rather than page-by-page; the same reasoning applies here) so a long
// page can be jumped to either end. Hidden once there's nothing left below
// to scroll to, the same way #back-to-top hides once already at the top.
(function () {
  const button = document.getElementById("back-to-bottom");
  if (!button) return;

  const HIDE_WITHIN_PX = 600;

  function distanceToBottom() {
    return document.documentElement.scrollHeight - window.innerHeight - window.scrollY;
  }

  function updateVisibility() {
    button.classList.toggle("back-to-bottom-visible", distanceToBottom() > HIDE_WITHIN_PX);
  }
  window.addEventListener("scroll", updateVisibility, { passive: true });
  window.addEventListener("resize", updateVisibility, { passive: true });
  updateVisibility();

  button.addEventListener("click", () => {
    window.scrollTo({ top: document.documentElement.scrollHeight, behavior: "smooth" });
  });
})();
