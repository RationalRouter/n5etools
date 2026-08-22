// 3D dice: WebGL renderer + rigid-body physics, driven by dice3d-solids.js.
//
// Exposes window.n5eDice3D = {available, roll, clear, resize}. static/js/dice-roller.js
// owns the tray, the results panel and the n5e:roll-result event; this file owns
// everything between "here is a pool of dice" and "here are the values, and they are
// on screen".
//
// Three design decisions carry the whole thing, and they are the reason this exists
// at all rather than another pass at a physics library:
//
//   1. VALUES ARE DRAWN UP FRONT, never read back off the simulation. crypto random
//      picks every number before a frame renders; when a die stops (or the hard cap
//      fires) it is *rotated* into the rest pose that shows its number. The physics
//      is choreography. A cocked die, a die that jitters forever, a die that gets
//      shoved off screen — none of them can produce a wrong or missing value.
//   2. A WATCHDOG INDEPENDENT OF THE RENDER LOOP resolves every roll. Tab hidden,
//      context lost, GPU wedged: SETTLE_TIMEOUT_MS later the promise resolves and the
//      tray shows the result anyway.
//   3. FIXED TIMESTEP with an accumulator. A 60 Hz and a 144 Hz screen run the same
//      simulation; framerate changes how smooth a roll looks, never how it behaves.
//
// Coordinates match dice3d-solids.js: Z is up, the table is z = 0, and one world unit
// is one CSS pixel. The camera looks straight down and is solved so the table plane
// exactly fills the viewport — that is what makes "the whole screen is the tray" true
// rather than approximately true, since the four physics walls then sit exactly on the
// window edges.
(function () {
  "use strict";

  // --- Tuning ---------------------------------------------------------------

  // Nominal die size. Each solid is scaled from this by KIND_SCALE below.
  const DIE_PIXELS = 100;

  // Solids come out of dice3d-solids.js at circumradius 1, but circumradius is a
  // poor measure of how big a die *looks*: a cube at circumradius 50 reads as a
  // 58px block while a d20 at the same circumradius reads as a 100px ball. These
  // multipliers are picked so each die's characteristic visible width (edge for
  // the flat-faced solids, diameter for the round ones) lands near DIE_PIXELS,
  // which is how a real dice set is sized.
  const KIND_SCALE = {
    d4: 1.08,
    d6: 1.39,
    d8: 1.02,
    d10: 0.96,
    d12: 0.98,
    d20: 1.0,
    "d100-tens": 0.96,
    "d100-units": 0.96,
  };

  const GRAVITY = 2400;        // px/s^2, tuned by eye — real g is far too fast at this scale
  const THROW_SPEED = 1050;    // px/s, before per-die +/-20% jitter
  const RESTITUTION = 0.32;    // bounciness against floor and walls
  const FRICTION = 0.45;       // Coulomb friction coefficient at contacts
  const SPAWN_STAGGER_MS = 55; // gap between dice entering; purely cosmetic
  const SPAWN_STAGGER_TOTAL_MS = 450; // the whole pool is in play by here
  const SETTLE_TIMEOUT_MS = 4000;
  const SETTLE_SLERP_MS = 380; // guided rotation into the predetermined face
  const FADE_DELAY_MS = 10000; // how long settled dice stay on screen
  const FADE_MS = 600;

  // Below this approach speed a contact is fully inelastic. Contact points on a
  // spinning die move fast even when the body is barely translating, so a low
  // threshold leaves a die trading bounces with the table forever.
  const BOUNCE_MIN_SPEED = 120; // px/s

  // Guaranteed calm-down. Friction and damping alone decide when a die stops,
  // which is fine on average and useless as a guarantee — a d20 is nearly a ball
  // and will happily roll for four seconds. From CALM_START_MS a per-step
  // velocity scale ramps in, reaching CALM_RATE by CALM_FULL_MS, which pins
  // every die to a stop about a second before the watchdog would fire. It reads
  // as the table having some drag, and it is what keeps the timeout path rare
  // rather than routine.
  const CALM_START_MS = 900;
  const CALM_FULL_MS = 2100;
  const CALM_RATE = 0.88; // per 1/120s step at full strength

  // Every die settles on its own clock, not just when the sleep test happens to
  // fire. By this age the calm ramp has had 500ms at full strength, so the die is
  // already all but stationary and the guided settle looks like it coming to
  // rest — which is the whole point. Sleep detection still ends most dice earlier;
  // this is what stops the *global* watchdog from being the thing that ends rolls,
  // because that one fires on whatever state the die is in, however fast.
  const DIE_DEADLINE_MS = 2600;

  // Walls switch on once a die is inside the viewport (see makeDie: it starts
  // outside). If one somehow hasn't made it in by here, switch them on anyway
  // rather than let it sail off and be corrected from off-screen later.
  const ARM_FALLBACK_MS = 700;

  const STEP = 1 / 120;
  const MAX_STEPS_PER_FRAME = 8; // clamp after a stall so we never spiral
  const SOLVER_ITERATIONS = 3;
  const MAX_DICE = 50;           // past this the roll resolves instantly, unanimated

  // Generous on purpose. The guided settle fixes the orientation exactly anyway,
  // so calling a die "stopped" while it still has a few px/s left costs nothing
  // and avoids chasing a perfect standstill the solver may never quite reach.
  const SLEEP_LINEAR = 28;   // px/s
  const SLEEP_ANGULAR = 1.6; // rad/s
  const SLEEP_STEPS = 8;

  // A die balanced on an edge or corner is a genuine zero-net-impulse
  // equilibrium in this simulation — nothing here models the table texture
  // or air movement that always tips a real die the rest of the way — so
  // velocity alone is not enough to call it stopped. SLEEP_FLAT_COS gates
  // sleep on the up axis actually being close to vertical; short of that,
  // EDGE_NUDGE gives it a small random kick instead of freezing tilted, so
  // beginSettle is only ever left the fraction of a degree it expects.
  const SLEEP_FLAT_COS = Math.cos(8 * Math.PI / 180);
  const EDGE_NUDGE = 2.5; // rad/s, per axis

  const IMPACT_MIN_GAP_MS = 40;
  const IMPACT_MAX_PER_ROLL = 12;
  const IMPACT_REF = 400; // impulse that maps to full-volume clack

  const DPR_CAP = 2;
  const ATLAS_CELL_PX = 96;
  const FOV_Y = 35 * Math.PI / 180;

  const UP = [0, 0, 1];
  const LIGHT = normalize3([-0.35, 0.45, 0.82]);

  const debug = (function () {
    try { return localStorage.getItem("n5e-dice-debug") === "1"; } catch (e) { return false; }
  })();

  // --- Small vector / quaternion helpers ------------------------------------
  //
  // These run inside the physics loop, so they take and return plain arrays and
  // avoid allocating where it is easy not to. Quaternions are [x, y, z, w].

  function normalize3(a) {
    const l = Math.hypot(a[0], a[1], a[2]) || 1;
    return [a[0] / l, a[1] / l, a[2] / l];
  }
  function dot3(a, b) { return a[0] * b[0] + a[1] * b[1] + a[2] * b[2]; }
  function cross3(a, b) {
    return [
      a[1] * b[2] - a[2] * b[1],
      a[2] * b[0] - a[0] * b[2],
      a[0] * b[1] - a[1] * b[0],
    ];
  }

  function quatNormalize(q) {
    const l = Math.hypot(q[0], q[1], q[2], q[3]) || 1;
    q[0] /= l; q[1] /= l; q[2] /= l; q[3] /= l;
    return q;
  }

  function quatMul(a, b) {
    return [
      a[3] * b[0] + a[0] * b[3] + a[1] * b[2] - a[2] * b[1],
      a[3] * b[1] - a[0] * b[2] + a[1] * b[3] + a[2] * b[0],
      a[3] * b[2] + a[0] * b[1] - a[1] * b[0] + a[2] * b[3],
      a[3] * b[3] - a[0] * b[0] - a[1] * b[1] - a[2] * b[2],
    ];
  }

  function quatRotate(q, v) {
    // v + 2 * cross(q.xyz, cross(q.xyz, v) + q.w * v)
    const t = [
      q[1] * v[2] - q[2] * v[1] + q[3] * v[0],
      q[2] * v[0] - q[0] * v[2] + q[3] * v[1],
      q[0] * v[1] - q[1] * v[0] + q[3] * v[2],
    ];
    return [
      v[0] + 2 * (q[1] * t[2] - q[2] * t[1]),
      v[1] + 2 * (q[2] * t[0] - q[0] * t[2]),
      v[2] + 2 * (q[0] * t[1] - q[1] * t[0]),
    ];
  }

  function quatRandom() {
    // Shoemake's uniform random rotation — a naive random axis + angle clumps
    // orientations near the poles and would bias which face a die starts near.
    const u1 = Math.random(), u2 = Math.random(), u3 = Math.random();
    const s1 = Math.sqrt(1 - u1), s2 = Math.sqrt(u1);
    return [
      s1 * Math.sin(2 * Math.PI * u2),
      s1 * Math.cos(2 * Math.PI * u2),
      s2 * Math.sin(2 * Math.PI * u3),
      s2 * Math.cos(2 * Math.PI * u3),
    ];
  }

  // Shortest-arc rotation taking unit vector `from` onto unit vector `to`.
  function quatShortestArc(from, to) {
    const d = dot3(from, to);
    if (d > 0.999999) return [0, 0, 0, 1];
    if (d < -0.999999) {
      // Antiparallel: any perpendicular axis, half turn.
      let axis = cross3([1, 0, 0], from);
      if (dot3(axis, axis) < 1e-6) axis = cross3([0, 1, 0], from);
      axis = normalize3(axis);
      return [axis[0], axis[1], axis[2], 0];
    }
    const c = cross3(from, to);
    return quatNormalize([c[0], c[1], c[2], 1 + d]);
  }

  function quatSlerp(a, b, t) {
    let d = a[0] * b[0] + a[1] * b[1] + a[2] * b[2] + a[3] * b[3];
    let bx = b[0], by = b[1], bz = b[2], bw = b[3];
    if (d < 0) { d = -d; bx = -bx; by = -by; bz = -bz; bw = -bw; }
    let s0, s1;
    if (d > 0.9995) {
      s0 = 1 - t; s1 = t;
    } else {
      const theta = Math.acos(d);
      const sin = Math.sin(theta);
      s0 = Math.sin((1 - t) * theta) / sin;
      s1 = Math.sin(t * theta) / sin;
    }
    return quatNormalize([
      a[0] * s0 + bx * s1,
      a[1] * s0 + by * s1,
      a[2] * s0 + bz * s1,
      a[3] * s0 + bw * s1,
    ]);
  }

  function quatToMat3(q) {
    const [x, y, z, w] = q;
    const x2 = x + x, y2 = y + y, z2 = z + z;
    const xx = x * x2, xy = x * y2, xz = x * z2;
    const yy = y * y2, yz = y * z2, zz = z * z2;
    const wx = w * x2, wy = w * y2, wz = w * z2;
    // Column-major, as GL wants it.
    return [
      1 - (yy + zz), xy + wz, xz - wy,
      xy - wz, 1 - (xx + zz), yz + wx,
      xz + wy, yz - wx, 1 - (xx + yy),
    ];
  }

  // --- Randomness -----------------------------------------------------------

  // Uniform integer in [1, n] with rejection sampling. Math.random() would do for
  // a dice tray, but the modulo-bias-free path costs nothing and this is the one
  // number in the whole feature a user might actually dispute.
  const randBuf = new Uint32Array(1);
  function randInt(n) {
    const crypto = window.crypto || window.msCrypto;
    if (!crypto || !crypto.getRandomValues) return 1 + Math.floor(Math.random() * n);
    const limit = Math.floor(0x100000000 / n) * n;
    for (;;) {
      crypto.getRandomValues(randBuf);
      if (randBuf[0] < limit) return 1 + (randBuf[0] % n);
    }
  }

  function rand(lo, hi) { return lo + Math.random() * (hi - lo); }

  // --- WebGL plumbing -------------------------------------------------------

  const BODY_VS = [
    "attribute vec3 aPos;",
    "attribute vec3 aNormal;",
    "attribute float aEdge;",
    "uniform mat4 uViewProj;",
    "uniform mat4 uModel;",
    "uniform mat3 uRot;",
    "varying vec3 vNormal;",
    "varying float vEdge;",
    "void main() {",
    "  vNormal = uRot * aNormal;",
    "  vEdge = aEdge;",
    "  gl_Position = uViewProj * uModel * vec4(aPos, 1.0);",
    "}",
  ].join("\n");

  // Output is premultiplied (colour already multiplied by alpha) to match the
  // canvas's default premultipliedAlpha and blendFunc(ONE, ONE_MINUS_SRC_ALPHA);
  // that is what lets the page show through cleanly around the dice.
  const BODY_FS = [
    "precision mediump float;",
    "uniform vec3 uColor;",
    "uniform vec3 uLight;",
    "uniform float uAlpha;",
    "varying vec3 vNormal;",
    "varying float vEdge;",
    "void main() {",
    "  vec3 n = normalize(vNormal);",
    "  float diff = max(dot(n, uLight), 0.0);",
    // vEdge ramps 0 -> 1 across the skirt ring the solids module emits, so this
    // darkens the rim of every face and reads as a bevelled edge.
    "  float rim = 1.0 - 0.45 * vEdge;",
    "  vec3 c = uColor * (0.38 + 0.72 * diff) * rim;",
    "  c += pow(max(n.z, 0.0), 24.0) * 0.16;",
    "  gl_FragColor = vec4(c * uAlpha, uAlpha);",
    "}",
  ].join("\n");

  const NUMERAL_VS = [
    "attribute vec3 aPos;",
    "attribute vec2 aUV;",
    "uniform mat4 uViewProj;",
    "uniform mat4 uModel;",
    "varying vec2 vUV;",
    "void main() {",
    "  vUV = aUV;",
    "  gl_Position = uViewProj * uModel * vec4(aPos, 1.0);",
    "}",
  ].join("\n");

  const NUMERAL_FS = [
    "precision mediump float;",
    "uniform sampler2D uTex;",
    "uniform vec3 uColor;",
    "uniform float uAlpha;",
    "varying vec2 vUV;",
    "void main() {",
    "  float a = texture2D(uTex, vUV).a * uAlpha;",
    "  gl_FragColor = vec4(uColor * a, a);",
    "}",
  ].join("\n");

  // One shader for both the floor shadows and (in debug mode) contact markers:
  // a screen-facing quad on the table plane, softly or hard filled.
  const BLOB_VS = [
    "attribute vec2 aQuad;",
    "uniform mat4 uViewProj;",
    "uniform vec3 uCenter;",
    "uniform float uSize;",
    "varying vec2 vQuad;",
    "void main() {",
    "  vQuad = aQuad;",
    "  vec3 p = uCenter + vec3(aQuad * uSize, 0.0);",
    "  gl_Position = uViewProj * vec4(p, 1.0);",
    "}",
  ].join("\n");

  const BLOB_FS = [
    "precision mediump float;",
    "uniform vec4 uColor;",
    "uniform float uAlpha;",
    "uniform float uSoft;",
    "varying vec2 vQuad;",
    "void main() {",
    "  float r = length(vQuad);",
    // Written as 1.0 - smoothstep(lo, hi, r) rather than smoothstep(hi, lo, r):
    // GLSL leaves smoothstep undefined when edge0 >= edge1, and some drivers do
    // return garbage for it.
    "  float edge = mix(step(r, 1.0), 1.0 - smoothstep(0.2, 1.0, r), uSoft);",
    "  float a = uColor.a * uAlpha * edge;",
    "  gl_FragColor = vec4(uColor.rgb * a, a);",
    "}",
  ].join("\n");

  function compile(gl, type, src) {
    const sh = gl.createShader(type);
    gl.shaderSource(sh, src);
    gl.compileShader(sh);
    if (!gl.getShaderParameter(sh, gl.COMPILE_STATUS)) {
      const log = gl.getShaderInfoLog(sh);
      gl.deleteShader(sh);
      throw new Error("dice shader failed: " + log);
    }
    return sh;
  }

  // Returns the program plus a lazily-populated uniform/attribute lookup, so call
  // sites read as prog.u.uColor rather than a pile of getUniformLocation calls.
  function link(gl, vsSrc, fsSrc, attribs, uniforms) {
    const p = gl.createProgram();
    const vs = compile(gl, gl.VERTEX_SHADER, vsSrc);
    const fs = compile(gl, gl.FRAGMENT_SHADER, fsSrc);
    gl.attachShader(p, vs);
    gl.attachShader(p, fs);
    gl.linkProgram(p);
    gl.deleteShader(vs);
    gl.deleteShader(fs);
    if (!gl.getProgramParameter(p, gl.LINK_STATUS)) {
      const log = gl.getProgramInfoLog(p);
      gl.deleteProgram(p);
      throw new Error("dice program failed: " + log);
    }
    const a = {};
    for (const name of attribs) a[name] = gl.getAttribLocation(p, name);
    const u = {};
    for (const name of uniforms) u[name] = gl.getUniformLocation(p, name);
    return { program: p, a, u };
  }

  function buffer(gl, data) {
    const b = gl.createBuffer();
    gl.bindBuffer(gl.ARRAY_BUFFER, b);
    gl.bufferData(gl.ARRAY_BUFFER, data, gl.STATIC_DRAW);
    return b;
  }

  function perspective(out, fovy, aspect, near, far) {
    const f = 1 / Math.tan(fovy / 2);
    const nf = 1 / (near - far);
    out[0] = f / aspect; out[1] = 0; out[2] = 0; out[3] = 0;
    out[4] = 0; out[5] = f; out[6] = 0; out[7] = 0;
    out[8] = 0; out[9] = 0; out[10] = (far + near) * nf; out[11] = -1;
    out[12] = 0; out[13] = 0; out[14] = 2 * far * near * nf; out[15] = 0;
    return out;
  }

  // --- Engine ---------------------------------------------------------------

  let canvas = null;
  let gl = null;
  let ready = false;      // GL resources built and usable
  let initFailed = false;
  let probed = null;      // cheap "could we ever have WebGL" answer, computed once

  let progBody = null, progNumeral = null, progBlob = null;
  let blobBuf = null;
  let atlasTex = null;
  let atlasCells = null; // glyph -> UV window, shared by every die's relabelling
  const meshes = new Map(); // kind -> {solid, body:{...bufs}, numeral:{...bufs}}

  const viewProj = new Float32Array(16);
  const projTmp = new Float32Array(16);
  const modelTmp = new Float32Array(16);

  let cssW = 0, cssH = 0, halfW = 0, halfH = 0, camZ = 0;

  let dice = [];
  let running = false;
  let rafID = 0;
  let accumulator = 0;
  let lastFrame = 0;
  let globalAlpha = 1;

  let activeRoll = null; // {resolve, detail, watchdog, startedAt, onImpact, impacts, lastImpactAt}
  let fadeTimer = 0;

  const debugPoints = [];

  function cssVar(name, fallback) {
    try {
      const v = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
      return v || fallback;
    } catch (e) {
      return fallback;
    }
  }

  // "#2e8555" -> [0.18, 0.52, 0.33]. Anything unparseable falls back to the
  // literal green rather than throwing — a bad CSS var must not kill a roll.
  function parseColor(text, fallback) {
    const hex = /^#?([0-9a-f]{6})$/i.exec((text || "").trim());
    if (!hex) return fallback;
    const n = parseInt(hex[1], 16);
    return [((n >> 16) & 255) / 255, ((n >> 8) & 255) / 255, (n & 255) / 255];
  }

  function probe() {
    if (probed !== null) return probed;
    probed = false;
    try {
      if (!window.WebGLRenderingContext) return probed;
      const c = document.createElement("canvas");
      c.width = 1; c.height = 1;
      const ctx = c.getContext("webgl2") || c.getContext("webgl") || c.getContext("experimental-webgl");
      probed = !!ctx;
      const lose = ctx && ctx.getExtension && ctx.getExtension("WEBGL_lose_context");
      if (lose) lose.loseContext();
    } catch (e) {
      probed = false;
    }
    return probed;
  }

  function ensureInit() {
    if (ready) return true;
    if (initFailed) return false;
    if (!window.n5eDiceSolids) { initFailed = true; return false; }

    canvas = document.getElementById("dice-3d");
    if (!canvas) { initFailed = true; return false; }

    const attrs = { alpha: true, antialias: true, premultipliedAlpha: true, depth: true, stencil: false };
    gl = canvas.getContext("webgl2", attrs) || canvas.getContext("webgl", attrs)
      || canvas.getContext("experimental-webgl", attrs);
    if (!gl) { initFailed = true; return false; }

    canvas.addEventListener("webglcontextlost", onContextLost, false);
    canvas.addEventListener("webglcontextrestored", onContextRestored, false);

    try {
      buildGL();
    } catch (err) {
      console.warn("dice3d: init failed,", err);
      initFailed = true;
      gl = null;
      return false;
    }
    ready = true;
    resize();
    return true;
  }

  function buildGL() {
    progBody = link(gl, BODY_VS, BODY_FS,
      ["aPos", "aNormal", "aEdge"],
      ["uViewProj", "uModel", "uRot", "uColor", "uLight", "uAlpha"]);
    progNumeral = link(gl, NUMERAL_VS, NUMERAL_FS,
      ["aPos", "aUV"],
      ["uViewProj", "uModel", "uTex", "uColor", "uAlpha"]);
    progBlob = link(gl, BLOB_VS, BLOB_FS,
      ["aQuad"],
      ["uViewProj", "uCenter", "uSize", "uColor", "uAlpha", "uSoft"]);

    blobBuf = buffer(gl, new Float32Array([-1, -1, 1, -1, 1, 1, -1, -1, 1, 1, -1, 1]));

    const atlas = window.n5eDiceSolids.buildAtlas(ATLAS_CELL_PX);
    atlasCells = atlas.cells;
    atlasTex = gl.createTexture();
    gl.bindTexture(gl.TEXTURE_2D, atlasTex);
    gl.pixelStorei(gl.UNPACK_PREMULTIPLY_ALPHA_WEBGL, false);
    gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, gl.RGBA, gl.UNSIGNED_BYTE, atlas.canvas);
    // No mipmaps: they would bleed neighbouring atlas cells together, and a 96px
    // cell is already near 1:1 for a ~100px die on a 2x display.
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);

    meshes.clear();
    for (const kind of window.n5eDiceSolids.KINDS) meshes.set(kind, buildMesh(kind, atlas));

    gl.enable(gl.DEPTH_TEST);
    gl.enable(gl.CULL_FACE);
    gl.frontFace(gl.CCW);
    gl.cullFace(gl.BACK);
    gl.enable(gl.BLEND);
    gl.blendFunc(gl.ONE, gl.ONE_MINUS_SRC_ALPHA);
    gl.clearColor(0, 0, 0, 0);
  }

  function buildMesh(kind, atlas) {
    const solid = window.n5eDiceSolids.get(kind);
    const body = {
      pos: buffer(gl, solid.body.positions),
      normal: buffer(gl, solid.body.normals),
      edge: buffer(gl, solid.body.edges),
      count: solid.body.count,
    };

    const positions = [];
    const uvs = [];
    const axes = [];
    for (const q of solid.numerals) {
      const cell = atlas.cells.get(q.glyph);
      if (!cell) continue;
      axes.push(q.axis);
      const corner = (sr, su) => [
        q.center[0] + q.right[0] * sr + q.up[0] * su,
        q.center[1] + q.right[1] * sr + q.up[1] * su,
        q.center[2] + q.right[2] * sr + q.up[2] * su,
      ];
      // (right, up, normal) is right-handed, so -1,-1 -> +1,-1 -> +1,+1 winds
      // counter-clockwise seen from outside the face and survives back-face
      // culling exactly when its face is visible.
      const p = [corner(-1, -1), corner(1, -1), corner(1, 1), corner(-1, 1)];
      // The atlas canvas has v = 0 at the top, and su = +1 is the glyph's top.
      const t = [[cell.u0, cell.v1], [cell.u1, cell.v1], [cell.u1, cell.v0], [cell.u0, cell.v0]];
      for (const i of [0, 1, 2, 0, 2, 3]) {
        positions.push(p[i][0], p[i][1], p[i][2]);
        uvs.push(t[i][0], t[i][1]);
      }
    }

    return {
      solid,
      radius: (DIE_PIXELS / 2) * (KIND_SCALE[kind] || 1),
      body,
      numeral: {
        pos: buffer(gl, new Float32Array(positions)),
        // The default labelling, and the template every die's own copy starts
        // from. Positions are shared — only the UVs differ per die, because
        // only which glyph sits on which face is ever re-assigned.
        uv: new Float32Array(uvs),
        axes,                      // rest-axis index per quad, 12 UV floats each
        count: positions.length / 3,
      },
    };
  }

  // Rewrite one die's numeral UVs from its current labelling. labelOfAxis[a] is
  // the rest-axis whose glyph axis a currently shows; identity is the solid's
  // own numbering.
  function writeLabels(die) {
    const mesh = die.mesh;
    const cells = atlasCells;
    const glyphs = die.solid.glyphs;
    const uv = die.uv;
    const axes = mesh.numeral.axes;
    for (let q = 0; q < axes.length; q++) {
      const cell = cells.get(glyphs[die.labelOfAxis[axes[q]]]);
      if (!cell) continue;
      const o = q * 12;
      // Same winding as buildMesh: (-1,-1) (1,-1) (1,1) / (-1,-1) (1,1) (-1,1).
      uv[o] = cell.u0; uv[o + 1] = cell.v1;
      uv[o + 2] = cell.u1; uv[o + 3] = cell.v1;
      uv[o + 4] = cell.u1; uv[o + 5] = cell.v0;
      uv[o + 6] = cell.u0; uv[o + 7] = cell.v1;
      uv[o + 8] = cell.u1; uv[o + 9] = cell.v0;
      uv[o + 10] = cell.u0; uv[o + 11] = cell.v0;
    }
    gl.bindBuffer(gl.ARRAY_BUFFER, die.uvBuf);
    gl.bufferSubData(gl.ARRAY_BUFFER, 0, uv);
    die.labelDirty = false;
  }

  function onContextLost(e) {
    e.preventDefault();
    ready = false;
    stopLoop();
    // Losing the GPU must not lose the roll: the values were drawn before the
    // first frame, so resolve with them and let the tray show the result.
    finishRoll("context-lost");
    dice = [];
  }

  function onContextRestored() {
    try {
      buildGL();
      ready = true;
      resize();
    } catch (err) {
      console.warn("dice3d: restore failed,", err);
      initFailed = true;
    }
  }

  // --- Camera and bounds ----------------------------------------------------

  // Solve the camera so the table plane (z = 0) exactly fills the viewport: the
  // physics walls then sit on the window edges, and a die stopping at the edge of
  // the screen is the same event as a die hitting a wall.
  function resize() {
    if (!ready || !canvas) return;
    cssW = Math.max(1, window.innerWidth);
    cssH = Math.max(1, window.innerHeight);
    halfW = cssW / 2;
    halfH = cssH / 2;

    const dpr = Math.min(window.devicePixelRatio || 1, DPR_CAP);
    const bw = Math.round(cssW * dpr);
    const bh = Math.round(cssH * dpr);
    if (canvas.width !== bw || canvas.height !== bh) {
      canvas.width = bw;
      canvas.height = bh;
    }
    gl.viewport(0, 0, bw, bh);

    camZ = halfH / Math.tan(FOV_Y / 2);
    perspective(projTmp, FOV_Y, cssW / cssH, camZ * 0.05, camZ * 2.5);
    // View is a pure translation: the camera sits at +Z looking down -Z, which is
    // GL's default orientation, so world +X is screen right and +Y is screen up.
    viewProj.set(projTmp);
    for (let i = 0; i < 4; i++) {
      viewProj[12 + i] = projTmp[8 + i] * -camZ + projTmp[12 + i];
    }

    // A resize mid-roll must not strand dice outside the new window.
    for (const d of dice) {
      if (!d.armed) continue;
      d.p[0] = Math.max(-halfW + d.radius, Math.min(halfW - d.radius, d.p[0]));
      d.p[1] = Math.max(-halfH + d.radius, Math.min(halfH - d.radius, d.p[1]));
    }
  }

  // --- Dice -----------------------------------------------------------------

  function kindFor(sides) {
    switch (sides) {
      case 4: return "d4";
      case 6: return "d6";
      case 8: return "d8";
      case 10: return "d10";
      case 12: return "d12";
      case 20: return "d20";
      default: return null;
    }
  }

  function makeDie(kind, value, index, total) {
    const mesh = meshes.get(kind);
    const R = mesh.radius;
    const solid = mesh.solid;

    // Each die gets its own edge, its own point along it, its own heading and its
    // own speed. The old roller launched everything from roughly one point and
    // the dice arrived as a clump; independent origins are the actual fix, not a
    // wider spread around a shared origin.
    const edge = Math.floor(Math.random() * 4);
    let p;
    const out = R * 2.2 + rand(0, R);
    if (edge === 0) {        // left edge
      p = [-halfW - out, rand(-halfH * 0.85, halfH * 0.85)];
    } else if (edge === 1) { // right edge
      p = [halfW + out, rand(-halfH * 0.85, halfH * 0.85)];
    } else if (edge === 2) { // bottom edge
      p = [rand(-halfW * 0.85, halfW * 0.85), -halfH - out];
    } else {                 // top edge
      p = [rand(-halfW * 0.85, halfW * 0.85), halfH + out];
    }

    // Aim at a random point in the middle of the screen rather than jittering
    // around the edge normal. The spread is just as wide, but every heading is
    // guaranteed to cross the interior — an edge-normal throw with enough jitter
    // can leave along the neighbouring edge without ever getting inside, and a
    // die that never gets inside never gets walls, sails off, and is still out
    // there when the roll has to end.
    const aim = [rand(-halfW * 0.45, halfW * 0.45), rand(-halfH * 0.45, halfH * 0.45)];
    const to = [aim[0] - p[0], aim[1] - p[1]];
    const toLen = Math.hypot(to[0], to[1]) || 1;
    const dir = [to[0] / toLen, to[1] / toLen];
    const speed = THROW_SPEED * rand(0.8, 1.2);

    const stagger = Math.min(SPAWN_STAGGER_MS, SPAWN_STAGGER_TOTAL_MS / Math.max(1, total));

    // Which rest axis carries this die's number, resolved once. maybeRelabel and
    // beginSettle both steer towards it; it never moves.
    let targetIdx = solid.values.indexOf(value);
    if (targetIdx < 0) targetIdx = 0; // impossible unless a spec changed

    // Own copy of the numeral UVs, because this die's numbering is its own —
    // see maybeRelabel. Positions stay shared with every other die of the kind.
    const uv = new Float32Array(mesh.numeral.uv);
    const uvBuf = gl.createBuffer();
    gl.bindBuffer(gl.ARRAY_BUFFER, uvBuf);
    gl.bufferData(gl.ARRAY_BUFFER, uv, gl.DYNAMIC_DRAW);

    return {
      kind,
      mesh,
      solid,
      radius: R,
      value,
      targetIdx,
      uv,
      uvBuf,
      labelOfAxis: solid.restAxes.map((_, i) => i), // identity: the solid's own numbering
      labelDirty: false,
      relabels: 0,
      invInertia: 1 / (solid.inertia * R * R),
      p: [p[0], p[1], rand(R * 1.6, R * 3.4)],
      v: [dir[0] * speed, dir[1] * speed, rand(-40, 120)],
      q: quatRandom(),
      w: [rand(-12, 12), rand(-12, 12), rand(-12, 12)],
      armed: false,     // walls activate once the die is actually on screen
      asleep: false,
      sleepCount: 0,
      startAt: index * stagger,
      live: false,
      settle: null,     // {from, to, fromP, toP, t}
      settled: false,
      spawnedAt: 0,
    };
  }

  // --- Physics --------------------------------------------------------------

  // Inside test: dot(n, x - point) >= 0. Walls point inward.
  function planesFor(die, out) {
    out.length = 0;
    out.push({ n: UP, o: 0 });
    if (die.armed) {
      out.push({ n: [1, 0, 0], o: -halfW });
      out.push({ n: [-1, 0, 0], o: -halfW });
      out.push({ n: [0, 1, 0], o: -halfH });
      out.push({ n: [0, -1, 0], o: -halfH });
    }
    return out;
  }

  const planeScratch = [];
  const contactScratch = [];

  function stepDie(die, dt, report, ageMs) {
    die.v[2] -= GRAVITY * dt;
    die.p[0] += die.v[0] * dt;
    die.p[1] += die.v[1] * dt;
    die.p[2] += die.v[2] * dt;

    // dq/dt = 0.5 * omega_quat * q
    const w = die.w;
    const q = die.q;
    const dq = quatMul([w[0], w[1], w[2], 0], q);
    q[0] += 0.5 * dq[0] * dt;
    q[1] += 0.5 * dq[1] * dt;
    q[2] += 0.5 * dq[2] * dt;
    q[3] += 0.5 * dq[3] * dt;
    quatNormalize(q);

    if (!die.armed
      && ((Math.abs(die.p[0]) < halfW - die.radius * 0.5
        && Math.abs(die.p[1]) < halfH - die.radius * 0.5)
        || ageMs > ARM_FALLBACK_MS)) {
      die.armed = true;
    }

    const planes = planesFor(die, planeScratch);
    const verts = die.solid.vertices;
    const R = die.radius;

    // World-space corners, reused for both the positional pass and the impulses.
    const world = die.worldVerts || (die.worldVerts = []);
    for (let i = 0; i < verts.length; i++) {
      const r = quatRotate(q, verts[i]);
      const wv = world[i] || (world[i] = [0, 0, 0]);
      wv[0] = die.p[0] + r[0] * R;
      wv[1] = die.p[1] + r[1] * R;
      wv[2] = die.p[2] + r[2] * R;
    }

    // Positional correction first, by the deepest corner per plane, so the
    // impulse pass below acts on a die that is touching rather than buried.
    let touchingFloor = false;
    const contacts = contactScratch;
    contacts.length = 0;
    for (const plane of planes) {
      let deepest = 0;
      for (let i = 0; i < world.length; i++) {
        const sep = dot3(plane.n, world[i]) - plane.o;
        if (sep < 0) {
          if (-sep > deepest) deepest = -sep;
          contacts.push({ plane, i });
        }
      }
      if (deepest > 0) {
        die.p[0] += plane.n[0] * deepest;
        die.p[1] += plane.n[1] * deepest;
        die.p[2] += plane.n[2] * deepest;
        for (let i = 0; i < world.length; i++) {
          world[i][0] += plane.n[0] * deepest;
          world[i][1] += plane.n[1] * deepest;
          world[i][2] += plane.n[2] * deepest;
        }
        if (plane.n === UP) touchingFloor = true;
      }
    }

    // Restitution has to be decided ONCE, from the approach velocity as the die
    // arrives, and then treated as a fixed target separating velocity. Folding
    // -(1+e)*vn into every solver iteration instead — the obvious way to write
    // it — quietly injects energy: contact A's impulse changes the body velocity,
    // contact B then sees its own fresh approach velocity and gets its own full
    // restitution kick, and a die landing on three corners at once leaves the
    // floor with more energy than it arrived with. That is a die that never
    // sleeps and always hits the watchdog.
    for (const c of contacts) {
      const n = c.plane.n;
      const wv = world[c.i];
      const r = [wv[0] - die.p[0], wv[1] - die.p[1], wv[2] - die.p[2]];
      const rel = cross3(die.w, r);
      const vn = dot3([die.v[0] + rel[0], die.v[1] + rel[1], die.v[2] + rel[2]], n);
      c.bounce = -vn > BOUNCE_MIN_SPEED ? RESTITUTION * -vn : 0;
    }

    // Sequential impulses. Applying a full impulse at every corner at once would
    // launch the die; recomputing the contact velocity between corners is what
    // makes a multi-corner landing settle instead of explode.
    let peak = 0;
    for (let iter = 0; iter < SOLVER_ITERATIONS; iter++) {
      for (const c of contacts) {
        const n = c.plane.n;
        const wv = world[c.i];
        const r = [wv[0] - die.p[0], wv[1] - die.p[1], wv[2] - die.p[2]];
        const rel = cross3(die.w, r);
        const vel = [die.v[0] + rel[0], die.v[1] + rel[1], die.v[2] + rel[2]];
        const vn = dot3(vel, n);
        // Solve towards the pre-computed separating velocity, not towards zero.
        // Once the contact reaches it, further iterations do nothing.
        if (vn >= c.bounce) continue;

        const rn = cross3(r, n);
        const denom = 1 + die.invInertia * dot3(rn, rn);
        const jn = (c.bounce - vn) / denom;
        applyImpulse(die, r, [n[0] * jn, n[1] * jn, n[2] * jn]);
        if (jn > peak) peak = jn;

        // Friction at the same point, clamped to the Coulomb cone. This is what
        // makes dice roll and skid rather than slide like hockey pucks.
        const rel2 = cross3(die.w, r);
        const vel2 = [die.v[0] + rel2[0], die.v[1] + rel2[1], die.v[2] + rel2[2]];
        const vn2 = dot3(vel2, n);
        const vt = [vel2[0] - n[0] * vn2, vel2[1] - n[1] * vn2, vel2[2] - n[2] * vn2];
        const vtLen = Math.hypot(vt[0], vt[1], vt[2]);
        if (vtLen < 1e-4) continue;
        const t = [vt[0] / vtLen, vt[1] / vtLen, vt[2] / vtLen];
        const rt = cross3(r, t);
        const jtMax = FRICTION * jn;
        let jt = -vtLen / (1 + die.invInertia * dot3(rt, rt));
        if (jt < -jtMax) jt = -jtMax;
        applyImpulse(die, r, [t[0] * jt, t[1] * jt, t[2] * jt]);
      }
    }

    if (peak > 0 && report) report(peak);

    // Damping. The extra bite while touching the floor stands in for rolling
    // resistance and is the difference between dice that stop and dice that
    // drift to a wall every single roll.
    let dampL = 0.998, dampA = 0.997;
    if (touchingFloor) { dampL = 0.982; dampA = 0.95; }
    if (ageMs > CALM_START_MS) {
      const k = Math.min(1, (ageMs - CALM_START_MS) / (CALM_FULL_MS - CALM_START_MS));
      const calm = 1 - (1 - CALM_RATE) * k * k;
      dampL *= calm;
      dampA *= calm;
    }
    die.v[0] *= dampL; die.v[1] *= dampL; die.v[2] *= dampL;
    die.w[0] *= dampA; die.w[1] *= dampA; die.w[2] *= dampA;

    // Keep the die's number on the up face at all times, so beginSettle has
    // nothing left to rotate. Safe to run every step: it moves nothing.
    maybeRelabel(die);

    const linear = Math.hypot(die.v[0], die.v[1], die.v[2]);
    const angular = Math.hypot(die.w[0], die.w[1], die.w[2]);
    // Deliberately not conditioned on touching the floor. A die that comes to
    // rest leaning on a neighbour never registers a floor contact, and would sit
    // there motionless until the watchdog — which then corrects it in full view,
    // the worst-looking case there is. Free fall adds ~20px/s of velocity per
    // step, so nothing airborne can stay under these thresholds for ten
    // consecutive steps; something has to be holding it up.
    if (linear < SLEEP_LINEAR && angular < SLEEP_ANGULAR) {
      const upIdx = upAxisIndex(die);
      const upZ = quatRotate(die.q, die.solid.restAxes[upIdx])[2];
      if (upZ > SLEEP_FLAT_COS) {
        die.sleepCount++;
        if (die.sleepCount >= SLEEP_STEPS) die.asleep = true;
      } else {
        // Stopped, but balanced on an edge or corner rather than a face.
        // Nudge it rather than letting it sleep tilted.
        die.sleepCount = 0;
        die.w[0] += rand(-EDGE_NUDGE, EDGE_NUDGE);
        die.w[1] += rand(-EDGE_NUDGE, EDGE_NUDGE);
        die.w[2] += rand(-EDGE_NUDGE, EDGE_NUDGE);
      }
    } else {
      die.sleepCount = 0;
    }

    if (debug && contacts.length) {
      for (const c of contacts) debugPoints.push([world[c.i][0], world[c.i][1]]);
    }
  }

  function applyImpulse(die, r, imp) {
    die.v[0] += imp[0];
    die.v[1] += imp[1];
    die.v[2] += imp[2];
    const torque = cross3(r, imp);
    die.w[0] += torque[0] * die.invInertia;
    die.w[1] += torque[1] * die.invInertia;
    die.w[2] += torque[2] * die.invInertia;
  }

  // Sphere-vs-sphere at the circumradius. Dice are near-spherical in silhouette,
  // so the approximation does not read as wrong, and it is unconditionally stable
  // where corner-vs-corner between two tumbling polyhedra is not.
  function collidePair(a, b, report) {
    const d = [b.p[0] - a.p[0], b.p[1] - a.p[1], b.p[2] - a.p[2]];
    const dist = Math.hypot(d[0], d[1], d[2]);
    const min = (a.radius + b.radius) * 0.92;
    if (dist >= min || dist < 1e-6) return;

    const n = [d[0] / dist, d[1] / dist, d[2] / dist];
    const push = (min - dist) * 0.5;
    a.p[0] -= n[0] * push; a.p[1] -= n[1] * push; a.p[2] -= n[2] * push;
    b.p[0] += n[0] * push; b.p[1] += n[1] * push; b.p[2] += n[2] * push;

    const ra = [n[0] * a.radius, n[1] * a.radius, n[2] * a.radius];
    const rb = [-n[0] * b.radius, -n[1] * b.radius, -n[2] * b.radius];
    const va = cross3(a.w, ra);
    const vb = cross3(b.w, rb);
    const rel = [
      b.v[0] + vb[0] - a.v[0] - va[0],
      b.v[1] + vb[1] - a.v[1] - va[1],
      b.v[2] + vb[2] - a.v[2] - va[2],
    ];
    const vn = dot3(rel, n);
    if (vn >= 0) return;

    const jn = -(1 + RESTITUTION) * vn / 2;
    applyImpulse(a, ra, [-n[0] * jn, -n[1] * jn, -n[2] * jn]);
    applyImpulse(b, rb, [n[0] * jn, n[1] * jn, n[2] * jn]);

    const vt = [rel[0] - n[0] * vn, rel[1] - n[1] * vn, rel[2] - n[2] * vn];
    const vtLen = Math.hypot(vt[0], vt[1], vt[2]);
    if (vtLen > 1e-4) {
      const t = [vt[0] / vtLen, vt[1] / vtLen, vt[2] / vtLen];
      const jt = Math.min(vtLen / 2, FRICTION * jn);
      applyImpulse(a, ra, [t[0] * jt, t[1] * jt, t[2] * jt]);
      applyImpulse(b, rb, [-t[0] * jt, -t[1] * jt, -t[2] * jt]);
    }

    a.asleep = false; b.asleep = false;
    a.sleepCount = 0; b.sleepCount = 0;
    if (report) report(jn);
  }

  // --- Settling: the part that guarantees the right number is showing --------

  // Which rest axis is currently pointing most nearly straight up — the face the
  // die would be read off if it stopped right now (the up *vertex*, for a d4).
  function upAxisIndex(die) {
    const axes = die.solid.restAxes;
    let idx = 0;
    let best = -Infinity;
    for (let i = 0; i < axes.length; i++) {
      const a = quatRotate(die.q, axes[i]);
      if (a[2] > best) { best = a[2]; idx = i; }
    }
    return idx;
  }

  // Keep the die's predetermined number on whichever face is currently up, by
  // renumbering the die rather than turning it.
  //
  // This is the whole answer to "the correction is visible". Earlier versions
  // rotated the die into the pose that showed the right number, and every way of
  // scheduling that rotation was visible somewhere: at rest it is a die spinning
  // on the table for no reason, mid-bounce it is better but still leaves a
  // residual, and every step it pins the die and stops it looking like it is
  // rolling. Renumbering moves nothing. The die tumbles exactly as the physics
  // says; only which glyph sits on which face changes, and at any speed where
  // that would be readable the answer has already stopped changing.
  //
  // The new labelling is a rotation symmetry of the solid applied to the
  // original numbering (see n5eDiceSolids's symmetries), not an arbitrary
  // shuffle: the die stays a legitimately numbered die, opposite faces still
  // sum to a constant, and no face ends up with a number twice.
  function maybeRelabel(die) {
    const upIdx = upAxisIndex(die);
    if (die.labelOfAxis[upIdx] === die.targetIdx) return; // already correct
    const perms = die.solid.symmetries;
    for (let i = 0; i < perms.length; i++) {
      const perm = perms[i];
      if (perm[die.targetIdx] !== upIdx) continue;
      // labelOfAxis is the inverse permutation: axis perm[t] shows axis t's
      // glyph, so the up axis ends up showing the target's number.
      for (let a = 0; a < perm.length; a++) die.labelOfAxis[perm[a]] = a;
      die.labelDirty = true;
      die.relabels++;
      return;
    }
    // No symmetry carries the target face to this one. Cannot happen for these
    // solids — their rotation groups are transitive on faces — and if it ever
    // did, beginSettle sees the labelling is still wrong and falls back to
    // rotating rather than showing a wrong number.
  }

  // Bring the die to its resting pose. Normally this is only a world-space
  // shortest arc taking the up axis exactly onto +Z — a fraction of a degree
  // that un-cocks a die which stopped leaning on a neighbour, and nothing more,
  // because maybeRelabel has already put the right number on that face without
  // moving anything.
  //
  // The body-space arc is the fallback for the case maybeRelabel could not
  // handle (no symmetry carries the target face to the up face — impossible for
  // these solids, but it must not silently show a wrong number if it happens).
  // That branch is the visible one, and it should never run.
  function beginSettle(die) {
    if (die.settle || die.settled) return;
    const solid = die.solid;
    const upIdx = upAxisIndex(die);

    let q1 = die.q;
    let axis = upIdx;
    if (die.labelOfAxis[upIdx] !== die.targetIdx) {
      const r = quatShortestArc(solid.restAxes[die.targetIdx], solid.restAxes[upIdx]);
      q1 = quatMul(die.q, r);
      axis = die.targetIdx;
      if (debug) console.warn("dice3d: fell back to rotating", die.kind, "onto", die.value);
    }
    const worldTarget = quatRotate(q1, solid.restAxes[axis]);
    const align = quatShortestArc(worldTarget, UP);
    const qFinal = quatNormalize(quatMul(align, q1));

    const restZ = solid.restHeight * die.radius;
    die.settle = {
      from: die.q.slice(),
      to: qFinal,
      fromP: die.p.slice(),
      toP: [
        Math.max(-halfW + die.radius, Math.min(halfW - die.radius, die.p[0])),
        Math.max(-halfH + die.radius, Math.min(halfH - die.radius, die.p[1])),
        restZ,
      ],
      // How far the die lifts at the halfway point, scaled by how much rotating
      // it actually has left to do. A die tipping onto another face physically
      // pivots over an edge and rises; without the lift, a residual correction
      // reads as the die spinning in place on the table, which is the tell.
      // Nothing left to correct means no lift at all.
      lift: die.radius * 0.28 * Math.min(1, arcAngle(die.q, qFinal) / 1.2),
      t: 0,
    };
    die.v[0] = die.v[1] = die.v[2] = 0;
    die.w[0] = die.w[1] = die.w[2] = 0;
    if (debug) {
      const age = performance.now() - die.spawnedAt;
      console.log("dice3d: settling", die.kind, "value", die.value,
        "after", age.toFixed(0) + "ms", age > DIE_DEADLINE_MS ? "(deadline)" : "(slept)",
        "correction", (arcAngle(die.q, qFinal) * 180 / Math.PI).toFixed(1) + "deg",
        "relabels", die.relabels, "upIdx", upIdx, "targetIdx", die.targetIdx);
    }
  }

  // Half the shortest-arc angle between two orientations, in radians — how much
  // rotating a settle still has to do, used to scale its tip.
  function arcAngle(a, b) {
    let d = Math.abs(a[0] * b[0] + a[1] * b[1] + a[2] * b[2] + a[3] * b[3]);
    if (d > 1) d = 1;
    return 2 * Math.acos(d);
  }

  function advanceSettle(die, dtMs) {
    const s = die.settle;
    s.t = Math.min(1, s.t + dtMs / SETTLE_SLERP_MS);
    // Ease-in-out: the die leans into the correction and arrives softly, rather
    // than jumping off the mark the instant it stops moving.
    const e = s.t * s.t * (3 - 2 * s.t);
    die.q = quatSlerp(s.from, s.to, e);
    die.p[0] = s.fromP[0] + (s.toP[0] - s.fromP[0]) * e;
    die.p[1] = s.fromP[1] + (s.toP[1] - s.fromP[1]) * e;
    // Sine hump: zero at both ends, s.lift at the midpoint.
    die.p[2] = s.fromP[2] + (s.toP[2] - s.fromP[2]) * e + s.lift * Math.sin(Math.PI * s.t);
    if (s.t >= 1) {
      die.settle = null;
      die.settled = true;
      die.asleep = true;
    }
  }

  // --- Loop -----------------------------------------------------------------

  function startLoop() {
    if (running) return;
    running = true;
    lastFrame = performance.now();
    accumulator = 0;
    rafID = requestAnimationFrame(frame);
  }

  function stopLoop() {
    running = false;
    if (rafID) cancelAnimationFrame(rafID);
    rafID = 0;
  }

  function frame(now) {
    rafID = 0;
    if (!running) return;

    const dtMs = Math.min(200, now - lastFrame);
    lastFrame = now;
    accumulator += dtMs / 1000;

    let steps = 0;
    while (accumulator >= STEP && steps < MAX_STEPS_PER_FRAME) {
      simulate(now, STEP);
      accumulator -= STEP;
      steps++;
    }
    if (steps === MAX_STEPS_PER_FRAME) accumulator = 0; // dropped frames: don't bank debt

    for (const d of dice) {
      if (d.settle) advanceSettle(d, dtMs);
    }

    render();

    if (activeRoll && dice.every((d) => d.settled)) finishRoll("settled");

    if (activeRoll || dice.some((d) => d.settle) || globalAlpha < 1) {
      rafID = requestAnimationFrame(frame);
    } else {
      running = false;
    }
  }

  function simulate(now, dt) {
    if (debug) debugPoints.length = 0;
    const report = activeRoll ? reportImpact : null;

    for (const d of dice) {
      if (!d.live) {
        if (activeRoll && now - activeRoll.startedAt >= d.startAt) {
          d.live = true;
          d.spawnedAt = now;
        } else {
          continue;
        }
      }
      if (d.settle || d.settled || d.asleep) continue;
      const age = now - d.spawnedAt;
      stepDie(d, dt, report, age);
      // Sleep detection OR the die's own deadline, whichever comes first. Waiting
      // only on sleep is what left rolls to be ended by the global watchdog.
      if (d.asleep || age > DIE_DEADLINE_MS) beginSettle(d);
    }

    for (let i = 0; i < dice.length; i++) {
      if (!dice[i].live || dice[i].settle || dice[i].settled) continue;
      for (let j = i + 1; j < dice.length; j++) {
        if (!dice[j].live || dice[j].settle || dice[j].settled) continue;
        collidePair(dice[i], dice[j], report);
      }
    }
  }

  function reportImpact(impulse) {
    const roll = activeRoll;
    if (!roll || !roll.onImpact) return;
    if (impulse < IMPACT_REF * 0.18) return;
    if (roll.impacts >= IMPACT_MAX_PER_ROLL) return;
    const now = performance.now();
    if (now - roll.lastImpactAt < IMPACT_MIN_GAP_MS) return;
    roll.lastImpactAt = now;
    roll.impacts++;
    try {
      roll.onImpact(Math.min(1, impulse / IMPACT_REF));
    } catch (e) {
      /* a broken sound hook must never break a roll */
    }
  }

  // --- Rendering ------------------------------------------------------------

  const rotTmp = new Float32Array(9);

  function setModel(die) {
    const m = quatToMat3(die.q);
    rotTmp.set(m);
    const R = die.radius;
    modelTmp[0] = m[0] * R; modelTmp[1] = m[1] * R; modelTmp[2] = m[2] * R; modelTmp[3] = 0;
    modelTmp[4] = m[3] * R; modelTmp[5] = m[4] * R; modelTmp[6] = m[5] * R; modelTmp[7] = 0;
    modelTmp[8] = m[6] * R; modelTmp[9] = m[7] * R; modelTmp[10] = m[8] * R; modelTmp[11] = 0;
    modelTmp[12] = die.p[0]; modelTmp[13] = die.p[1]; modelTmp[14] = die.p[2]; modelTmp[15] = 1;
    return rotTmp;
  }

  let bodyColor = [0.18, 0.52, 0.33];
  let numeralColor = [1, 1, 1];

  function refreshColors() {
    bodyColor = parseColor(cssVar("--dice-green", "#2e8555"), [0.18, 0.52, 0.33]);
    numeralColor = parseColor(cssVar("--dice-numeral", "#ffffff"), [1, 1, 1]);
  }

  function render() {
    if (!ready) return;
    gl.clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT);

    const visible = dice.filter((d) => d.live);

    // 1. Floor shadows. A soft blob is indistinguishable from a real shadow map
    // at a straight-down camera, and costs one quad.
    gl.useProgram(progBlob.program);
    gl.uniformMatrix4fv(progBlob.u.uViewProj, false, viewProj);
    gl.uniform1f(progBlob.u.uAlpha, globalAlpha);
    gl.uniform1f(progBlob.u.uSoft, 1);
    gl.bindBuffer(gl.ARRAY_BUFFER, blobBuf);
    gl.enableVertexAttribArray(progBlob.a.aQuad);
    gl.vertexAttribPointer(progBlob.a.aQuad, 2, gl.FLOAT, false, 0, 0);
    gl.depthMask(false);
    for (const d of visible) {
      const rest = d.solid.restHeight * d.radius;
      const lift = Math.max(0, d.p[2] - rest);
      const fade = Math.max(0.12, 1 - lift / (d.radius * 5));
      gl.uniform3f(progBlob.u.uCenter, d.p[0], d.p[1], 0.5);
      gl.uniform1f(progBlob.u.uSize, d.radius * (1.02 + lift / (d.radius * 6)));
      gl.uniform4f(progBlob.u.uColor, 0, 0, 0, 0.34 * fade);
      gl.drawArrays(gl.TRIANGLES, 0, 6);
    }

    if (debug && debugPoints.length) {
      gl.uniform1f(progBlob.u.uSoft, 0);
      gl.uniform1f(progBlob.u.uSize, 4);
      gl.uniform4f(progBlob.u.uColor, 1, 0.25, 0.25, 0.9);
      for (const pt of debugPoints) {
        gl.uniform3f(progBlob.u.uCenter, pt[0], pt[1], 1);
        gl.drawArrays(gl.TRIANGLES, 0, 6);
      }
    }
    gl.disableVertexAttribArray(progBlob.a.aQuad);
    gl.depthMask(true);

    // 2. Bodies. Pushed back by a polygon offset so the numerals, which float a
    // hair off each face, cannot z-fight with the face they belong to.
    gl.useProgram(progBody.program);
    gl.uniformMatrix4fv(progBody.u.uViewProj, false, viewProj);
    gl.uniform3fv(progBody.u.uColor, bodyColor);
    gl.uniform3fv(progBody.u.uLight, LIGHT);
    gl.uniform1f(progBody.u.uAlpha, globalAlpha);
    gl.enable(gl.POLYGON_OFFSET_FILL);
    gl.polygonOffset(1, 1);
    for (const d of visible) {
      const rot = setModel(d);
      const mesh = d.mesh;
      gl.uniformMatrix4fv(progBody.u.uModel, false, modelTmp);
      gl.uniformMatrix3fv(progBody.u.uRot, false, rot);
      gl.bindBuffer(gl.ARRAY_BUFFER, mesh.body.pos);
      gl.enableVertexAttribArray(progBody.a.aPos);
      gl.vertexAttribPointer(progBody.a.aPos, 3, gl.FLOAT, false, 0, 0);
      gl.bindBuffer(gl.ARRAY_BUFFER, mesh.body.normal);
      gl.enableVertexAttribArray(progBody.a.aNormal);
      gl.vertexAttribPointer(progBody.a.aNormal, 3, gl.FLOAT, false, 0, 0);
      gl.bindBuffer(gl.ARRAY_BUFFER, mesh.body.edge);
      gl.enableVertexAttribArray(progBody.a.aEdge);
      gl.vertexAttribPointer(progBody.a.aEdge, 1, gl.FLOAT, false, 0, 0);
      gl.drawArrays(gl.TRIANGLES, 0, mesh.body.count);
    }
    gl.disable(gl.POLYGON_OFFSET_FILL);
    gl.disableVertexAttribArray(progBody.a.aPos);
    gl.disableVertexAttribArray(progBody.a.aNormal);
    gl.disableVertexAttribArray(progBody.a.aEdge);

    // 3. Numerals: alpha-blended atlas quads, depth-tested against the body so a
    // numeral on the far side is hidden, but not depth-written so overlapping
    // glyph quads on adjacent faces cannot clip each other.
    gl.useProgram(progNumeral.program);
    gl.uniformMatrix4fv(progNumeral.u.uViewProj, false, viewProj);
    gl.uniform3fv(progNumeral.u.uColor, numeralColor);
    gl.uniform1f(progNumeral.u.uAlpha, globalAlpha);
    gl.activeTexture(gl.TEXTURE0);
    gl.bindTexture(gl.TEXTURE_2D, atlasTex);
    gl.uniform1i(progNumeral.u.uTex, 0);
    gl.depthMask(false);
    for (const d of visible) {
      const mesh = d.mesh;
      if (!mesh.numeral.count) continue;
      if (d.labelDirty) writeLabels(d);
      setModel(d);
      gl.uniformMatrix4fv(progNumeral.u.uModel, false, modelTmp);
      gl.bindBuffer(gl.ARRAY_BUFFER, mesh.numeral.pos);
      gl.enableVertexAttribArray(progNumeral.a.aPos);
      gl.vertexAttribPointer(progNumeral.a.aPos, 3, gl.FLOAT, false, 0, 0);
      gl.bindBuffer(gl.ARRAY_BUFFER, d.uvBuf);
      gl.enableVertexAttribArray(progNumeral.a.aUV);
      gl.vertexAttribPointer(progNumeral.a.aUV, 2, gl.FLOAT, false, 0, 0);
      gl.drawArrays(gl.TRIANGLES, 0, mesh.numeral.count);
    }
    gl.depthMask(true);
    gl.disableVertexAttribArray(progNumeral.a.aPos);
    gl.disableVertexAttribArray(progNumeral.a.aUV);
  }

  // --- Roll orchestration ---------------------------------------------------

  function clear() {
    // Resolve anything in flight first. clear() is reached both from the tray's
    // Clear Dice button and from the start of the next roll, and in either case
    // a pending promise left unresolved would strand its caller — dice-roller.js
    // awaits it and only re-enables the tray in the finally.
    finishRoll("cleared");
    // Each die owns a numeral UV buffer (see makeDie); without this every roll
    // would strand another poolful of them on the GPU.
    if (ready) {
      for (const d of dice) {
        if (d.uvBuf) gl.deleteBuffer(d.uvBuf);
      }
    }
    dice = [];
    globalAlpha = 1;
    if (fadeTimer) { clearTimeout(fadeTimer); fadeTimer = 0; }
    stopLoop();
    if (ready) {
      gl.clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT);
    }
  }

  function finishRoll(reason) {
    const roll = activeRoll;
    if (!roll) return;
    activeRoll = null;
    if (roll.watchdog) clearTimeout(roll.watchdog);
    if (debug) {
      console.log("dice3d: roll finished (" + reason + ") in "
        + (performance.now() - roll.startedAt).toFixed(0) + "ms", roll.detail);
    }
    // Settled dice linger, then fade out, so the screen does not stay cluttered
    // and the next roll starts on a clean table.
    if (fadeTimer) clearTimeout(fadeTimer);
    fadeTimer = setTimeout(beginFade, FADE_DELAY_MS);
    roll.resolve(roll.detail);
  }

  function beginFade() {
    fadeTimer = 0;
    if (activeRoll || !dice.length) return;
    const start = performance.now();
    const tick = () => {
      const t = Math.min(1, (performance.now() - start) / FADE_MS);
      globalAlpha = 1 - t;
      if (ready) render();
      if (t < 1) requestAnimationFrame(tick);
      else clear();
    };
    requestAnimationFrame(tick);
  }

  // groups: [{sides, count}], opts: {onImpact(strength)}.
  // Resolves with [{sides, values:[...]}] in the same order — the shape
  // dice-roller.js's showResult and the n5e:roll-result event already use.
  function roll(groups, opts) {
    const options = opts || {};
    const detail = [];
    let total = 0;
    for (const g of groups || []) {
      const count = Math.max(0, g.count | 0);
      const values = [];
      for (let i = 0; i < count; i++) values.push(randInt(g.sides));
      if (values.length) detail.push({ sides: g.sides, values });
      total += values.length;
    }
    if (!detail.length) return Promise.resolve([]);

    if (!ensureInit()) return Promise.reject(new Error("dice3d unavailable"));

    clear();
    refreshColors();
    resize();

    // Past this many bodies the animation stops being readable and starts being
    // a framerate problem; the values are already decided, so just report them.
    if (total > MAX_DICE) return Promise.resolve(detail);

    let index = 0;
    let bodyCount = 0;
    for (const group of detail) {
      bodyCount += group.sides === 100 ? group.values.length * 2 : group.values.length;
    }
    for (const group of detail) {
      for (const value of group.values) {
        if (group.sides === 100) {
          // Thrown as a real percentile pair: 00-90 plus 0-9, where 00 + 0 is
          // 100. Two bodies on screen, still one number reported.
          dice.push(makeDie("d100-tens", Math.floor(value / 10) % 10, index++, bodyCount));
          dice.push(makeDie("d100-units", value % 10, index++, bodyCount));
        } else {
          const kind = kindFor(group.sides);
          if (!kind) continue; // unknown die: no body, but its value is still reported
          dice.push(makeDie(kind, value, index++, bodyCount));
        }
      }
    }

    if (!dice.length) return Promise.resolve(detail);

    return new Promise((resolve) => {
      const pending = {
        resolve,
        detail,
        startedAt: performance.now(),
        onImpact: options.onImpact || null,
        impacts: 0,
        lastImpactAt: 0,
        watchdog: 0,
      };
      activeRoll = pending;
      // The hard cap. Independent of rAF, so a hidden tab, a lost context or a
      // die that never sleeps still produces a result at a predictable time.
      // The identity check matters: a watchdog that outlived its own roll must
      // never cut short whatever roll is running now.
      pending.watchdog = setTimeout(() => {
        if (activeRoll !== pending) return;
        if (debug) console.log("dice3d: watchdog fired at " + SETTLE_TIMEOUT_MS + "ms");
        for (const d of dice) {
          d.live = true;
          if (!d.settled && !d.settle) beginSettle(d);
        }
        finishRoll("watchdog");
        startLoop(); // the guided settle still has to be drawn
      }, SETTLE_TIMEOUT_MS);
      startLoop();
    });
  }

  let resizeTimer = 0;
  window.addEventListener("resize", () => {
    if (resizeTimer) clearTimeout(resizeTimer);
    resizeTimer = setTimeout(() => {
      resizeTimer = 0;
      resize();
      if (!running && dice.length && ready) render();
    }, 120);
  });

  window.n5eDice3D = {
    get available() { return ready || (!initFailed && probe()); },
    roll,
    clear,
    resize,
  };
})();
