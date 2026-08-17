// Holds one connection open to /alive for as long as this page is open.
//
// This is the server's primary "a tab is still here" signal, and it replaces
// nothing less than the reason the app used to shut itself down while the
// user was still using it. The old signal was heartbeat.js's timer: ping
// every 2 seconds, and the server exits after a long enough silence. Timers
// are the wrong instrument for the job — a browser throttles them in a
// backgrounded tab, stops them outright in a frozen one, and a suspended or
// hibernated machine runs none at all. Every one of those looks identical to
// a closed tab from the server's side, which is how sleeping the computer
// with the tab still open ended up killing the server (part-6 report).
//
// A connection has none of those failure modes. It is closed by the browser,
// promptly and unconditionally, when the page goes away — so a real tab close
// is noticed in milliseconds. And it survives suspend, hibernate and tab
// freezing untouched, because none of those close sockets. The server watches
// the connection instead of counting silence, so it can be both quicker to
// exit on a genuine close and immune to the sleep case.
//
// EventSource rather than fetch() or WebSocket: it reconnects on its own
// after a drop (which is what makes a resumed machine or a dropped socket
// self-heal with no code here), it needs no protocol upgrade, and it sends
// the session cookie like any other same-origin GET. heartbeat.js stays as
// the fallback for a browser where this never connects at all; the server
// only falls back to the ping budget while it has never seen an /alive
// stream.
(function () {
  if (typeof EventSource !== "function") return;

  // No message handler: the server never sends events, and it isn't meant
  // to. The open connection IS the signal. onerror is left to EventSource's
  // own automatic reconnection rather than being turned into a reload —
  // during a graceful shutdown the stream drops too, and reloading then
  // would only produce a connection-refused error page.
  const stream = new EventSource("/alive");
  stream.onerror = () => {};

  // Closing explicitly on pagehide is belt and braces: the browser closes
  // the socket when the page is discarded anyway, but doing it here makes
  // the FIN land at the start of a navigation rather than whenever the old
  // page's resources happen to be torn down, which keeps the server's
  // no-streams-open gap comfortably inside its grace window.
  window.addEventListener("pagehide", () => stream.close());
})();
