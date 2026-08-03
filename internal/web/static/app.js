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
/* A hold is the one field a human writes that git cannot produce, so it is
   never shown alone: when the evidence contradicts it, the contradiction
   is part of the label. Showing the bare note would let a reason that has
   stopped being true keep passing as current. */
function holdTag(s) {
  if (!s.hold) return "";
  return s.hold_contradicted
    ? `<span class="tag hold stale" title="${esc(s.hold_contradicted)}">! held: ${esc(s.hold)}</span>`
    : `<span class="tag hold" title="on hold">⏸ ${esc(s.hold)}</span>`;
}
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
const F = { text: "", epics: new Set(), sprints: new Set(), owners: new Set(), types: new Set(), repos: new Set(), older: false };
const filterActive = () => F.text !== "" || F.epics.size > 0 || F.sprints.size > 0 || F.owners.size > 0 || F.types.size > 0 || F.repos.size > 0;

/* Repo dimension: a branch label's prefix is its repo ("api:feature/…"),
   unprefixed means the hub. Branch names cannot contain ":", so the split
   is safe. */
const branchRepo = label => { const i = label.indexOf(":"); return i > 0 ? label.slice(0, i) : "hub"; };
/* A story belongs to a repo through declared repos: intent, a per-repo
   landing entry, a linked branch living there, or the landing itself. */
function specRepos(s) {
  const out = new Set(s.repos || []);
  for (const pr of s.per_repo || []) out.add(pr.repo);
  for (const br of s.branches || []) out.add(branchRepo(br));
  if (s.landed) out.add(s.landed_repo || "hub");
  return out;
}
const unitRepo = u => u.repo || "hub";
const repoOn = r => !F.repos.size || F.repos.has(r);

function specMatches(s) {
  if (F.text && !(s.title + " " + s.id).toLowerCase().includes(F.text)) return false;
  if (F.epics.size && !F.epics.has(s.epic || "")) return false;
  if (F.sprints.size && !F.sprints.has(s.sprint || "")) return false;
  if (F.owners.size && !F.owners.has(s.owner || "")) return false;
  if (F.types.size && !F.types.has(s.type || "story")) return false;
  if (F.repos.size && ![...specRepos(s)].some(r => F.repos.has(r))) return false;
  return true;
}
function syncFilterChips(b) {
  // Repo chips exist only when a workspace does — single-repo boards see
  // no new UI at all.
  const repos = b.workspace?.length ? ["hub", ...b.workspace.map(r => r.name)] : [];
  document.getElementById("f-repos").innerHTML = repos.map(r =>
    `<button class="fchip${F.repos.has(r) ? " on" : ""}" data-repo="${esc(r)}">${r === "hub" ? "⌂ " : ""}${esc(r)}</button>`).join("");
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
  else if (chip.dataset.repo !== undefined) toggle(F.repos, chip.dataset.repo);
  else return;
  rerender();
});
document.getElementById("f-clear").addEventListener("click", () => {
  F.text = ""; F.epics.clear(); F.sprints.clear(); F.owners.clear(); F.types.clear(); F.repos.clear();
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
                (d.scope_creep?.length||0) + (d.unknown_repos?.length||0) +
                (b.claims?.length||0) + n("regressed");
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
      s.points ? `<span class="tag pts">${esc(s.points)}pt</span>` : ""}${typeTag(s.type)}${
      s.waiting?.length ? `<span class="tag wait" title="waiting on ${esc(s.waiting.join(", "))}">⧗ ${esc(s.waiting.join(" "))}</span>` : ""}${epicTag(s.epic)}${
      s.sprint ? `<span class="tag sprint">${esc(s.sprint)}</span>` : ""}${holdTag(s)}</div>
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
  // Unknown repos belong to no chip by definition — always shown.
  for (const ur of d.unknown_repos || [])
    out.push(`<div class="finding"><span class="ico" style="color:var(--regressed, #e5534b)">✗</span>
      <span class="what"><b>Unknown repo</b> — ${esc(ur)}</span></div>`);
  for (const sc of (d.scope_creep || []).filter(sc => repoOn(branchRepo(sc.branch))))
    out.push(`<div class="finding"><span class="ico" style="color:var(--stalled)">⇢</span>
      <span class="what"><b>Scope creep</b> — <code>${esc(sc.spec)}</code> / <code>${esc(sc.branch)}</code>:
      ${Math.round(100*sc.outside_files/sc.total_files)}% of the diff outside spec paths (mostly ${esc(sc.top_dirs)})</span></div>`);
  // A hold note the evidence disagrees with is drift in the same sense as
  // a stale promise: the board would otherwise repeat a reason that has
  // stopped being true.
  for (const h of d.contradicted_holds || [])
    out.push(`<div class="finding"><span class="ico" style="color:var(--stalled)">!</span>
      <span class="what"><b>Contradicted hold</b> — <code>${esc(h.id)}</code> ${esc(h.title)}:
      held for “${esc(h.hold)}”, but ${esc(h.why)}</span></div>`);
  for (const u of (d.stale_promises || []).filter(u => repoOn(unitRepo(u))))
    out.push(`<div class="finding"><span class="ico" style="color:var(--stalled)">⏸</span>
      <span class="what"><b>Stale promise</b> — <code>${esc(unitLabel(u))}</code>: ${esc(u.evidence)}</span></div>`);
  const sw = (d.shadow_work || []).filter(c => repoOn(c.repo || "hub"));
  sw.slice(0, 6).forEach(c => out.push(
    `<div class="finding"><span class="ico" style="color:var(--muted)">∅</span>
      <span class="what"><b>Shadow work</b> — ${esc(c.subject)} <code>${esc(c.hash)}</code></span></div>`));
  if (sw.length > 6) out.push(`<div class="more">… and ${sw.length - 6} more shadow commits</div>`);
  return `<section class="panel"><h2>Drift — where the board could lie</h2>
    ${out.length ? out.join("") : `<span class="clean">clean — the board matches reality</span>`}</section>`;
}

