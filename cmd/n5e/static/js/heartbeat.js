// Pings /heartbeat every 2 seconds while this page is open — the FALLBACK
// signal that a tab is still open. alive.js's held-open connection is the
// primary one now, and the server only falls back to counting silence here
// while no /alive stream has ever connected (a browser with EventSource
// unavailable or blocked). See alive.js for why a timer is the weaker
// signal, and server.go's watchHeartbeat for why neither can be a plain
// pagehide/unload listener: every internal nav link click unloads the page
// exactly the same way closing the tab does, so a naive "shut down on
// unload" would kill the server on every click.
//
// Browsers throttle setInterval in backgrounded tabs (down to roughly once
// a minute in Chrome once "intensive throttling" kicks in), so merely
// switching to another tab for a while can silently starve this ping —
// heartbeatTimeout on the server is set generously above that for exactly
// this reason (see its doc comment for the real bug report this caused).
// As extra insurance, fire an immediate ping the moment this tab becomes
// visible again rather than waiting on a possibly-stale throttled interval
// to get around to it.
(function () {
  function ping() {
    fetch("/heartbeat", { method: "POST" }).catch(() => {
      // A single failed ping is fine — the server-side watchdog only acts
      // after a long stretch of continuous silence, not one dropped beat.
    });
  }
  ping();
  setInterval(ping, 2000);
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "visible") ping();
  });
})();
