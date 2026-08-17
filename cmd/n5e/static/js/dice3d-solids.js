// Dice geometry: the polyhedra behind d4-d100, generated procedurally.
//
// Pure maths and one 2D canvas for the numeral atlas — no WebGL, no page DOM,
// no knowledge of the dice tray. static/js/dice3d.js consumes this and does
// all the rendering and physics; keeping the two apart means a shape bug can
// be reasoned about (and logged) without a GPU anywhere in the picture, which
// is most of the reason this is its own file.
//
// Nothing here is a hand-typed vertex/face table. Each solid is defined by its
// handful of generating vertices (the ones you can write from memory and check
// by eye) and the faces are then *derived* by a brute-force supporting-plane
// hull: for every triple of vertices, if every other vertex lies on one side
// of their plane, that plane is a face. It is O(n^4) and it runs once, on a
// 20-vertex solid, at startup. A hand-typed list of twelve pentagons for the
// dodecahedron is the kind of thing that is wrong in exactly one place and
// looks fine until a specific number never comes up.
//
// Conventions used throughout, and shared with dice3d.js:
//   * Z is up. The table is the plane z = 0 and gravity points at -z.
//   * Every solid is centred on the origin and scaled to circumradius 1.
//     dice3d.js multiplies by the pixel size it wants.
//   * A die's value is decided by which "rest axis" points up. For most dice
//     that is a face normal (the face on top). For the d4 it is a *vertex*
//     direction, because a resting tetrahedron has a face down and a point up
//     — see the d4 notes below.
(function () {
  "use strict";

  const PHI = (1 + Math.sqrt(5)) / 2;
  const EPS = 1e-6;

  // Plain-array vec3 helpers. This module runs once at startup and allocates
  // a few thousand short-lived arrays doing it; clarity is worth more here
  // than the allocation-free style dice3d.js's per-frame code uses.
  const sub = (a, b) => [a[0] - b[0], a[1] - b[1], a[2] - b[2]];
  const addv = (a, b) => [a[0] + b[0], a[1] + b[1], a[2] + b[2]];
  const mul = (a, s) => [a[0] * s, a[1] * s, a[2] * s];
  const dot = (a, b) => a[0] * b[0] + a[1] * b[1] + a[2] * b[2];
  const cross = (a, b) => [
    a[1] * b[2] - a[2] * b[1],
    a[2] * b[0] - a[0] * b[2],
    a[0] * b[1] - a[1] * b[0],
  ];
  const length = (a) => Math.sqrt(dot(a, a));
  const norm = (a) => mul(a, 1 / length(a));

  // --- Generating vertex sets ---------------------------------------------

  function tetrahedron() {
    // Four alternating corners of a cube — the standard way to get a regular
    // tetrahedron without writing down any irrational coordinates.
    return [[1, 1, 1], [1, -1, -1], [-1, 1, -1], [-1, -1, 1]];
  }

  function cube() {
    const v = [];
    for (const x of [-1, 1]) for (const y of [-1, 1]) for (const z of [-1, 1]) v.push([x, y, z]);
    return v;
  }

  function octahedron() {
    return [[1, 0, 0], [-1, 0, 0], [0, 1, 0], [0, -1, 0], [0, 0, 1], [0, 0, -1]];
  }

  function dodecahedron() {
    // The eight cube corners plus three cyclic families of golden-ratio
    // rectangles.
    const v = cube();
    const a = 1 / PHI;
    for (const s1 of [-1, 1]) for (const s2 of [-1, 1]) {
      v.push([0, s1 * a, s2 * PHI]);
      v.push([s1 * a, s2 * PHI, 0]);
      v.push([s2 * PHI, 0, s1 * a]);
    }
    return v;
  }

  function icosahedron() {
    // Three mutually perpendicular golden-ratio rectangles.
    const v = [];
    for (const s1 of [-1, 1]) for (const s2 of [-1, 1]) {
      v.push([0, s1, s2 * PHI]);
      v.push([s1, s2 * PHI, 0]);
      v.push([s2 * PHI, 0, s1]);
    }
    return v;
  }

  function trapezohedron() {
    // Pentagonal trapezohedron — the d10/d100 shape. Two apexes at (0,0,±h)
    // and two rings of five, offset 36° from each other, at z = ±c.
    //
    // The ten kite faces are only planar for one particular h/c ratio. Taking
    // the face through the top apex, two adjacent upper-ring points and the
    // lower-ring point between them, and projecting onto the vertical plane
    // that bisects it, the three distinct radii/heights (0,h), (cos36°,c) and
    // (1,-c) have to be collinear. Solving that gives
    //     h/c = (1 + cos36°) / (1 - cos36°) = 5 + 2√5 ≈ 9.472
    // Fixing h = 1 (so the apexes sit at circumradius 1) leaves c below.
    // Get this wrong and the faces are subtly warped: the hull search below
    // finds triangles instead of kites and the die comes out with 20 faces.
    const c = 1 / (5 + 2 * Math.sqrt(5));
    const v = [[0, 0, 1], [0, 0, -1]];
    for (let i = 0; i < 5; i++) {
      const upper = (i * 72) * Math.PI / 180;
      const lower = (i * 72 + 36) * Math.PI / 180;
      v.push([Math.cos(upper), Math.sin(upper), c]);
      v.push([Math.cos(lower), Math.sin(lower), -c]);
    }
    return v;
  }

  // --- Face extraction -----------------------------------------------------

  // Every face plane of a convex solid centred on the origin is a supporting
  // plane: some triple of vertices spans it, and no vertex lies outside it.
  // Returns faces as ordered vertex loops, counter-clockwise seen from
  // outside, each with its centroid and outward normal.
  function hullFaces(verts) {
    const planes = [];
    const n = verts.length;
    for (let i = 0; i < n; i++) {
      for (let j = i + 1; j < n; j++) {
        for (let k = j + 1; k < n; k++) {
          const e1 = sub(verts[j], verts[i]);
          const e2 = sub(verts[k], verts[i]);
          const c = cross(e1, e2);
          const clen = length(c);
          if (clen < EPS) continue; // collinear triple, no plane
          let normal = mul(c, 1 / clen);
          let d = dot(normal, verts[i]);
          // Orient outward. Every solid here is centred on the origin, so the
          // outward normal is whichever of the two gives a positive offset.
          if (d < 0) { normal = mul(normal, -1); d = -d; }
          if (d < EPS) continue; // plane through the centre; never a face

          let supporting = true;
          for (let m = 0; m < n; m++) {
            if (dot(normal, verts[m]) > d + 1e-5) { supporting = false; break; }
          }
          if (!supporting) continue;
          if (planes.some((p) => dot(p.normal, normal) > 1 - 1e-5 && Math.abs(p.d - d) < 1e-5)) continue;
          planes.push({ normal, d });
        }
      }
    }

    return planes.map((plane) => {
      const on = [];
      for (let m = 0; m < verts.length; m++) {
        if (Math.abs(dot(plane.normal, verts[m]) - plane.d) < 1e-5) on.push(m);
      }
      let centroid = [0, 0, 0];
      for (const idx of on) centroid = addv(centroid, verts[idx]);
      centroid = mul(centroid, 1 / on.length);

      // Order the loop by angle in the face plane. (u, w, normal) is
      // right-handed, so increasing angle is counter-clockwise seen from
      // outside and a centroid fan comes out front-facing.
      const u = norm(sub(verts[on[0]], centroid));
      const w = cross(plane.normal, u);
      on.sort((a, b) => {
        const pa = sub(verts[a], centroid);
        const pb = sub(verts[b], centroid);
        return Math.atan2(dot(pa, w), dot(pa, u)) - Math.atan2(dot(pb, w), dot(pb, u));
      });

      return { indices: on, centroid, normal: plane.normal, offset: plane.d };
    });
  }

  // Largest circle that fits in a face, measured centroid-to-edge rather than
  // centroid-to-corner — the corner distance would let a numeral overhang the
  // edges of a triangle badly.
  function faceInradius(face, verts) {
    let best = Infinity;
    const loop = face.indices;
    for (let i = 0; i < loop.length; i++) {
      const a = verts[loop[i]];
      const b = verts[loop[(i + 1) % loop.length]];
      const mid = mul(addv(a, b), 0.5);
      best = Math.min(best, length(sub(mid, face.centroid)));
    }
    return best;
  }

  // --- Numbering -----------------------------------------------------------

  // Faces are numbered in antipodal pairs so that opposite faces sum to a
  // constant, the way real dice are made (1 opposite 20, 2 opposite 19, ...).
  // It costs nothing and it is immediately obvious to anyone who picks the
  // die up on screen and looks at it.
  function pairFaces(faces) {
    const used = new Array(faces.length).fill(false);
    const pairs = [];
    for (let i = 0; i < faces.length; i++) {
      if (used[i]) continue;
      let best = -1;
      let bestDot = Infinity;
      for (let j = 0; j < faces.length; j++) {
        if (j === i || used[j]) continue;
        const d = dot(faces[i].normal, faces[j].normal);
        if (d < bestDot) { bestDot = d; best = j; }
      }
      used[i] = true;
      if (best >= 0) used[best] = true;
      pairs.push([i, best]);
    }
    return pairs;
  }

  // spec.values[i] is the number the die reports; spec.glyphs[i] is what gets
  // painted on the face. They differ only for the d100 pair, where the tens
  // die reports 0-9 but shows 00-90.
  function assignFaceNumbers(faces, spec) {
    const out = new Array(faces.length).fill(null);
    const pairs = pairFaces(faces);
    pairs.forEach(([i, j], p) => {
      out[i] = p;
      if (j >= 0) out[j] = spec.values.length - 1 - p;
    });
    faces.forEach((face, i) => {
      face.value = spec.values[out[i]];
      face.glyph = spec.glyphs[out[i]];
    });
  }

  function standardSpec(sides) {
    const values = [];
    const glyphs = [];
    for (let i = 1; i <= sides; i++) { values.push(i); glyphs.push(String(i)); }
    return { values, glyphs };
  }

  // The d100 is thrown as a real percentile pair: a tens die reading 00-90 and
  // a units die reading 0-9, where 00 + 0 is 100. dice3d.js splits one drawn
  // 1-100 value across the two and the roller still reports a single number,
  // so nothing downstream (sheet-chat.js's notation, the tray breakdown) sees
  // a difference.
  function tensSpec() {
    const values = [];
    const glyphs = [];
    for (let i = 0; i <= 9; i++) { values.push(i); glyphs.push(i === 0 ? "00" : String(i * 10)); }
    return { values, glyphs };
  }

  function unitsSpec() {
    const values = [];
    const glyphs = [];
    for (let i = 0; i <= 9; i++) { values.push(i); glyphs.push(String(i)); }
    return { values, glyphs };
  }

  // --- Rotation symmetries -------------------------------------------------

  // Every rotation that maps the solid onto itself, expressed as a permutation
  // of the rest axes: perm[i] = j means the rotation carries axis i onto axis j.
  //
  // This is what lets dice3d.js correct a die's showing number by RE-LABELLING
  // it instead of rotating it. Rotating a settled die is visible and reads as
  // cheating; swapping which numeral sits on which face is a texture change with
  // no motion at all, and because the swap is a symmetry of the solid the result
  // is still a legitimately numbered die (opposite faces still sum to a
  // constant). The group is transitive on faces for every solid here, so a
  // permutation carrying any given face to any other always exists.
  //
  // Enumeration: a rotation is pinned down by where it sends two non-parallel
  // axes. Take axes[0] and the first axis not parallel to it as a reference
  // pair; every candidate is then some ordered pair (a, b) subtending the same
  // angle. Build the rotation those two correspondences define and keep it only
  // if it permutes the whole axis set. O(n^4) worst case on n <= 20 axes, once,
  // at startup — the same budget hullFaces already spends.
  function rotationSymmetries(axes) {
    const n = axes.length;
    let k = 1;
    while (k < n && Math.abs(dot(axes[0], axes[k])) > 1 - 1e-6) k++;
    const identity = axes.map((_, i) => i);
    if (k >= n) return [identity];

    // Orthonormal right-handed frame from an axis and a second, non-parallel one.
    const frame = (u, w) => {
      const w2 = norm(sub(w, mul(u, dot(u, w))));
      return [u, w2, cross(u, w2)];
    };
    const ref = frame(axes[0], axes[k]);
    const refDot = dot(axes[0], axes[k]);

    const out = [];
    const seen = new Set();
    for (let a = 0; a < n; a++) {
      for (let b = 0; b < n; b++) {
        if (a === b) continue;
        if (Math.abs(dot(axes[a], axes[b]) - refDot) > 1e-6) continue;
        const img = frame(axes[a], axes[b]);
        // R(x) = sum over the frame of img[i] * (ref[i] . x). Both frames are
        // right-handed, so R is a rotation and never a reflection.
        const R = (x) => addv(
          addv(mul(img[0], dot(ref[0], x)), mul(img[1], dot(ref[1], x))),
          mul(img[2], dot(ref[2], x)),
        );

        const perm = new Array(n).fill(-1);
        let ok = true;
        for (let i = 0; i < n && ok; i++) {
          const r = R(axes[i]);
          let found = -1;
          for (let j = 0; j < n; j++) {
            if (dot(r, axes[j]) > 1 - 1e-5) { found = j; break; }
          }
          if (found < 0) ok = false;
          else perm[i] = found;
        }
        if (!ok) continue;
        const key = perm.join(",");
        if (seen.has(key)) continue;
        seen.add(key);
        out.push(perm);
      }
    }
    return out.length ? out : [identity];
  }

  // --- Mesh building -------------------------------------------------------

  // Face geometry is emitted in two rings so the shader can shade a bevel:
  // an inner fan (edge = 0) covering most of the face flat, and a skirt of
  // quads bridging the inner ring out to the true silhouette (edge 0 -> 1).
  // Darkening by `edge` in the fragment shader reads as the rounded, slightly
  // occluded rim a real die has. The outer ring sits on the exact polygon, so
  // the silhouette stays a true polyhedron.
  const BEVEL_INSET = 0.86;

  function buildBodyMesh(verts, faces) {
    const positions = [];
    const normals = [];
    const edges = [];

    function push(p, n, e) {
      positions.push(p[0], p[1], p[2]);
      normals.push(n[0], n[1], n[2]);
      edges.push(e);
    }

    for (const face of faces) {
      const loop = face.indices.map((i) => verts[i]);
      const inner = loop.map((p) => addv(face.centroid, mul(sub(p, face.centroid), BEVEL_INSET)));
      const n = face.normal;
      for (let i = 0; i < loop.length; i++) {
        const j = (i + 1) % loop.length;
        // Inner fan.
        push(face.centroid, n, 0);
        push(inner[i], n, 0);
        push(inner[j], n, 0);
        // Skirt out to the real edge.
        push(inner[i], n, 0);
        push(loop[i], n, 1);
        push(loop[j], n, 1);
        push(inner[i], n, 0);
        push(loop[j], n, 1);
        push(inner[j], n, 0);
      }
    }

    return {
      positions: new Float32Array(positions),
      normals: new Float32Array(normals),
      edges: new Float32Array(edges),
      count: positions.length / 3,
    };
  }

  // Numerals are their own geometry — small quads floating just off each face
  // — rather than UVs baked into the face triangles. That is what makes the
  // d4 tractable (three numerals per face, each rotated to its own corner)
  // without a second triangulation path, and it means the bevel shading above
  // and the glyph placement can be tuned independently.
  const NUMERAL_LIFT = 0.004;

  // axis is the rest-axis index this numeral belongs to — the face index for
  // most dice, the vertex index for the d4. dice3d.js repaints a quad by looking
  // its axis up through a permutation, so every quad has to know which one it
  // speaks for.
  function numeralQuad(centroid, normal, up, glyph, halfSize, axis) {
    // (right, up, normal) right-handed: right x up = normal.
    const right = cross(up, normal);
    return {
      center: addv(centroid, mul(normal, NUMERAL_LIFT)),
      right: mul(right, halfSize),
      up: mul(up, halfSize),
      normal,
      glyph,
      axis,
    };
  }

  function buildNumerals(verts, faces, kind) {
    const quads = [];
    if (kind === "d4") {
      // A resting tetrahedron has a face flat on the table and a vertex
      // pointing up, so a d4 is read at its apex, not off a top face. Each
      // face therefore carries three numerals, one tucked into each corner
      // and rotated so its top points at that corner: whichever vertex ends
      // up highest is ringed by three copies of its own number, exactly like
      // a physical d4. See restAxes below — for this solid they are vertex
      // directions.
      for (const face of faces) {
        const inr = faceInradius(face, verts);
        for (const vi of face.indices) {
          const toCorner = sub(verts[vi], face.centroid);
          const dir = norm(toCorner);
          const center = addv(face.centroid, mul(dir, length(toCorner) * 0.56));
          quads.push(numeralQuad(center, face.normal, dir, verts[vi].glyph, inr * 0.36, vi));
        }
      }
      return quads;
    }
    faces.forEach((face, fi) => {
      // An arbitrary but stable in-face "up": towards the first vertex of the
      // loop. Real dice have no consistent numeral rotation either.
      const up = norm(sub(verts[face.indices[0]], face.centroid));
      quads.push(numeralQuad(face.centroid, face.normal, up, face.glyph, faceInradius(face, verts) * 0.62, fi));
    });
    return quads;
  }

  // --- Inertia -------------------------------------------------------------

  // dice3d.js treats every die as having isotropic inertia (a single scalar
  // rather than a tensor that has to be rotated into world space every step).
  // For shapes this close to spherical the visual difference is nil and it
  // removes a whole class of instability from the integrator. The scalar is
  // still the real one for the solid, not a guess: for isotropic inertia each
  // diagonal component is (2/3)∫|x|²dV / V per unit mass.
  //
  // ∫|x|²dV is summed over tetrahedra (origin, edge of a face fan), using the
  // exact result for a tetra with one vertex at the origin:
  //     ∫|x|²dV = V/10 * (|a|² + |b|² + |c|² + a·b + a·c + b·c)
  function isotropicInertia(verts, faces) {
    let volume = 0;
    let integral = 0;
    for (const face of faces) {
      const loop = face.indices.map((i) => verts[i]);
      for (let i = 1; i < loop.length - 1; i++) {
        const a = loop[0];
        const b = loop[i];
        const c = loop[i + 1];
        const v = Math.abs(dot(a, cross(b, c))) / 6;
        volume += v;
        integral += (v / 10) * (dot(a, a) + dot(b, b) + dot(c, c) + dot(a, b) + dot(a, c) + dot(b, c));
      }
    }
    return (2 / 3) * (integral / volume);
  }

  // --- Assembly ------------------------------------------------------------

  const KINDS = {
    d4: { verts: tetrahedron, spec: () => standardSpec(4) },
    d6: { verts: cube, spec: () => standardSpec(6) },
    d8: { verts: octahedron, spec: () => standardSpec(8) },
    d10: { verts: trapezohedron, spec: () => standardSpec(10) },
    d12: { verts: dodecahedron, spec: () => standardSpec(12) },
    d20: { verts: icosahedron, spec: () => standardSpec(20) },
    "d100-tens": { verts: trapezohedron, spec: tensSpec },
    "d100-units": { verts: trapezohedron, spec: unitsSpec },
  };

  const cache = new Map();

  function build(kind) {
    const def = KINDS[kind];
    if (!def) throw new Error("unknown die kind: " + kind);

    // Scale to circumradius 1 so dice3d.js has one number to multiply by.
    const raw = def.verts();
    const scale = 1 / Math.max(...raw.map(length));
    const verts = raw.map((v) => mul(v, scale));

    const faces = hullFaces(verts);
    const spec = def.spec();
    const restAxes = [];
    const values = [];
    const glyphs = [];

    if (kind === "d4") {
      // Values live on the vertices; the up-pointing vertex is the reading.
      verts.forEach((v, i) => {
        v.glyph = spec.glyphs[i];
        v.value = spec.values[i];
        restAxes.push(norm(v));
        values.push(spec.values[i]);
        glyphs.push(spec.glyphs[i]);
      });
    } else {
      assignFaceNumbers(faces, spec);
      for (const face of faces) {
        restAxes.push(face.normal);
        values.push(face.value);
        glyphs.push(face.glyph);
      }
    }

    // Distance from the centre to the table when the die is at rest. Whatever
    // is up, the thing touching the table is a face, and every face of these
    // solids is the same distance from the centre — so this is just the
    // inradius, and it is the same for every resting orientation.
    const inradius = Math.min(...faces.map((f) => f.offset));

    return {
      kind,
      vertices: verts.map((v) => [v[0], v[1], v[2]]),
      faces,
      body: buildBodyMesh(verts, faces),
      numerals: buildNumerals(verts, faces, kind),
      restAxes,
      values,
      glyphs,
      symmetries: rotationSymmetries(restAxes),
      restHeight: inradius,
      circumradius: 1,
      inertia: isotropicInertia(verts, faces),
    };
  }

  function get(kind) {
    if (!cache.has(kind)) cache.set(kind, build(kind));
    return cache.get(kind);
  }

  // --- Numeral atlas -------------------------------------------------------

  function glyphList() {
    const set = new Set();
    for (const kind of Object.keys(KINDS)) {
      for (const g of KINDS[kind].spec().glyphs) set.add(g);
    }
    return Array.from(set);
  }

  // One canvas, every glyph, drawn white on transparent so the shader can
  // tint it. Cells are square and the font shrinks to fit, so a two-character
  // "00" and a one-character "7" occupy the same quad and every numeral on a
  // die is the same physical size.
  //
  // No mipmaps (and therefore no power-of-two requirement): mip levels would
  // bleed neighbouring cells into each other, and at a ~100px die on a 2x
  // display a 96px cell is already close to 1:1 on screen.
  function buildAtlas(cellPx) {
    const glyphs = glyphList();
    const cols = 8;
    const rows = Math.ceil(glyphs.length / cols);
    const canvas = document.createElement("canvas");
    canvas.width = cols * cellPx;
    canvas.height = rows * cellPx;
    const ctx = canvas.getContext("2d");
    ctx.clearRect(0, 0, canvas.width, canvas.height);
    ctx.fillStyle = "#ffffff";
    ctx.strokeStyle = "#ffffff";
    ctx.textAlign = "center";
    ctx.textBaseline = "middle";

    const cells = new Map();
    glyphs.forEach((glyph, i) => {
      const cx = (i % cols) * cellPx;
      const cy = Math.floor(i / cols) * cellPx;

      // Fit the glyph inside the cell with a margin, shrinking for the
      // two-character numbers rather than letting them run to the edges.
      let font = cellPx * 0.66;
      ctx.font = "700 " + font + "px " + FONT_STACK;
      const maxWidth = cellPx * 0.72;
      const w = ctx.measureText(glyph).width;
      if (w > maxWidth) {
        font *= maxWidth / w;
        ctx.font = "700 " + font + "px " + FONT_STACK;
      }

      // 6 and 9 are underlined, as on any real set — without it a d6 showing
      // one of them is genuinely ambiguous, and so is a d20's 6 against 9.
      const underlined = glyph === "6" || glyph === "9";
      const baselineShift = underlined ? -cellPx * 0.05 : 0;
      ctx.fillText(glyph, cx + cellPx / 2, cy + cellPx / 2 + baselineShift);
      if (underlined) {
        const half = ctx.measureText(glyph).width * 0.42;
        ctx.lineWidth = Math.max(2, cellPx * 0.05);
        ctx.beginPath();
        ctx.moveTo(cx + cellPx / 2 - half, cy + cellPx * 0.78);
        ctx.lineTo(cx + cellPx / 2 + half, cy + cellPx * 0.78);
        ctx.stroke();
      }

      // Inset the UV window by half a texel so LINEAR filtering at the cell
      // border cannot reach into the neighbouring glyph.
      const pad = 0.5;
      cells.set(glyph, {
        u0: (cx + pad) / canvas.width,
        v0: (cy + pad) / canvas.height,
        u1: (cx + cellPx - pad) / canvas.width,
        v1: (cy + cellPx - pad) / canvas.height,
      });
    });

    return { canvas, cells };
  }

  const FONT_STACK = '-apple-system, "Segoe UI", Roboto, sans-serif';

  window.n5eDiceSolids = { get, glyphList, buildAtlas, KINDS: Object.keys(KINDS) };
})();