function claims(b) {
  // Per-spoke enrichment means claims can exist even when the hub itself
  // has no forge — the panel shows whenever any repo's forge answered.
  const forges = [b.forge, ...(b.workspace || []).map(r => r.forge)].filter(Boolean);
  if (!forges.length) return "";
  const out = [];
  for (const [kind, [ico, head]] of Object.entries(CLAIM_HEADS)) {
    for (const c of (b.claims || []).filter(x => x.kind === kind).slice(0, 8))
      out.push(`<div class="finding"><span class="ico" style="color:var(--stalled)">${ico}</span>
        <span class="what"><b>${esc(c.subject)}</b> ${esc(head)} — ${esc(c.detail)}</span></div>`);
  }
  return `<section class="panel"><h2>Claims vs proof — ${esc(forges.join(", "))}</h2>
    ${out.length ? out.join("") : `<span class="clean">every tracker claim is backed by the repo</span>`}</section>`;
}

// unitLabel prefixes a branch with its workspace repo — api:feature/… —
// the same label the audit uses in evidence strings.
function unitLabel(u) { return u.repo ? `${u.repo}:${u.name}` : u.name; }

// Which refs a branch still has, named the way git names them — deleting
// the local ref and deleting origin's are two different acts, and only one
// of them is anyone else's business.
const refTags = u =>
  `<span class="refs">${u.local ? `<span class="tag ref">local</span>` : ""}${
    u.remote ? `<span class="tag ref">origin</span>` : ""}</span>`;

function branches(b) {
  if (!b.units?.length) return "";
  const units = b.units.filter(u => repoOn(unitRepo(u)));
  const rows = UNIT_ORDER.map(st => units.filter(u => u.status === st).map(u =>
    `<div class="r branch">${chip(u.status)}<code>${esc(unitLabel(u))}</code>
     <span class="ev2">${esc(u.evidence)}${(u.flags||[]).map(f=>` — ⚠ ${esc(f)}`).join("")}</span>
     ${refTags(u)}${RO ? "" : `<button class="bdel" title="Retire this branch"
       data-branch="${esc(u.name)}" data-brepo="${esc(u.repo || "")}">🗑</button>`}</div>`
  ).join("")).join("");
  // Spent branches are the reason this panel now has buttons: merged work
  // whose refs nobody got around to removing.
  const spent = units.filter(u => u.status === "done").length;
  return `<section class="panel"><h2>Branches${
    spent ? ` <span class="count">· ${spent} spent</span>` : ""}</h2><div class="rows">${rows}</div></section>`;
}

