// The weapon/armor sub-filters shared by the /items library and the character
// sheet's Inventory library pane: Simple/Martial/Exotic beside "Weapons",
// Light/Medium/Heavy beside "Armor".
//
// A factory rather than an IIFE per page, for the same reason jutsu-filter.js
// became one — the two lists have to agree on the buckets and their order, and
// two copies would drift the first time a category is added.
//
// It deliberately does NOT set row.hidden itself. Each host list already has a
// single apply() pass that owns visibility (sheet-library.js's comment spells
// out why: two listeners independently writing row.hidden fight, and whichever
// ran last wins). So this returns a predicate the host folds into its own pass,
// and calls cfg.onChange when a box is ticked so the host re-runs it.
//
// Which rows a group judges comes from the presence of the data- attribute, not
// from the row's kind label — the sheet pane's data-kind is a display label
// ("Weapons", "Poisons") while /items' is a raw kind ("weapon"), and only the
// server knows which rows are really weapons. A row with no data-weapon-category
// is invisible to the weapon group entirely.
//
//   cfg.container  element the checkbox rows are appended to; hidden when no
//                  group applies (the feats pane, which shares the template)
//   cfg.rows       the row elements to derive buckets from
//   cfg.onChange   called after any box is toggled
//   cfg.initial    previously saved state() output, or null
(function () {
  const GROUPS = [
    {
      attr: "weaponCategory",
      label: "Weapons",
      order: ["simple", "martial", "exotic", "other"],
      labels: { simple: "Simple", martial: "Martial", exotic: "Exotic", other: "Other" },
    },
    {
      attr: "armorCategory",
      label: "Armor",
      order: ["light", "medium", "heavy", "other"],
      labels: { light: "Light", medium: "Medium", heavy: "Heavy", other: "Other" },
    },
  ];

  // A row of the right kind whose category the rules don't record is stamped
  // "other" server-side, but an empty attribute value is normalised the same
  // way here so a future template that emits a bare attribute can't produce a
  // nameless checkbox.
  function bucketOf(row, attr) {
    return row.dataset[attr] || "other";
  }

  window.n5eEquipmentSubfilters = function (cfg) {
    const container = cfg.container;
    const rows = cfg.rows ? [...cfg.rows] : [];
    // Every host stays working when its pane has no equipment in it at all:
    // the predicate passes everything and state() round-trips as empty.
    const inert = { matches: () => true, state: () => ({}) };
    if (!container) return inert;
    if (rows.length === 0) {
      container.hidden = true;
      return inert;
    }

    // attr -> Set of ticked buckets, for the groups that ended up on screen.
    const active = new Map();
    // attr -> Set of unticked buckets, which is what state() persists. Storing
    // what is OFF rather than what is ON is what lets a bucket introduced by a
    // later rules update default to visible instead of arriving pre-hidden
    // with no box ever having been unticked.
    const off = new Map();

    for (const group of GROUPS) {
      const present = new Set();
      for (const row of rows) {
        if (group.attr in row.dataset) present.add(bucketOf(row, group.attr));
      }
      // One bucket can only hide everything or nothing — a checkbox that says
      // nothing about the list is noise, so the group is skipped.
      if (present.size < 2) continue;

      const values = [...present].sort((a, b) => {
        const ia = group.order.indexOf(a);
        const ib = group.order.indexOf(b);
        // Anything the rules add later that this file hasn't been taught
        // sorts after the known buckets, alphabetically among itself.
        if (ia !== ib) return (ia < 0 ? group.order.length : ia) - (ib < 0 ? group.order.length : ib);
        return a.localeCompare(b);
      });

      const savedOff = cfg.initial && Array.isArray(cfg.initial[group.attr])
        ? new Set(cfg.initial[group.attr])
        : new Set();
      const checked = new Set(values.filter((v) => !savedOff.has(v)));
      const unchecked = new Set(values.filter((v) => savedOff.has(v)));
      active.set(group.attr, checked);
      off.set(group.attr, unchecked);

      const wrap = document.createElement("div");
      wrap.className = "items-subfilter-group";
      const heading = document.createElement("span");
      heading.className = "items-subfilter-label";
      heading.textContent = group.label;
      wrap.appendChild(heading);
      // The boxes get their own element rather than sitting beside the
      // heading in one flex row: in a narrow pane the row wraps, and without
      // this a wrapped checkbox starts back under the heading instead of
      // lining up with the ones above it.
      const boxes = document.createElement("div");
      boxes.className = "items-kind-filters items-subfilter-boxes";
      wrap.appendChild(boxes);

      for (const value of values) {
        const label = document.createElement("label");
        const cb = document.createElement("input");
        cb.type = "checkbox";
        cb.checked = checked.has(value);
        cb.addEventListener("change", () => {
          if (cb.checked) {
            checked.add(value);
            unchecked.delete(value);
          } else {
            checked.delete(value);
            unchecked.add(value);
          }
          if (cfg.onChange) cfg.onChange();
        });
        label.appendChild(cb);
        label.append(" " + (group.labels[value] || value.charAt(0).toUpperCase() + value.slice(1)));
        boxes.appendChild(label);
      }
      container.appendChild(wrap);
    }

    if (active.size === 0) {
      container.hidden = true;
      return inert;
    }

    return {
      matches(row) {
        for (const [attr, checked] of active) {
          if (!(attr in row.dataset)) continue;
          if (!checked.has(bucketOf(row, attr))) return false;
        }
        return true;
      },
      // {weaponCategory: ["exotic"], …} — the UNTICKED buckets; see `off`.
      state() {
        const out = {};
        for (const [attr, unchecked] of off) out[attr] = [...unchecked];
        return out;
      },
    };
  };
})();
