"use strict";
const STATUS = {
  "regressed":   { ico: "✕", label: "Regressed" },
  "in-review":   { ico: "◉", label: "In review" },
  "in-progress": { ico: "◐", label: "In progress" },
  "planned":     { ico: "○", label: "Planned" },
  "stalled":     { ico: "⏸", label: "Stalled" },
  "done":        { ico: "✓", label: "Done" },
};
const SPEC_ORDER = ["regressed","in-review","in-progress","planned","stalled","done"];
const UNIT_ORDER = ["in-review","in-progress","stalled","done"];
const CLAIM_HEADS = {
  "ticket-done-but-open": ["✓", "done but still open"],
  "ticket-stale": ["⏸", "assigned, gone quiet"],
  "unticketed-work": ["?", "work nobody promised"],
  "pr-abandoned": ["✕", "closed without merging"],
};
const esc = s => String(s ?? "").replace(/[&<>"']/g, c => ({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[c]));
const sv = st => `var(--${esc(st)})`;

/* Epic identity: color follows the name (stable hash into 8 categorical
   slots), so filtering or new epics never repaint existing ones. */
function epicColor(name) {
  let h = 0;
  for (const ch of name) h = (h * 31 + ch.codePointAt(0)) >>> 0;
  return `var(--cat${(h % 8) + 1})`;
}
const epicTag = name => name ? `<span class="tag"><i class="dot" style="background:${epicColor(name)}"></i>${esc(name)}</span>` : "";
const TYPE_ICO = { story: "✦", bug: "✗", task: "⚙" };
const typeTag = t => (t && t !== "story") ? `<span class="tag type-${esc(t)}">${TYPE_ICO[t] || ""} ${esc(t)}</span>` : "";

/* ---------- theme: system → light → dark ---------- */
const THEMES = [["", "◐ auto"], ["light", "☀ light"], ["dark", "☾ dark"]];
function applyTheme(t) {
  if (t) document.documentElement.dataset.theme = t;
  else delete document.documentElement.dataset.theme;
  document.getElementById("theme").textContent = (THEMES.find(x => x[0] === t) || THEMES[0])[1];
}
applyTheme(localStorage.getItem("tb-theme") || "");
document.getElementById("theme").addEventListener("click", () => {
  const cur = localStorage.getItem("tb-theme") || "";
  const next = THEMES[(THEMES.findIndex(x => x[0] === cur) + 1) % THEMES.length][0];
  localStorage.setItem("tb-theme", next);
  applyTheme(next);
});

/* ---------- filters ---------- */
const F = { text: "", epics: new Set(), sprints: new Set(), owners: new Set(), types: new Set(), older: false };
const filterActive = () => F.text !== "" || F.epics.size > 0 || F.sprints.size > 0 || F.owners.size > 0 || F.types.size > 0;
function specMatches(s) {
  if (F.text && !(s.title + " " + s.id).toLowerCase().includes(F.text)) return false;
  if (F.epics.size && !F.epics.has(s.epic || "")) return false;
  if (F.sprints.size && !F.sprints.has(s.sprint || "")) return false;
  if (F.owners.size && !F.owners.has(s.owner || "")) return false;
  if (F.types.size && !F.types.has(s.type || "story")) return false;
  return true;
}
function syncFilterChips(b) {
  const epics = [...new Set((b.specs || []).map(s => s.epic).filter(Boolean))].sort();
  const epicPts = {};
  for (const s of b.specs || []) if (s.epic && s.points) epicPts[s.epic] = (epicPts[s.epic] || 0) + s.points;
  document.getElementById("f-epics").innerHTML = epics.map(e =>
    `<button class="fchip${F.epics.has(e) ? " on" : ""}" data-epic="${esc(e)}">
      <span class="dot" style="background:${epicColor(e)}"></span>${esc(e)}${epicPts[e] ? ` · ${epicPts[e]}pt` : ""}</button>`).join("");
  const sprints = [...new Set((b.specs || []).map(s => s.sprint).filter(Boolean))].sort().reverse();
  document.getElementById("f-sprints").innerHTML = sprints.map(sp =>
    `<button class="fchip${F.sprints.has(sp) ? " on" : ""}" data-sprint="${esc(sp)}">${esc(sp)}</button>`).join("");
  const owners = [...new Set((b.specs || []).map(s => s.owner).filter(Boolean))].sort();
  let ownerChips = owners.map(o =>
    `<button class="fchip${F.owners.has(o) ? " on" : ""}" data-owner="${esc(o)}">${esc(initials(o))} · ${esc(o)}</button>`);
  if ((b.specs || []).some(s => !s.owner))
    ownerChips.push(`<button class="fchip${F.owners.has("") ? " on" : ""}" data-owner="">∅ unassigned</button>`);
  document.getElementById("f-owners").innerHTML = ownerChips.length > 1 ? ownerChips.join("") : "";
  const types = [...new Set((b.specs || []).map(s => s.type || "story"))];
  document.getElementById("f-types").innerHTML = types.length > 1 ? ["story","bug","task"].filter(t => types.includes(t)).map(t =>
    `<button class="fchip${F.types.has(t) ? " on" : ""}" data-type="${t}">${TYPE_ICO[t]} ${t}</button>`).join("") : "";
  document.getElementById("f-clear").hidden = !filterActive();
}
document.getElementById("f-text").addEventListener("input", e => {
  F.text = e.target.value.trim().toLowerCase(); rerender();
});
document.querySelector(".filters").addEventListener("click", e => {
  const chip = e.target.closest(".fchip");
  if (!chip) return;
  const toggle = (set, v) => set.has(v) ? set.delete(v) : set.add(v);
  if (chip.dataset.epic !== undefined) toggle(F.epics, chip.dataset.epic);
  else if (chip.dataset.sprint !== undefined) toggle(F.sprints, chip.dataset.sprint);
  else if (chip.dataset.owner !== undefined) toggle(F.owners, chip.dataset.owner);
  else if (chip.dataset.type !== undefined) toggle(F.types, chip.dataset.type);
  else return;
  rerender();
});
document.getElementById("f-clear").addEventListener("click", () => {
  F.text = ""; F.epics.clear(); F.sprints.clear(); F.owners.clear(); F.types.clear();
  document.getElementById("f-text").value = "";
  rerender();
});
const chip = st => { const d = STATUS[st] || { ico: "·", label: st };
  return `<span class="status" style="color:${sv(st)}">${d.ico} ${esc(d.label)}</span>`; };
const initials = name => (name || "").split(/[\s.-]+/).filter(Boolean).slice(0,2).map(w=>w[0]).join("") || "·";

/* ---------- tiny safe markdown: escape first, then transform ---------- */
function inlineMd(s) {
  return s.replace(/`([^`]+)`/g, "<code>$1</code>")
          .replace(/\*\*([^*]+)\*\*/g, "<b>$1</b>")
          .replace(/(^|\s)\*([^*]+)\*(?=[\s.,;:!?)]|$)/g, "$1<i>$2</i>");
}
// interactive: task checkboxes become live sign-off controls carrying
// their ordinal (data-ti) so a click can flip the right [ ] in the body.
function md(src, interactive = false) {
  const lines = esc(src ?? "").split("\n");
  const out = []; let code = false, list = false, para = [], task = 0;
  const flushP = () => { if (para.length) { out.push("<p>" + inlineMd(para.join(" ")) + "</p>"); para = []; } };
  const closeL = () => { if (list) { out.push("</ul>"); list = false; } };
  for (const line of lines) {
    if (line.startsWith("```")) { flushP(); closeL(); code = !code; out.push(code ? "<pre><code>" : "</code></pre>"); continue; }
    if (code) { out.push(line + "\n"); continue; }
    let m;
    if ((m = line.match(/^#{1,4}\s+(.*)/))) { flushP(); closeL(); out.push("<h3>" + inlineMd(m[1]) + "</h3>"); continue; }
    if ((m = line.match(/^\s*[-*]\s+\[([ xX])\]\s+(.*)/))) {
      flushP(); if (!list) { out.push("<ul>"); list = true; }
      const attrs = interactive ? ` data-ti="${task++}"` : " disabled";
      out.push(`<li class="task"><label><input type="checkbox"${attrs}${m[1] !== " " ? " checked" : ""}>${inlineMd(m[2])}</label></li>`); continue;
    }
    if ((m = line.match(/^\s*[-*]\s+(.*)/))) {
      flushP(); if (!list) { out.push("<ul>"); list = true; }
      out.push("<li>" + inlineMd(m[1]) + "</li>"); continue;
    }
    if (!line.trim()) { flushP(); closeL(); continue; }
    // Lazy continuation: a wrapped line inside a list belongs to its item.
    if (list && out.length && out[out.length - 1].endsWith("</li>")) {
      out[out.length - 1] = out[out.length - 1].replace(/<\/li>$/, " " + inlineMd(line.trim()) + "</li>");
      continue;
    }
    para.push(line.trim());
  }
  flushP(); closeL(); if (code) out.push("</code></pre>");
  return out.join("");
}

/* ---------- board rendering ---------- */
let lastBoard = null;

function tiles(b) {
  const n = st => (b.specs || []).filter(s => s.status === st).length;
  const active = n("in-progress") + n("in-review");
  const d = b.drift || {};
  const drift = (d.stale_promises?.length||0) + (d.shadow_work?.length||0) +
                (d.scope_creep?.length||0) + (b.claims?.length||0) + n("regressed");
  const tile = (num, label, color) =>
    `<div class="tile"><div class="num">${num}</div>
      <div class="lbl"><span class="mark" style="background:${color}"></span>${label}</div></div>`;
  const pts = (b.specs || []).reduce((a, s) => { if (s.points) { a.total += s.points; if (s.status === "done") a.done += s.points; } return a; }, { done: 0, total: 0 });
  return `<div class="tiles">` +
    tile(n("done"), "done", "var(--done)") +
    (pts.total ? tile(`${pts.done}/${pts.total}`, "points landed", "var(--done)") : "") +
    tile(active, "in flight", "var(--in-progress)") +
    tile(n("planned"), "planned", "var(--planned)") +
    tile(drift, "drift findings", drift ? "var(--stalled)" : "var(--done)") +
    `</div>`;
}

function cardHTML(s, st) {
  const total = s.acceptance_total || 0, done = s.acceptance_done || 0;
  const pct = total ? Math.round(100 * done / total) : 0;
  const prog = total ? `<span class="prog"><span class="bar"><i class="${pct===100?"full":""}" style="width:${pct}%"></i></span><span class="n">${done}/${total}</span></span>` : `<span class="prog"></span>`;
  return `<div class="card" data-spec="${esc(s.id)}" style="border-left-color:${sv(st)}; view-transition-name: c-${esc(s.id)}">
    <div class="title">${esc(s.title)}</div>
    <div class="chips"><code>${esc(s.id)}</code>${
      s.priority ? `<span class="tag pri">p${esc(s.priority)}</span>` : ""}${
      s.points ? `<span class="tag pts">${esc(s.points)}pt</span>` : ""}${typeTag(s.type)}${epicTag(s.epic)}${
      s.sprint ? `<span class="tag sprint">${esc(s.sprint)}</span>` : ""}</div>
    <div class="ev">${esc(s.evidence)}</div>
    <div class="cfoot"><span class="avatar" title="${esc(s.owner || "unowned")}">${esc(initials(s.owner))}</span>${prog}</div>
  </div>`;
}

function kanban(b) {
  if (!b.specs?.length)
    return `<section class="panel"><h2>Spec board</h2>
      <div class="empty-col">No stories yet — click <b>+ New story</b>, or <code>truthboard spec new "Title"</code></div></section>`;
  const visible = b.specs.filter(specMatches);
  if (!visible.length)
    return `<section class="panel"><h2>Spec board</h2>
      <div class="empty-col">No stories match the filters.</div></section>`;
  const recent = new Set((b.shipped || []).map(s => s.id));
  const cols = SPEC_ORDER.filter(st => visible.some(s => s.status === st));
  return `<div class="board">` + cols.map(st => {
    const d = STATUS[st];
    let specs = visible.filter(s => s.status === st);
    let older = "";
    // Focus: the done column shows only recently-landed stories unless
    // expanded — but never hide what a filter is explicitly looking for.
    if (st === "done" && !filterActive() && recent.size) {
      const old = specs.filter(s => !recent.has(s.id));
      if (old.length && !F.older) {
        specs = specs.filter(s => recent.has(s.id));
        older = `<button class="older" id="show-older">show ${old.length} older</button>`;
      } else if (old.length) {
        older = `<button class="older" id="show-older">hide older</button>`;
      }
    }
    return `<div class="col"><h3 style="color:${sv(st)}">${d.ico} ${esc(d.label)}
      <span class="count">${specs.length}</span></h3>${specs.map(s => cardHTML(s, st)).join("")}${older}</div>`;
  }).join("") + `</div>`;
}

// Sprints are arithmetic over the same derived statuses as the board —
// a sprint "finishes" when its stories land, and there is nothing to set.
function sprintsPanel(b) {
  if (!b.sprints?.length) return "";
  const rows = b.sprints.map(sp => {
    const pct = sp.total ? Math.round(100 * sp.done / sp.total) : 0;
    const open = (sp.open || []).map(o =>
      `<span class="spopen" style="color:${sv(o.status)}">${(STATUS[o.status]||{}).ico || ""}</span> <code>${esc(o.id)}</code> <span class="spopen">${esc(o.title)}</span>`
    ).join(" ");
    let window = "";
    if (sp.state) {
      const left = sp.state === "active" ? (sp.days_left ? ` · ${sp.days_left}d left` : " · ends today") : "";
      window = `<span class="spwindow" title="${esc(sp.start)} → ${esc(sp.end)}">${esc(sp.start)} → ${esc(sp.end)} · <b class="sp-${esc(sp.state)}">${esc(sp.state)}</b>${left}</span>`;
    }
    return `<div class="sprow"><span class="spname">${esc(sp.name)}</span>
      <span class="spbar"><i class="${pct===100?"full":""}" style="width:${pct}%"></i></span>
      <span class="spn">${sp.done}/${sp.total} done${sp.points_total ? ` · ${sp.points_done || 0}/${sp.points_total} pts` : ""}${sp.unestimated && sp.points_total ? ` (+${sp.unestimated} unest.)` : ""}</span>${window}${open}</div>`;
  }).join("");
  return `<section class="panel"><h2>Sprints — derived, a sprint finishes when its stories land</h2>${rows}</section>`;
}

function drift(b) {
  const d = b.drift || {};
  const out = [];
  for (const sc of d.scope_creep || [])
    out.push(`<div class="finding"><span class="ico" style="color:var(--stalled)">⇢</span>
      <span class="what"><b>Scope creep</b> — <code>${esc(sc.spec)}</code> / <code>${esc(sc.branch)}</code>:
      ${Math.round(100*sc.outside_files/sc.total_files)}% of the diff outside spec paths (mostly ${esc(sc.top_dirs)})</span></div>`);
  for (const u of d.stale_promises || [])
    out.push(`<div class="finding"><span class="ico" style="color:var(--stalled)">⏸</span>
      <span class="what"><b>Stale promise</b> — <code>${esc(u.name)}</code>: ${esc(u.evidence)}</span></div>`);
  const sw = d.shadow_work || [];
  sw.slice(0, 6).forEach(c => out.push(
    `<div class="finding"><span class="ico" style="color:var(--muted)">∅</span>
      <span class="what"><b>Shadow work</b> — ${esc(c.subject)} <code>${esc(c.hash)}</code></span></div>`));
  if (sw.length > 6) out.push(`<div class="more">… and ${sw.length - 6} more shadow commits</div>`);
  return `<section class="panel"><h2>Drift — where the board could lie</h2>
    ${out.length ? out.join("") : `<span class="clean">clean — the board matches reality</span>`}</section>`;
}

function claims(b) {
  if (!b.forge) return "";
  const out = [];
  for (const [kind, [ico, head]] of Object.entries(CLAIM_HEADS)) {
    for (const c of (b.claims || []).filter(x => x.kind === kind).slice(0, 8))
      out.push(`<div class="finding"><span class="ico" style="color:var(--stalled)">${ico}</span>
        <span class="what"><b>${esc(c.subject)}</b> ${esc(head)} — ${esc(c.detail)}</span></div>`);
  }
  return `<section class="panel"><h2>Claims vs proof — ${esc(b.forge)}</h2>
    ${out.length ? out.join("") : `<span class="clean">every tracker claim is backed by the repo</span>`}</section>`;
}

function branches(b) {
  if (!b.units?.length) return "";
  const rows = UNIT_ORDER.map(st => b.units.filter(u => u.status === st).map(u =>
    `<div class="r">${chip(u.status)}<code>${esc(u.name)}</code>
     <span class="ev2">${esc(u.evidence)}${(u.flags||[]).map(f=>` — ⚠ ${esc(f)}`).join("")}</span></div>`
  ).join("")).join("");
  return `<section class="panel"><h2>Branches</h2><div class="rows">${rows}</div></section>`;
}

function digest(b) {
  const torder = { story: 0, bug: 1, task: 2 };
  const byType = [...(b.shipped || [])].sort((a, c) => (torder[a.type || "story"] || 0) - (torder[c.type || "story"] || 0));
  const shipped = byType.map(s =>
    `<div class="r"><time>${esc(s.date)}</time>
      <span style="color:var(--done)">✓</span>
      <span><b>${esc(s.title)}</b> ${typeTag(s.type)} <span style="color:var(--muted)"><code>${esc(s.id)}</code>${s.epic ? " · " + esc(s.epic) : ""}</span></span></div>`).join("");
  const rest = (b.digest || []).filter(c => !c.spec).slice(0, 12).map(c =>
    `<div class="r"><time>${esc(c.date)}</time><span class="ev2">${esc(c.subject)}</span></div>`).join("");
  const divider = shipped && rest ? `<div class="r"><span class="ev2" style="font-size:.7rem">also landed</span></div>` : "";
  return `<section class="panel digest"><h2>Landed in the last ${b.digest_days} days</h2>
    <div class="rows">${(shipped + divider + rest) || `<span class="ev2">nothing landed</span>`}</div></section>`;
}

function render(b) {
  lastBoard = b;
  const repoLabel = b.forge || (b.repo || "").split("/").filter(Boolean).pop() || b.repo;
  document.getElementById("meta").textContent =
    `${repoLabel} · integration branch ${b.integration_branch} (${b.elected_via})`;
  syncFilterChips(b);
  let html = "";
  if (b.election_note) html += `<div class="warn">⚠ ${esc(b.election_note)}</div>`;
  html += tiles(b) + kanban(b) + sprintsPanel(b);
  html += `<div class="grid2">` + drift(b) + claims(b) + `</div>`;
  html += `<div class="grid2">` + branches(b) + digest(b) + `</div>`;
  document.getElementById("app").innerHTML = html;
}

// rerender re-draws from the cached board; card moves animate when the
// browser supports View Transitions (pure enhancement otherwise).
function rerender() {
  if (!lastBoard) return;
  if (document.startViewTransition) document.startViewTransition(() => render(lastBoard));
  else render(lastBoard);
}
document.getElementById("app").addEventListener("click", e => {
  if (e.target.id === "show-older") { F.older = !F.older; rerender(); }
});

let syncAt = "";
function ago(iso) {
  const s = Math.max(0, Math.round((Date.now() - new Date(iso)) / 1000));
  return s < 90 ? `${s}s ago` : `${Math.round(s / 60)}m ago`;
}

// A board shared beyond this machine has no auth story, so it shows the
// truth and edits nothing — the server refuses writes; we hide the doors.
let RO = false;
function setReadOnly(ro) {
  RO = ro;
  document.getElementById("new-story").hidden = ro;
  document.getElementById("dt-edit").hidden = ro;
  document.getElementById("dt-assign-wrap").hidden = ro;
}

let last = "";
let ticking = false;
async function tick(reschedule = true) {
  if (ticking) return; // an SSE nudge during a poll: that poll already sees the new state
  ticking = true;
  try {
    const r = await fetch("/api/board");
    if (!r.ok) throw new Error(r.statusText);
    const dirty = parseInt(r.headers.get("X-Truthboard-Dirty") || "0", 10);
    const dirtyEl = document.getElementById("dirty");
    dirtyEl.hidden = !dirty;
    if (dirty) dirtyEl.textContent = `● ${dirty} uncommitted intent change${dirty > 1 ? "s" : ""} — review and commit .truthboard/specs like code`;
    setReadOnly(r.headers.get("X-Truthboard-Readonly") === "1");
    // The serving binary's version, so a stale board is visible at a
    // glance (`truthboard update`, then stop && ui --detach).
    const v = r.headers.get("X-Truthboard-Version");
    if (v) document.getElementById("foot").textContent = `truthboard ${v} · refreshes automatically`;
    syncAt = r.headers.get("X-Truthboard-Sync-At") || "";
    const syncErr = r.headers.get("X-Truthboard-Sync-Err");
    const syncNote = r.headers.get("X-Truthboard-Sync-Note");
    const syncEl = document.getElementById("sync");
    // A failing fetch or a skipped fast-forward must read as staleness,
    // never as a quiet repo.
    if (syncErr) { syncEl.hidden = false; syncEl.textContent = `⚠ remote sync failing: ${syncErr}`; }
    else if (syncNote) { syncEl.hidden = false; syncEl.textContent = `⚠ branch statuses are fresh, but story files are not: ${syncNote}`; }
    else syncEl.hidden = true;
    const text = await r.text();
    // generated_at changes on every audit; comparing without it keeps
    // unchanged boards from re-rendering (and cross-fading) every poll.
    const key = text.replace(/"generated_at":"[^"]+"/, "");
    // Never re-render under an open dialog — the next poll after it
    // closes picks the change up.
    if (key !== last && !dlg.open && !detailDlg.open) {
      last = key;
      const b = JSON.parse(text);
      if (lastBoard && document.startViewTransition) document.startViewTransition(() => render(b));
      else render(b);
    }
    document.getElementById("updated").textContent = "live · " + new Date().toLocaleTimeString() +
      (syncAt ? " · remote synced " + ago(syncAt) : "");
  } catch (e) {
    document.getElementById("updated").textContent = "audit unavailable — retrying";
  }
  ticking = false;
  if (reschedule) setTimeout(tick, 3000);
}
tick();

// Server push: a webhook-armed board announces pushes over SSE, so the
// page refreshes the moment work lands instead of on the next poll. The
// poll above keeps running regardless — SSE is an accelerator, not a
// dependency.
try {
  const es = new EventSource("/api/events");
  es.onmessage = () => tick(false);
} catch (e) { /* no EventSource, polling covers it */ }

/* ---------- detail view ---------- */
const detailDlg = document.getElementById("detail");
let detailSpec = null;

function openDetail(full) {
  detailSpec = full;
  const onBoard = (lastBoard?.specs || []).find(x => x.id === full.id) || {};
  const st = onBoard.status || "planned";
  document.getElementById("dt-status").outerHTML = `<span class="status" id="dt-status" style="color:${sv(st)}">${(STATUS[st]||{}).ico || ""} ${esc((STATUS[st]||{}).label || st)}</span>`;
  document.getElementById("dt-title").textContent = full.title;
  document.getElementById("dt-chips").innerHTML =
    `<code>${esc(full.id)}</code>` +
    (full.priority ? `<span class="tag pri">p${esc(full.priority)}</span>` : "") +
    (full.points ? `<span class="tag pts">${esc(full.points)}pt</span>` : "") +
    typeTag(full.type) +
    epicTag(full.epic) +
    (full.sprint ? `<span class="tag sprint">${esc(full.sprint)}</span>` : "") +
    (full.owner ? `<span class="tag">${esc(full.owner)}</span>` : "");
  document.getElementById("dt-assign-wrap").hidden = RO;
  document.getElementById("dt-assign").value = full.owner || "";
  document.getElementById("dt-assign-note").textContent = "";
  document.getElementById("owners-known").innerHTML =
    [...new Set((lastBoard?.specs || []).map(x => x.owner).filter(Boolean))].sort()
      .map(o => `<option value="${esc(o)}">`).join("");
  document.getElementById("dt-md").innerHTML = md(full.body, true);
  if (RO) document.querySelectorAll("#dt-md input[type=checkbox]").forEach(b => { b.disabled = true; });
  document.getElementById("dt-hint").hidden = RO || !/^\s*[-*]\s+\[[ xX]\]/m.test(full.body);
  const rows = [["Status", `${esc(onBoard.evidence || "no matching branch or commit yet")}`]];
  if (onBoard.branches?.length) rows.push(["Branches", onBoard.branches.map(x=>`<code>${esc(x)}</code>`).join(" ")]);
  if (onBoard.landed) rows.push(["Landed", `<code>${esc(onBoard.landed.slice(0,7))}</code>`]);
  rows.push(["Linking", `any branch containing <code>${esc(full.id)}</code> · trailer <code>Spec: ${esc(full.id)}</code>` +
    (full.branch ? ` · glob <code>${esc(full.branch)}</code>` : "")]);
  if (full.paths?.length) rows.push(["Scope", full.paths.map(x=>`<code>${esc(x)}</code>`).join(" ")]);
  document.getElementById("dt-truth").innerHTML = `<h4>Derived truth — computed, not editable</h4>` +
    rows.map(([k,v]) => `<div class="kv"><b>${k}</b><span>${v}</span></div>`).join("");
  detailDlg.showModal();
}

document.getElementById("app").addEventListener("click", async e => {
  const card = e.target.closest("[data-spec]");
  if (!card) return;
  try {
    const r = await fetch("/api/specs/" + encodeURIComponent(card.dataset.spec));
    if (!r.ok) throw new Error(await r.text());
    openDetail(await r.json());
  } catch (err) { console.error(err); }
});
document.getElementById("dt-assign").addEventListener("change", async e => {
  if (!detailSpec || RO) return;
  const owner = e.target.value.trim();
  if (owner === (detailSpec.owner || "")) return;
  const note = document.getElementById("dt-assign-note");
  try {
    const r = await fetch("/api/specs/" + encodeURIComponent(detailSpec.id), {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ owner }),
    });
    if (!r.ok) throw new Error(await r.text());
    detailSpec.owner = owner;
    note.textContent = owner ? `assigned to ${owner}` : "unassigned";
    last = ""; // avatars and owner chips pick it up on the next poll
  } catch (err) {
    e.target.value = detailSpec.owner || "";
    note.textContent = "could not assign: " + err.message;
  }
});

document.getElementById("dt-close").addEventListener("click", () => detailDlg.close());
document.getElementById("dt-edit").addEventListener("click", () => { detailDlg.close(); openEditor(detailSpec); });

// Sign-off: clicking the nth checkbox flips the nth [ ]/[x] in the body
// and saves — an intent edit like any other, visible as a git diff.
document.getElementById("dt-md").addEventListener("change", async e => {
  const box = e.target.closest("input[type=checkbox][data-ti]");
  if (!box || !detailSpec) return;
  if (RO) { box.checked = !box.checked; return; }
  const idx = +box.dataset.ti;
  let i = -1;
  const newBody = detailSpec.body.replace(/^(\s*[-*]\s+)\[([ xX])\]/gm, (m, pre, mark) => {
    i++;
    return i === idx ? pre + (mark === " " ? "[x]" : "[ ]") : m;
  });
  box.disabled = true;
  try {
    const r = await fetch("/api/specs/" + encodeURIComponent(detailSpec.id), {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ body: newBody }),
    });
    if (!r.ok) throw new Error(await r.text());
    detailSpec.body = newBody;
    last = ""; // progress bars pick it up on the next poll
  } catch (err) {
    box.checked = !box.checked;
    document.getElementById("dt-hint").hidden = false;
    document.getElementById("dt-hint").textContent = "Could not save sign-off: " + err.message;
  } finally {
    box.disabled = false;
  }
});