function digest(b) {
  const torder = { story: 0, bug: 1, task: 2 };
  // Shipped entries are spec-level: a repo filter keeps the ones whose
  // story touches that repo (same membership rule as the cards).
  const byId = new Map((b.specs || []).map(s => [s.id, s]));
  const byType = [...(b.shipped || [])]
    .filter(s => { const sp = byId.get(s.id); return !sp || [...specRepos(sp)].some(repoOn); })
    .sort((a, c) => (torder[a.type || "story"] || 0) - (torder[c.type || "story"] || 0));
  const shipped = byType.map(s =>
    `<div class="r"><time>${esc(s.date)}</time>
      <span style="color:var(--done)">✓</span>
      <span><b>${esc(s.title)}</b> ${typeTag(s.type)} <span style="color:var(--muted)"><code>${esc(s.id)}</code>${s.epic ? " · " + esc(s.epic) : ""}</span></span></div>`).join("");
  const rest = (b.digest || []).filter(c => !c.spec && repoOn(c.repo || "hub")).slice(0, 12).map(c =>
    `<div class="r"><time>${esc(c.date)}</time><span class="ev2">${c.repo ? `<code>${esc(c.repo)}</code> ` : ""}${esc(c.subject)}</span></div>`).join("");
  const divider = shipped && rest ? `<div class="r"><span class="ev2" style="font-size:.7rem">also landed</span></div>` : "";
  return `<section class="panel digest"><h2>Landed in the last ${b.digest_days} days</h2>
    <div class="rows">${(shipped + divider + rest) || `<span class="ev2">nothing landed</span>`}</div></section>`;
}

