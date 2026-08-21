// The character sheet's pocket calculator: four functions, one pending
// operation at a time, exactly like a physical desk calculator rather than
// an expression parser. Typing 2 + 3 + 4 evaluates the first + when the
// second is pressed, so the display reads 5 and then 9 — no precedence, no
// parentheses, nothing to parse.
//
// Everything is computed from numbers held in this closure. There is no
// eval() of the display string anywhere, and no server round trip: the
// calculator keeps no state worth persisting and none of its results feed
// any other part of the sheet.
(function () {
  const root = document.querySelector(".sheet-calculator");
  const display = document.getElementById("sheet-calc-display");
  if (!root || !display) return;

  let current = "0"; // what the display shows, as typed
  let accumulator = null; // the left-hand operand of a pending operation
  let pendingOp = null;
  let startFresh = false; // next digit replaces the display rather than appending
  let errored = false;

  // toLocaleString, unlike Number.prototype.toString, never falls back to
  // exponent notation — a result of a million reads "1000000", not "1e+6".
  // Grouping is off because the display doubles as the next operand's
  // starting value.
  function show(value) {
    display.value = typeof value === "string"
      ? value
      : value.toLocaleString("en-US", { maximumFractionDigits: 10, useGrouping: false });
  }

  function reset() {
    current = "0";
    accumulator = null;
    pendingOp = null;
    startFresh = false;
    errored = false;
    show("0");
  }

  function apply(a, op, b) {
    switch (op) {
      case "+": return a + b;
      case "-": return a - b;
      case "*": return a * b;
      case "/": return b === 0 ? NaN : a / b;
      default: return b;
    }
  }

  // Collapses any pending operation and leaves the result in the display.
  // Returns the numeric result, or null if the operation was undefined
  // (divide by zero), in which case the calculator is left in its error
  // state and needs a C before it will do anything else.
  function resolve() {
    const value = parseFloat(current);
    const result = pendingOp === null ? value : apply(accumulator, pendingOp, value);
    if (!isFinite(result)) {
      errored = true;
      show("Error");
      return null;
    }
    accumulator = result;
    current = String(result);
    show(result);
    return result;
  }

  function press(key) {
    // Once in the error state the only meaningful input is a reset —
    // otherwise a stray digit would silently resume from a nonsense value.
    if (errored && key !== "clear") return;

    if (key >= "0" && key <= "9") {
      current = startFresh || current === "0" ? key : current + key;
      startFresh = false;
      show(current);
      return;
    }

    switch (key) {
      case ".":
        if (startFresh) {
          current = "0.";
          startFresh = false;
        } else if (!current.includes(".")) {
          current += ".";
        }
        show(current);
        return;
      case "clear":
        reset();
        return;
      case "back":
        if (startFresh) return;
        current = current.length > 1 ? current.slice(0, -1) : "0";
        if (current === "-") current = "0";
        show(current);
        return;
      case "negate":
        current = current.startsWith("-") ? current.slice(1) : "-" + current;
        show(current);
        return;
      case "=":
        if (resolve() === null) return;
        pendingOp = null;
        startFresh = true;
        return;
      case "+":
      case "-":
      case "*":
      case "/":
        // Pressing an operator twice in a row just swaps which operator is
        // pending; it must not re-evaluate against the same operand.
        if (startFresh && pendingOp !== null) {
          pendingOp = key;
          return;
        }
        if (resolve() === null) return;
        pendingOp = key;
        startFresh = true;
        return;
      default:
        return;
    }
  }

  root.addEventListener("click", (e) => {
    const btn = e.target.closest("button[data-calc]");
    if (!btn) return;
    press(btn.dataset.calc);
  });

  // Keyboard entry, scoped to the calculator: the listener is on the
  // widget itself, not the document, so typing anywhere else on the sheet
  // (the chat box, the HP boxes, a Bio textarea) is untouched. The display
  // is a `readonly` <input> rather than an <output> specifically so it's
  // clickable/focusable — an <output> can never receive focus, so clicking
  // it and typing did nothing. `readonly` still blocks the browser's own
  // native text editing, so every digit still goes through press() below
  // and the "physical calculator, no eval()" model above holds.
  root.addEventListener("keydown", (e) => {
    if (e.ctrlKey || e.metaKey || e.altKey) return;
    const keys = {
      Enter: "=", "=": "=", Escape: "clear", Backspace: "back",
      "+": "+", "-": "-", "*": "*", "/": "/", ".": ".",
    };
    const key = (e.key >= "0" && e.key <= "9") ? e.key : keys[e.key];
    if (!key) return;
    e.preventDefault();
    press(key);
  });
})();