/* ---------- editor ---------- */
const dlg = document.getElementById("editor");
let editingId = null;
const TEMPLATE = "## Goal\n\n(what outcome, and why)\n\n## Acceptance\n\n- [ ] (observable criterion)\n";

function openEditor(spec) {
  editingId = spec ? spec.id : null;
  document.getElementById("ed-title").textContent = spec ? `Edit ${spec.id} — intent only` : "New story";
  document.getElementById("ed-t").value = spec?.title || "";
  document.getElementById("ed-o").value = spec?.owner || "";
  document.getElementById("ed-e").value = spec?.epic || "";
  document.getElementById("ed-sp").value = spec?.sprint || "";
  document.getElementById("ed-p").value = String(spec?.priority || 0);
  document.getElementById("ed-pts").value = spec?.points || "";
  document.getElementById("ed-ty").value = (spec?.type === "story" ? "" : spec?.type) || "";
  document.getElementById("ed-b").value = spec?.body ?? TEMPLATE;
  document.getElementById("ed-err").textContent = "";
  setTab(false);
  dlg.showModal();
}

function setTab(preview) {
  document.getElementById("tab-write").classList.toggle("on", !preview);
  document.getElementById("tab-preview").classList.toggle("on", preview);
  document.getElementById("ed-b").hidden = preview;
  const pv = document.getElementById("ed-preview");
  pv.hidden = !preview;
  if (preview) pv.innerHTML = md(document.getElementById("ed-b").value) || `<p style="color:var(--muted)">Nothing to preview.</p>`;
}
document.getElementById("tab-write").addEventListener("click", () => setTab(false));
document.getElementById("tab-preview").addEventListener("click", () => setTab(true));

document.getElementById("new-story").addEventListener("click", () => openEditor(null));

document.getElementById("ed-form").addEventListener("submit", async e => {
  if (e.submitter?.value === "cancel") return;
  e.preventDefault();
  const payload = {
    title: document.getElementById("ed-t").value.trim(),
    owner: document.getElementById("ed-o").value.trim(),
    epic: document.getElementById("ed-e").value.trim(),
    sprint: document.getElementById("ed-sp").value.trim(),
    priority: parseInt(document.getElementById("ed-p").value, 10) || 0,
    points: parseInt(document.getElementById("ed-pts").value, 10) || 0,
    type: document.getElementById("ed-ty").value,
    body: document.getElementById("ed-b").value,
  };
  try {
    const r = await fetch(editingId ? "/api/specs/" + encodeURIComponent(editingId) : "/api/specs", {
      method: editingId ? "PUT" : "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
    if (!r.ok) throw new Error(await r.text());
    dlg.close();
    last = ""; // force re-render on next poll
  } catch (err) {
    document.getElementById("ed-err").textContent = err.message;
  }
});