function render(b) {
  lastBoard = b;
  // The first audit has answered, so the placeholder has done its job.
  document.getElementById("loading").hidden = true;
  const repoLabel = b.forge || (b.repo || "").split("/").filter(Boolean).pop() || b.repo;
  let meta = esc(`${repoLabel} · integration branch ${b.integration_branch} (${b.elected_via})`);
  if (b.workspace?.length) {
    // The workspace list doubles as a filter: clicking a repo name toggles
    // its chip. Single-repo boards render the same plain text as ever.
    const spokes = b.workspace.map(r =>
      `<span class="rlink${F.repos.has(r.name) ? " on" : ""}" data-repo="${esc(r.name)}">${esc(r.name)}${r.error ? " ✗" : ""}</span>`).join(", ");
    meta += ` · workspace: ${spokes}`;
  }
  document.getElementById("meta").innerHTML = meta;
  syncFilterChips(b);
  let html = "";
  if (b.election_note) html += `<div class="warn">⚠ ${esc(b.election_note)}</div>`;
  for (const r of b.workspace || []) {
    // A spoke the audit cannot see must be loud — a board silently missing
    // a repo would be a board lying about scope.
    if (r.error) html += `<div class="warn">⚠ workspace repo ${esc(r.name)}: ${esc(r.error)}</div>`;
    // A spoke whose forge stayed dark is a quieter truth: git still speaks.
    else if (r.forge_note) html += `<div class="warn">◦ workspace repo ${esc(r.name)}: ${esc(r.forge_note)}</div>`;
  }
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
document.getElementById("meta").addEventListener("click", e => {
  const l = e.target.closest(".rlink");
  if (!l) return;
  F.repos.has(l.dataset.repo) ? F.repos.delete(l.dataset.repo) : F.repos.add(l.dataset.repo);
  rerender();
});

let syncAt = "";
function ago(iso) {
  const s = Math.max(0, Math.round((Date.now() - new Date(iso)) / 1000));
  return s < 90 ? `${s}s ago` : `${Math.round(s / 60)}m ago`;
}

// A board shared beyond this machine shows the truth and, by default,
// edits nothing — the server refuses writes; we hide the doors. When the
// server is armed with an edit token, the doors stay open: writes carry
// the token and the server lands each edit as a commit pushed to origin.
let RO = false;
let MODE = "rw"; // rw (same machine) · token (shared, token-gated) · ro (shared)
const TOKEN_KEY = "truthboard-edit-token";
function authHeaders() {
  const t = localStorage.getItem(TOKEN_KEY);
  return MODE === "token" && t ? { "X-Truthboard-Token": t } : {};
}
function setMode(mode) {
  MODE = mode;
  RO = mode === "ro";
  document.getElementById("new-story").hidden = RO;
  document.getElementById("dt-edit").hidden = RO;
  document.getElementById("dt-delete").hidden = RO;
  document.getElementById("dt-assign-wrap").hidden = RO;
  const unlock = document.getElementById("unlock");
  unlock.hidden = mode !== "token";
  if (mode === "token") unlock.textContent = localStorage.getItem(TOKEN_KEY) ? "🔑 unlocked" : "🔑 unlock";
}

// All writes go through here — intent edits and branch cleanup alike: the
// token rides along, and a 403 on a token-gated board means "not unlocked
// yet" — offer the key.
async function writeFetch(url, opts) {
  const r = await fetch(url, { ...opts, headers: { ...(opts.headers || {}), ...authHeaders() } });
  if (r.status === 403 && MODE === "token") document.getElementById("tokendlg").showModal();
  return r;
}

/* ---------- transient feedback ---------- */

// toast says an action worked, by name. A dialog that closes silently is
// indistinguishable from one that crashed, and the two biggest actions —
// saving a story and retiring one — used to do exactly that.
//
// Failures do not auto-dismiss: an error you did not happen to be looking
// at is an error nobody reported.
function toast(message, kind = "ok") {
  const el = document.createElement("div");
  el.className = "toast " + kind;
  el.append(Object.assign(document.createElement("span"), { textContent: message }));

  const close = Object.assign(document.createElement("button"), {
    className: "x", textContent: "✕", title: "Dismiss",
  });
  close.setAttribute("aria-label", "Dismiss");
  const dismiss = () => {
    el.classList.add("leaving");
    // Match the animation, but never strand the node if it never runs
    // (reduced motion turns the animation off entirely).
    setTimeout(() => el.remove(), 200);
  };
  close.addEventListener("click", dismiss);
  el.append(close);

  document.getElementById("toasts").append(el);
  if (kind !== "bad") setTimeout(dismiss, 4000);
  return el;
}

// refreshNow re-derives immediately instead of waiting out the poll. Every
// write used to set `last = ""` and leave it at that, so the person who
// made the change waited up to 3s to see it — while other viewers, who now
// get an SSE nudge, saw it at once. The actor was the last to know.
function refreshNow() {
  last = "";
  tick(false);
}

// On a token-armed board the server pushes each edit to origin; a push
// that failed must be said out loud, not buried in a server log.
function surfacePush(out) {
  const el = document.getElementById("pusherr");
  if (out && out.push_error) {
    el.hidden = false;
    el.textContent = "⚠ story saved on the board's clone, but did not reach origin: " + out.push_error;
  } else {
    el.hidden = true;
  }
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
    setMode(r.headers.get("X-Truthboard-Readonly") === "1" ? "ro"
      : r.headers.get("X-Truthboard-Edit") === "token" ? "token" : "rw");
    // The serving binary's version, so a stale board is visible at a
    // glance (`truthboard update`, then stop && ui --detach).
    const v = r.headers.get("X-Truthboard-Version");
    if (v) {
      // "dev" is not a version — say what it is instead of looking broken.
      const label = v === "dev" ? "dev build (source)" : v;
      document.getElementById("ver").textContent = label;
      document.getElementById("foot").textContent = `truthboard ${label} · refreshes automatically`;
    }
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
    if (key !== last && !dlg.open && !detailDlg.open && !branchDlg.open) {
      last = key;
      const b = JSON.parse(text);
      if (lastBoard && document.startViewTransition) document.startViewTransition(() => render(b));
      else render(b);
    }
    document.getElementById("updated").textContent = "live · " + new Date().toLocaleTimeString() +
      (syncAt ? " · remote synced " + ago(syncAt) : "");
  } catch (e) {
    document.getElementById("updated").textContent = "audit unavailable — retrying";
    // Nothing has ever rendered, so the placeholder is all there is to
    // read — make it say what is actually happening.
    if (!lastBoard) {
      document.getElementById("loading").textContent =
        "the board could not read the repository — retrying: " + e.message;
    }
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
    holdTag(onBoard) +
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
  if (full.needs?.length) rows.push(["Needs", full.needs.map(id => {
    const dep = (lastBoard?.specs || []).find(x => x.id === id);
    const st = dep ? dep.status : "missing";
    return `<code>${esc(id)}</code> <span style="color:${sv(st)}">${(STATUS[st]||{}).ico || "?"} ${esc(st)}</span>`;
  }).join(" · ")]);
  if (onBoard.per_repo?.length) rows.push(["Repos", onBoard.per_repo.map(r => {
    const good = r.state === "landed";
    const bad = r.state === "not-in-workspace" || r.state === "unreadable";
    const mark = good ? "✓" : bad ? "✗" : "…";
    const col = good ? "var(--done)" : bad ? "var(--regressed, #e5534b)" : "var(--muted)";
    const extra = r.sha ? ` <code>${esc(r.sha.slice(0,7))}</code>` : r.branch ? ` <code>${esc(r.branch)}</code>` : "";
    return `<code>${esc(r.repo)}</code> <span style="color:${col}">${mark} ${esc(r.state)}</span>${extra}`;
  }).join(" · ")]);
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
    const r = await writeFetch("/api/specs/" + encodeURIComponent(detailSpec.id), {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ owner }),
    });
    if (!r.ok) throw new Error(await r.text());
    surfacePush(await r.json());
    detailSpec.owner = owner;
    note.textContent = owner ? `assigned to ${owner}` : "unassigned";
    refreshNow(); // avatars and owner chips, straight away
  } catch (err) {
    e.target.value = detailSpec.owner || "";
    note.textContent = "could not assign: " + err.message;
    toast("Could not assign: " + err.message, "bad");
  }
});

document.getElementById("dt-close").addEventListener("click", () => detailDlg.close());
document.getElementById("dt-edit").addEventListener("click", () => { detailDlg.close(); openEditor(detailSpec); });

// Retiring a story. The server refuses while git still points at the
// story, and says what points at it — so the second confirm quotes the
// server's own reason rather than a guess made here.
document.getElementById("dt-delete").addEventListener("click", async () => {
  if (!detailSpec) return;
  const err = document.getElementById("dt-err");
  err.textContent = "";
  if (!confirm(`Delete ${detailSpec.id}?\n\n"${detailSpec.title}"\n\nThe deletion is a commit — recover it with git revert.`)) return;

  const btn = document.getElementById("dt-delete");
  const label = btn.textContent;
  btn.disabled = true;
  btn.textContent = "deleting…";
  try {
    let r = await writeFetch("/api/specs/" + encodeURIComponent(detailSpec.id), { method: "DELETE" });
    if (r.status === 409) {
      // Proof exists. Say exactly what, and make the override deliberate.
      const why = (await r.text()).trim();
      if (!confirm(`${why}\n\nRetire it anyway?`)) return;
      r = await writeFetch("/api/specs/" + encodeURIComponent(detailSpec.id) + "?force=1", { method: "DELETE" });
    }
    if (!r.ok) throw new Error(await r.text());
    const gone = detailSpec.id;
    surfacePush(await r.json());
    detailDlg.close();
    toast(`${gone} retired — git revert the deletion commit to bring it back`);
    refreshNow();
  } catch (e) {
    err.textContent = e.message;
    toast("Could not retire the story: " + e.message, "bad");
  } finally {
    btn.disabled = false;
    btn.textContent = label;
  }
});

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
    const r = await writeFetch("/api/specs/" + encodeURIComponent(detailSpec.id), {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ body: newBody }),
    });
    if (!r.ok) throw new Error(await r.text());
    surfacePush(await r.json());
    detailSpec.body = newBody;
    toast(box.checked ? "Acceptance criterion signed off" : "Sign-off withdrawn");
    refreshNow(); // progress bars, straight away
  } catch (err) {
    box.checked = !box.checked;
    document.getElementById("dt-hint").hidden = false;
    document.getElementById("dt-hint").textContent = "Could not save sign-off: " + err.message;
    // Said where the action happened, and again where it cannot be missed.
    toast("Could not save sign-off: " + err.message, "bad");
  } finally {
    box.disabled = false;
  }
});

/* ---------- branch cleanup ---------- */
// The board has reported "landed but not deleted" since the beginning and
// left it at that. Acting on it is the one destructive thing this UI can
// do, so it asks twice: once for which refs, once to mean it — and the ref
// on origin is named as a push, because that one goes for everyone.
const branchDlg = document.getElementById("branchdlg");
let branchUnit = null;
let branchForce = false; // only ever set by the server's own refusal

const branchScope = () => ({
  local: !document.getElementById("br-local-wrap").hidden && document.getElementById("br-local").checked,
  remote: !document.getElementById("br-remote-wrap").hidden && document.getElementById("br-remote").checked,
});

function branchStep(n, finalText) {
  branchForce = false; // stepping back and forth means starting over
  document.getElementById("br-step1").hidden = n !== 1;
  document.getElementById("br-step2").hidden = n !== 2;
  document.getElementById("br-next").hidden = n !== 1;
  document.getElementById("br-back").hidden = n !== 2;
  const go = document.getElementById("br-confirm");
  go.hidden = n !== 2;
  go.textContent = "Delete permanently";
  if (finalText) document.getElementById("br-final").innerHTML = finalText;
}

function openBranchDialog(u) {
  branchUnit = u;
  document.getElementById("br-title").textContent = "Retire " + unitLabel(u);
  const d = STATUS[u.status] || {};
  document.getElementById("br-why").innerHTML =
    `<span class="status" style="color:${sv(u.status)}">${d.ico || "·"} ${esc(d.label || u.status)}</span> — ${esc(u.evidence)}`;
  document.getElementById("br-local-wrap").hidden = !u.local;
  document.getElementById("br-remote-wrap").hidden = !u.remote;
  document.getElementById("br-local").checked = !!u.local;
  document.getElementById("br-remote").checked = !!u.remote;
  document.getElementById("br-err").textContent = "";
  branchStep(1);
  branchDlg.showModal();
}

document.getElementById("app").addEventListener("click", e => {
  const btn = e.target.closest(".bdel");
  if (!btn || RO) return;
  const u = (lastBoard?.units || []).find(x =>
    x.name === btn.dataset.branch && (x.repo || "") === btn.dataset.brepo);
  if (u) openBranchDialog(u);
});

document.getElementById("br-cancel").addEventListener("click", () => branchDlg.close());
document.getElementById("br-back").addEventListener("click", () => {
  document.getElementById("br-err").textContent = "";
  branchStep(1);
});
document.getElementById("br-next").addEventListener("click", () => {
  const scope = branchScope();
  const err = document.getElementById("br-err");
  if (!scope.local && !scope.remote) {
    err.textContent = "Nothing selected — pick at least one ref to delete.";
    return;
  }
  err.textContent = "";
  const parts = [];
  if (scope.local) parts.push(`the local branch <code>${esc(branchUnit.name)}</code>`);
  if (scope.remote) parts.push(`<code>origin/${esc(branchUnit.name)}</code>, which is a push — it disappears for everyone`);
  branchStep(2, `This deletes ${parts.join(" and ")}${
    branchUnit.repo ? ` in <code>${esc(branchUnit.repo)}</code>` : ""}. The board has no undo for it.`);
});

document.getElementById("br-confirm").addEventListener("click", async () => {
  if (!branchUnit) return;
  const scope = branchScope();
  const q = new URLSearchParams({ name: branchUnit.name });
  if (branchUnit.repo) q.set("repo", branchUnit.repo);
  if (scope.local) q.set("local", "1");
  if (scope.remote) q.set("remote", "1");
  if (branchForce) q.set("force", "1");

  const btn = document.getElementById("br-confirm");
  const err = document.getElementById("br-err");
  let nextLabel = btn.textContent;
  btn.disabled = true;
  btn.textContent = "deleting…";
  err.textContent = "";
  try {
    const r = await writeFetch("/api/branches?" + q.toString(), { method: "DELETE" });
    if (r.status === 409) {
      // The server refuses a branch that still carries work and says what
      // would be lost. Quote it — a paraphrase made here could be wrong
      // about the one thing that matters.
      const why = (await r.text()).trim();
      err.textContent = why;
      if (why.includes("force=1")) {
        branchForce = true;
        nextLabel = "Delete anyway — this drops the work";
      }
      return;
    }
    if (!r.ok) throw new Error(await r.text());
    const out = await r.json();
    const label = unitLabel(branchUnit);
    branchDlg.close();
    const gone = (out.deleted || []).map(x => x === "origin" ? "the ref on origin" : "the local ref").join(" and ");
    toast(`${label} retired — ${gone} deleted`);
    // A push origin refused is not a detail: half a cleanup, said out loud.
    if (out.failed?.length) toast(`${label}: ${out.failed.join("; ")}`, "bad");
    refreshNow();
  } catch (e) {
    err.textContent = e.message;
  } finally {
    btn.disabled = false;
    btn.textContent = nextLabel;
  }
});

/* ---------- editor ---------- */
const dlg = document.getElementById("editor");
let editingId = null;
const TEMPLATE = "## Goal\n\n(what outcome, and why)\n\n## Acceptance\n\n- [ ] (observable criterion)\n";

// fillRepos builds the Repos options from the workspace manifest, so the
// field offers repos that exist instead of trusting whatever was typed —
// a typo used to become a story that could never be done, since done means
// the trailer landed in every declared repo.
//
// Two cases the options must survive. A repo the spec already declares but
// the manifest no longer has (renamed, removed) stays selectable and
// selected: dropping it here would quietly rewrite intent the board is
// still reporting as drift. And a single-repo board has nothing to choose
// between, so the field is hidden unless a stale declaration needs
// clearing.
function fillRepos(selected) {
  const box = document.getElementById("ed-rp");
  const known = lastBoard?.workspace?.length ? ["hub", ...lastBoard.workspace.map(r => r.name)] : [];
  const extra = selected.filter(r => !known.includes(r));
  document.getElementById("ed-rp-wrap").hidden = !known.length && !extra.length;

  // type="button": these live inside a <form method="dialog">, where a
  // bare <button> submits.
  box.innerHTML = [...known, ...extra].map(r => {
    const gone = !known.includes(r);
    const label = (r === "hub" ? "⌂ " : "") + r + (gone ? " — not in workspace" : "");
    return `<button type="button" class="fchip${selected.includes(r) ? " on" : ""}" ` +
      `data-repo="${esc(r)}" aria-pressed="${selected.includes(r)}">${esc(label)}</button>`;
  }).join("");
}

// Each chip toggles only itself. That is the whole point of the control:
// the multi-select it replaced treated an unmodified click as "select this
// one and drop the rest", on a field that decides what done requires.
document.getElementById("ed-rp").addEventListener("click", e => {
  const chip = e.target.closest(".fchip");
  if (!chip) return;
  const on = !chip.classList.contains("on");
  chip.classList.toggle("on", on);
  chip.setAttribute("aria-pressed", String(on));
});

function selectedRepos() {
  return [...document.querySelectorAll("#ed-rp .fchip.on")].map(c => c.dataset.repo);
}

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
  document.getElementById("ed-nd").value = (spec?.needs || []).join(", ");
  document.getElementById("ed-hd").value = spec?.hold || "";
  fillRepos(spec?.repos || []);
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
    needs: document.getElementById("ed-nd").value.split(",").map(x => x.trim()).filter(Boolean),
    hold: document.getElementById("ed-hd").value.trim(),
    repos: selectedRepos(),
    body: document.getElementById("ed-b").value,
  };
  // On a shared board a save is a commit and a push to the forge, so it
  // takes as long as the network does. Silence for that whole time reads
  // as a hang and gets the button pressed again — say it is working, and
  // refuse the second press.
  // By id, not e.submitter: submitting with Enter leaves that null.
  const save = document.getElementById("ed-save");
  const saveLabel = save.textContent;
  save.disabled = true;
  save.textContent = "saving…";
  document.getElementById("ed-err").textContent = "";
  try {
    const r = await writeFetch(editingId ? "/api/specs/" + encodeURIComponent(editingId) : "/api/specs", {
      method: editingId ? "PUT" : "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
    if (!r.ok) throw new Error(await r.text());
    const saved = await r.json();
    surfacePush(saved);
    const was = editingId;
    dlg.close();
    toast(was ? `${was} updated` : `Story created · ${saved.id}`);
    refreshNow();
  } catch (err) {
    document.getElementById("ed-err").textContent = err.message;
  } finally {
    save.disabled = false;
    save.textContent = saveLabel;
  }
});

/* ---------- edit token (shared boards) ---------- */
const tokendlg = document.getElementById("tokendlg");
document.getElementById("unlock").addEventListener("click", () => {
  document.getElementById("tk-in").value = localStorage.getItem(TOKEN_KEY) || "";
  tokendlg.showModal();
});
tokendlg.addEventListener("close", () => {
  if (tokendlg.returnValue === "cancel") return;
  const t = document.getElementById("tk-in").value.trim();
  if (t) localStorage.setItem(TOKEN_KEY, t);
  else localStorage.removeItem(TOKEN_KEY);
  setMode(MODE); // refresh the 🔑 label
});
