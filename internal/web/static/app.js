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
/* GitHub-flavoured tables: a header row, a delimiter row, then body rows.
   The delimiter is what makes it a table — pipes turn up in prose and in
   code, and promoting those to tables would be a worse bug than the one
   this fixes. Source is already escaped by md() before these run. */
const tableRow = l => /^\s*\|.*\|\s*$/.test(l);
const tableDelim = l => /^\s*\|[\s:|-]*-[\s:|-]*\|\s*$/.test(l);
const tableCells = l => l.trim().replace(/^\||\|$/g, "").split("|").map(c => c.trim());
const tableAligns = l => tableCells(l).map(c => {
  const l0 = c.startsWith(":"), r0 = c.endsWith(":");
  // `:---` is stated rather than left implicit: left happens to be the
  // default today, and a written alignment should not depend on that.
  return l0 && r0 ? "center" : r0 ? "right" : l0 ? "left" : "";
});

function md(src, interactive = false) {
  const lines = esc(src ?? "").split("\n");
  const out = []; let code = false, list = false, para = [], task = 0;
  const flushP = () => { if (para.length) { out.push("<p>" + inlineMd(para.join(" ")) + "</p>"); para = []; } };
  const closeL = () => { if (list) { out.push("</ul>"); list = false; } };
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    if (line.startsWith("```")) { flushP(); closeL(); code = !code; out.push(code ? "<pre><code>" : "</code></pre>"); continue; }
    if (code) { out.push(line + "\n"); continue; }
    if (tableRow(line) && !tableDelim(line) && i + 1 < lines.length && tableDelim(lines[i + 1])) {
      flushP(); closeL();
      const head = tableCells(line), al = tableAligns(lines[i + 1]);
      const cell = (tag, text, k) =>
        `<${tag}${al[k] ? ` style="text-align:${al[k]}"` : ""}>${inlineMd(text ?? "")}</${tag}>`;
      const body = [];
      let j = i + 2;
      while (j < lines.length && tableRow(lines[j]) && !tableDelim(lines[j])) {
        const r = tableCells(lines[j]);
        // Indexed off the header, so a short row leaves empty cells rather
        // than shifting content into the wrong column.
        body.push("<tr>" + head.map((_, k) => cell("td", r[k], k)).join("") + "</tr>");
        j++;
      }
      out.push(`<div class="tablewrap"><table><thead><tr>${
        head.map((h, k) => cell("th", h, k)).join("")}</tr></thead><tbody>${body.join("")}</tbody></table></div>`);
      i = j - 1;
      continue;
    }
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
                (d.scope_creep?.length||0) + (d.unknown_repos?.length||0) + (d.unwired_repos?.length||0) +
                (d.unverified_acceptance?.length||0) + (b.claims?.length||0) + n("regressed");
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

/* The sprint about to start. Everything here is already on /api/board —
   tb-0274 put the rollup on the audit Result — so this is rendering, not a
   second computation, and it refreshes with every other section because
   render() rebuilds from lastBoard.

   Two rules the panel must not break. It never implies a velocity: the
   reference is one prior sprint and the caption says so. And it never
   flattens a stalled rollover story into "open" — a story nobody has
   touched is the most useful thing on this panel. */
function planStory(it, extra = "") {
  const st = it.status || "planned";
  return `<span class="pstory" data-status="${esc(st)}">
    <span class="pico" style="color:${sv(st)}" title="${esc((STATUS[st]||{}).label || st)}">${(STATUS[st]||{}).ico || "·"}</span>
    <code class="pid">${esc(it.id)}</code>
    <span class="ptitle">${esc(it.title)}</span>${extra}${
      it.epic ? `<span class="pepic"><i class="dot" style="background:${epicColor(it.epic)}"></i>${esc(it.epic)}</span>` : ""}${
      it.points ? `<span class="ppts">${it.points}</span>` : `<span class="ppts none">no estimate</span>`}
  </span>`;
}

function planDays(plan) {
  if (!plan.start || !plan.end) return 0;
  const a = Date.parse(plan.start + "T00:00:00Z"), b = Date.parse(plan.end + "T00:00:00Z");
  if (isNaN(a) || isNaN(b) || b < a) return 0;
  return Math.round((b - a) / 86400000) + 1; // the end date is inclusive
}

function planPanel(b) {
  const p = b.plan;
  if (!p) return "";
  const band = (heading, items, tally, decorate) => items?.length ? `
    <div class="pband">
      <div class="pband-head"><h3>${esc(heading)}</h3><span class="ptally">${esc(tally)}</span></div>
      <div class="pstories">${items.map(it => planStory(it, decorate ? decorate(it) : "")).join("")}</div>
    </div>` : "";

  const days = planDays(p);
  const head = p.sprint
    ? `<span class="ptarget">${esc(p.sprint)}</span>
       <span class="pwindow">${p.start && p.end
          ? `${esc(p.start)} → ${esc(p.end)}${days ? ` · ${days} days` : ""}`
          : "no dates declared for this sprint"}</span>
       ${p.state ? `<span class="pstate sp-${esc(p.state)}">${esc(p.state)}</span>` : ""}`
    : `<span class="ptarget none">No sprint is waiting to start</span>
       <span class="pwindow">rollover and candidates only — give a sprint dates to plan into it</span>`;

  const committed = p.points || 0, rolling = p.rollover_points || 0;
  const total = committed + rolling;
  let load = "";
  if (total > 0 || p.reference_points) {
    const span = Math.max(total, p.reference_points || 0) || 1;
    const mark = p.reference_points ? Math.min(100, 100 * p.reference_points / span) : null;
    load = `<div class="pload">
      <div class="pload-figures"><b>${total} pts</b>
        <span>on the table — ${committed} committed, ${rolling} rolling over</span></div>
      <div class="pload-track">
        <span class="seg-committed" style="width:${100 * committed / span}%"></span>
        <span class="seg-rollover" style="width:${100 * rolling / span}%"></span>
      </div>
      ${mark === null ? "" : `<div class="pload-mark"><i style="left:${mark}%"></i>
        <span style="left:${mark}%">${esc(p.reference_sprint || "last sprint")} landed ${p.reference_points}</span></div>`}
      <p class="pload-note">${p.reference_points
        ? "One prior sprint, not a velocity — there is no history behind this number."
        : "No prior sprint to compare against."}${
        p.unestimated ? ` ${p.unestimated} committed ${p.unestimated === 1 ? "story carries" : "stories carry"} no estimate and ${p.unestimated === 1 ? "is" : "are"} excluded from the total.` : ""}</p>
    </div>`;
  }

  return `<section class="panel plan">
    <h2>The sprint about to start</h2>
    <div class="phead">${head}</div>
    ${load}
    ${band("Rolls over from " + esc(p.from || "the last sprint"), p.rollover, `${(p.rollover||[]).length} · ${rolling} pts if all of it is pulled in`)}
    ${band("Already committed", p.committed, `${(p.committed||[]).length} · ${committed} pts`)}
    ${band("Ready to pull in", p.ready, `${(p.ready||[]).length} · backlog order`)}
    ${band("Blocked", p.blocked, `${(p.blocked||[]).length}`,
      it => it.waiting?.length ? `<span class="pblock">needs ${it.waiting.map(w => `<code>${esc(w)}</code>`).join(" ")}</span>` : "")}
  </section>`;
}

// The one panel written for the person who does not read git. Every word
// of judgement here — the headline, the section names, why something is
// paused — was chosen in internal/audit and is shared verbatim with
// `truthboard summary`, so the page and the pasted document cannot come to
// disagree about what happened.
function summaryPanel(b) {
  const s = b.summary;
  if (!s) return "";
  const list = (heading, items, kind) => items?.length ? `
    <div class="sumsec">
      <h3>${esc(heading)} <span class="sumn">${items.length}</span></h3>
      <ul class="sumlist ${kind}">${items.map(it => `
        <li><span class="sumt">${esc(it.title)}</span>${
          it.points ? `<span class="sumpts">${it.points} points</span>` : ""}${
          it.reason ? `<span class="sumwhy">${esc(it.reason)}</span>` : ""}</li>`).join("")}</ul>
    </div>` : "";
  return `<section class="panel summary">
    <h2>Where things stand — ${esc(s.scope)}</h2>
    <p class="sumhead">${esc(s.headline)}</p>
    <div class="sumgrid">
      ${list("Delivered", s.delivered, "ok")}
      ${list("Broke after delivery", s.broken, "bad")}
      ${list("Being worked on", s.in_flight, "live")}
      ${list("Paused", s.paused, "held")}
      ${list("Not started yet", s.not_started, "cold")}
    </div>
    ${s.unestimated ? `<p class="sumfoot">${s.unestimated} open ${s.unestimated === 1 ? "story carries" : "stories carry"} no estimate, so ${s.unestimated === 1 ? "it is" : "they are"} not in the point totals — that is not the same as being zero-sized.</p>` : ""}
  </section>`;
}

// Sprints are arithmetic over the same derived statuses as the board —
// a sprint "finishes" when its stories land, and there is nothing to set.
function sprintsPanel(b) {
  if (!b.sprints?.length) return "";
  const rows = b.sprints.map(sp => {
    const pct = sp.total ? Math.round(100 * sp.done / sp.total) : 0;
    // One element per story: icon, id and title are children of it, so a
    // narrow viewport wraps whole stories instead of splitting an id from
    // its title beside the next story's icon. The chip's own inset is the
    // separator — a gap alone left them running together.
    const open = (sp.open || []).map(o =>
      `<span class="spstory"><span class="spico" style="color:${sv(o.status)}" title="${esc((STATUS[o.status]||{}).label || o.status)}">${(STATUS[o.status]||{}).ico || ""}</span><code>${esc(o.id)}</code><span class="spopen">${esc(o.title)}</span></span>`
    ).join("");
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
  // Wiring is a property of the repo, not of any branch, so a spoke filter
  // would only ever hide the finding from the person who needs it.
  for (const ur of d.unwired_repos || [])
    out.push(`<div class="finding"><span class="ico" style="color:var(--stalled)">⚙</span>
      <span class="what"><b>Unwired spoke</b> — ${esc(ur)}</span></div>`);
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
  // Landed and unverified: git proved the work, nobody read the promise
  // back. The card still says done — this only stops the omission being
  // invisible, which is the whole reason it kept happening.
  for (const ua of d.unverified_acceptance || [])
    out.push(`<div class="finding"><span class="ico" style="color:var(--stalled)">☐</span>
      <span class="what"><b>Unverified acceptance</b> — <code>${esc(ua.id)}</code> ${esc(ua.title)}:
      landed with ${ua.done}/${ua.total} criteria ticked</span></div>`);
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
  html += tiles(b) + kanban(b) + summaryPanel(b) + planPanel(b) + sprintsPanel(b);
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

// The detail view is the only place a story is read in full, so it mirrors
// the editor field for field: everything a human can write must be
// readable here, including the fields nobody filled in. An omitted field
// and an empty one look identical once they are both absent, and that
// ambiguity is what sent declared repos missing for a whole release —
// they were only ever drawn from the audit's per-repo evidence, so a
// cross-repo story showed nothing at all until work started landing.
const NOT_SET = `<span class="unset">not set</span>`;
const PRIORITY_LABEL = { 1: "p1 · now", 2: "p2 · next", 3: "p3 · later" };
const kvRows = rows => rows.map(([k, v]) => `<div class="kv"><b>${k}</b><span>${v}</span></div>`).join("");
const codeList = xs => xs.map(x => `<code>${esc(x)}</code>`).join(" ");

function openDetail(full) {
  detailSpec = full;
  const onBoard = (lastBoard?.specs || []).find(x => x.id === full.id) || {};
  const st = onBoard.status || "planned";
  document.getElementById("dt-status").outerHTML = `<span class="status" id="dt-status" style="color:${sv(st)}">${(STATUS[st]||{}).ico || ""} ${esc((STATUS[st]||{}).label || st)}</span>`;
  document.getElementById("dt-title").textContent = full.title;
  // Chips are the glance — the card's own summary, set values only. The
  // intent block below is the record, and repeats them on purpose.
  // The hold note comes from the story file (saved a moment ago, maybe),
  // its contradiction from the last audit — never the note alone.
  document.getElementById("dt-chips").innerHTML =
    `<code>${esc(full.id)}</code>` +
    (full.priority ? `<span class="tag pri">p${esc(full.priority)}</span>` : "") +
    (full.points ? `<span class="tag pts">${esc(full.points)}pt</span>` : "") +
    typeTag(full.type) +
    (onBoard.waiting?.length ? `<span class="tag wait" title="waiting on ${esc(onBoard.waiting.join(", "))}">⧗ ${esc(onBoard.waiting.join(" "))}</span>` : "") +
    epicTag(full.epic) +
    (full.sprint ? `<span class="tag sprint">${esc(full.sprint)}</span>` : "") +
    holdTag({ hold: full.hold, hold_contradicted: onBoard.hold_contradicted }) +
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
  // Declared intent: every field the editor writes, in the editor's order,
  // whether or not it holds anything. None of it is derived, so none of it
  // belongs under the heading below.
  const intent = [
    ["Owner", full.owner ? esc(full.owner) : NOT_SET],
    ["Type", esc(full.type || "story")],
    ["Priority", full.priority ? esc(PRIORITY_LABEL[full.priority] || `p${full.priority}`) : NOT_SET],
    ["Points", full.points ? `${esc(full.points)} pt` : NOT_SET],
    ["Epic", full.epic ? epicTag(full.epic) : NOT_SET],
    ["Sprint", full.sprint ? esc(full.sprint) : NOT_SET],
    // Separated the way the per-repo evidence row below is, so the two
    // read as the same list — one declared, one proven.
    ["Repos", full.repos?.length
      ? full.repos.map(r => `<code>${esc(r)}</code>`).join(" · ") +
        ` <span class="unset">— done requires all of them</span>`
      : NOT_SET],
    ["Needs", full.needs?.length ? full.needs.map(id => {
      const dep = (lastBoard?.specs || []).find(x => x.id === id);
      const dst = dep ? dep.status : "missing";
      return `<code>${esc(id)}</code> <span style="color:${sv(dst)}">${(STATUS[dst]||{}).ico || "?"} ${esc(dst)}</span>`;
    }).join(" · ") : NOT_SET],
    ["Scope", full.paths?.length ? codeList(full.paths) : NOT_SET],
    ["Branch glob", full.branch ? `<code>${esc(full.branch)}</code>` : NOT_SET],
    ["On hold", full.hold
      ? esc(full.hold) + (onBoard.hold_contradicted
          ? ` <span style="color:var(--regressed, #e5534b)">! git says otherwise: ${esc(onBoard.hold_contradicted)}</span>` : "")
      : NOT_SET],
  ];
  document.getElementById("dt-intent").innerHTML =
    `<h4>Declared intent — this is what "Edit story" writes</h4>` + kvRows(intent);

  const rows = [["Status", `${esc(onBoard.evidence || "no matching branch or commit yet")}`]];
  const total = onBoard.acceptance_total || 0;
  if (total) rows.push(["Acceptance", `${onBoard.acceptance_done || 0} of ${total} signed off`]);
  if (onBoard.branches?.length) rows.push(["Branches", codeList(onBoard.branches)]);
  if (onBoard.landed) rows.push(["Landed", `<code>${esc(onBoard.landed.slice(0,7))}</code>` +
    ` in <code>${esc(onBoard.landed_repo || "hub")}</code>`]);
  if (onBoard.per_repo?.length) rows.push(["Per repo", onBoard.per_repo.map(r => {
    const good = r.state === "landed";
    const bad = r.state === "not-in-workspace" || r.state === "unreadable";
    const mark = good ? "✓" : bad ? "✗" : "…";
    const col = good ? "var(--done)" : bad ? "var(--regressed, #e5534b)" : "var(--muted)";
    const extra = r.sha ? ` <code>${esc(r.sha.slice(0,7))}</code>` : r.branch ? ` <code>${esc(r.branch)}</code>` : "";
    return `<code>${esc(r.repo)}</code> <span style="color:${col}">${mark} ${esc(r.state)}</span>${extra}`;
  }).join(" · ")]);
  rows.push(["Linking", `any branch containing <code>${esc(full.id)}</code> · trailer <code>Spec: ${esc(full.id)}</code>`]);
  if (onBoard.file) rows.push(["Story file", `<code>${esc(onBoard.file.replace(/^.*\/\.truthboard\//, ".truthboard/"))}</code>`]);
  document.getElementById("dt-truth").innerHTML = `<h4>Derived truth — computed, not editable</h4>` + kvRows(rows);
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
  document.getElementById("ed-b-hint").hidden = preview;
  const pv = document.getElementById("ed-preview");
  pv.hidden = !preview;
  if (preview) pv.innerHTML = md(document.getElementById("ed-b").value) || `<p style="color:var(--muted)">Nothing to preview.</p>`;
}
document.getElementById("tab-write").addEventListener("click", () => setTab(false));
document.getElementById("tab-preview").addEventListener("click", () => setTab(true));

/* ---------- the acceptance list writes itself ----------

   The checkbox list is the part of the body the board counts, and it was
   also the part authors had to type marker and all — five characters of
   punctuation per criterion, which is exactly the friction that gets a
   story written with one criterion instead of four. Two affordances, no
   new concepts: Enter continues whatever list the caret is in, and one
   button extends the acceptance list from anywhere in the form. */

// One list item, split into indent / marker / whatever was written after
// it. The marker is kept whole so a continuation can reuse it: a checklist
// begets a checklist, a bullet begets a bullet.
const LIST_ITEM = /^([ \t]*)([-*+][ \t]+(?:\[[ xX]\][ \t]+)?|\d+[.)][ \t]+)(.*)$/;
const CRITERION = "- [ ] ";
const HEADING = /^[ \t]*#{1,6}[ \t]/;
const ACCEPTANCE_HEAD = /^[ \t]*#{1,6}[ \t]*acceptance\b/i;

// typeInto writes through execCommand because it is still the only
// insertion the browser's own undo stack knows about: assigning .value or
// calling setRangeText silently discards everything typed before it, so a
// helpful auto-inserted marker would cost the author their Ctrl-Z history.
function typeInto(ta, text) {
  ta.focus();
  const ok = text ? document.execCommand("insertText", false, text)
                  : document.execCommand("delete");
  if (!ok) ta.setRangeText(text, ta.selectionStart, ta.selectionEnd, "end");
}

function nextMarker(marker) {
  // Always an unchecked box, even after a signed-off one: these boxes are
  // the sign-off record, and nobody has verified a criterion that does not
  // exist yet.
  if (marker.includes("[")) return marker.replace(/\[[ xX]\]/, "[ ]");
  const n = /^(\d+)([.)][ \t]+)$/.exec(marker);
  return n ? Number(n[1]) + 1 + n[2] : marker;
}

document.getElementById("ed-b").addEventListener("keydown", e => {
  // Shift+Enter stays a plain newline — the escape hatch for a wrapped
  // criterion — and a modified or composing Enter is not ours to read.
  if (e.key !== "Enter" || e.isComposing) return;
  if (e.shiftKey || e.altKey || e.ctrlKey || e.metaKey) return;
  const ta = e.currentTarget;
  if (ta.selectionStart !== ta.selectionEnd) return; // Enter over a selection replaces it

  const start = ta.value.slice(0, ta.selectionStart).lastIndexOf("\n") + 1;
  const nl = ta.value.indexOf("\n", ta.selectionStart);
  const line = ta.value.slice(start, nl === -1 ? ta.value.length : nl);
  const m = LIST_ITEM.exec(line);
  if (!m) return;

  e.preventDefault();
  if (m[3].trim()) {
    typeInto(ta, "\n" + m[1] + nextMarker(m[2]));
    return;
  }
  // Enter on an item nobody filled in ends the list instead of laying down
  // another empty marker — the way out that needs no mouse.
  ta.setSelectionRange(start, start + line.length);
  typeInto(ta, "");
});

// addCriterion appends an empty criterion to the acceptance list and puts
// the caret on it, writing where the author would have: the end of the
// list under the Acceptance heading, or a whole new Acceptance section
// when the body has none. Never in the middle of the goal, and never over
// anything already written.
function addCriterion() {
  const ta = document.getElementById("ed-b");
  setTab(false); // the button is offered from the Preview tab too
  const lines = ta.value.split("\n");
  const head = lines.findIndex(l => ACCEPTANCE_HEAD.test(l));

  let at, text;
  if (head === -1) {
    const body = ta.value.replace(/\s+$/, "");
    at = body.length;
    text = (body ? "\n\n" : "") + "## Acceptance\n\n" + CRITERION;
  } else {
    // The section runs to the next heading; trailing blank lines inside it
    // belong to the gap before that heading, not to the list.
    let end = lines.findIndex((l, i) => i > head && HEADING.test(l));
    if (end === -1) end = lines.length;
    while (end > head + 1 && !lines[end - 1].trim()) end--;

    const prev = lines[end - 1];
    const item = LIST_ITEM.exec(prev);
    at = lines.slice(0, end).join("\n").length;
    // A list item continues the list; a heading or a paragraph needs the
    // blank line that makes the list a list at all.
    text = item ? "\n" + item[1] + CRITERION : "\n\n" + CRITERION;
  }
  ta.setSelectionRange(at, at);
  typeInto(ta, text);
}
document.getElementById("ed-add-ac").addEventListener("click", addCriterion);

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

/* ---------- export: the board as a slide deck ----------

   A PO leaving the repo with what the board knows. The PDF comes from the
   browser's own print engine — no dependency in the binary, real
   typography, and the preview on screen *is* the printed page, so nobody
   prints to find out what they got.

   Two rules the deck must not break. It never invents: every number on it
   is one the audit already derived, and stories arrive already filtered by
   whatever the board is filtered by (said out loud on the cover, because a
   deck that quietly omits half the sprint is worse than no deck). And it
   must survive paper: a printed slide cannot scroll, so pagination is
   computed from how much each story carries, and long prose is clamped
   rather than allowed to run off the page. */

const EX_SCOPES = [
  ["delivered", "Delivered recently"],
  ["sprint", "One sprint"],
  ["board", "Whole board"],
];
// Field keys are what the slides read; presets are just sets of them, so
// "standard, but with the acceptance list" is one click away from a preset
// rather than a different feature.
const EX_FIELDS = [
  ["evidence", "Evidence"],
  ["meta", "Epic · owner · points"],
  ["progress", "Sign-off count"],
  ["refs", "Branches and landing"],
  ["goal", "Goal"],
  ["acceptance", "Acceptance checklist"],
];
const EX_PRESETS = [
  ["titles", "Titles only", []],
  ["standard", "Standard", ["evidence", "meta", "progress"]],
  ["everything", "Everything", ["evidence", "meta", "progress", "refs", "goal", "acceptance"]],
];
const EX = { scope: "delivered", sprint: "", statuses: new Set(["done"]), fields: new Set(EX_PRESETS[1][2]) };

const exDlg = document.getElementById("exportdlg");
const deckWrap = document.getElementById("deckwrap");
const exChip = (on, data, label) =>
  `<button type="button" class="fchip${on ? " on" : ""}" ${data} aria-pressed="${on}">${label}</button>`;

// Sprints newest first, the order the sprint filter already uses.
const exSprints = () => [...new Set((lastBoard?.specs || []).map(s => s.sprint).filter(Boolean))].sort().reverse();

function exStories() {
  const b = lastBoard;
  if (!b) return [];
  // specMatches: the deck shows what the board shows. A filtered board
  // exporting the unfiltered truth would be a different document than the
  // one the exporter was looking at.
  let specs = (b.specs || []).filter(specMatches);
  if (EX.scope === "delivered") {
    const recent = new Set((b.shipped || []).map(s => s.id));
    specs = specs.filter(s => recent.has(s.id));
  } else if (EX.scope === "sprint") {
    // No sprint picked means no sprint exists to pick: "" would otherwise
    // match every story that never declared one — the opposite of a
    // sprint deck.
    if (!EX.sprint) return [];
    specs = specs.filter(s => s.sprint === EX.sprint);
  }
  return specs.filter(s => EX.statuses.has(s.status));
}

// How many stories fit on one slide, from how much each one carries. A
// slide cannot scroll, so this is the difference between a deck and a
// deck with the last two stories cut off.
function exPerSlide() {
  if (EX.fields.has("goal") || EX.fields.has("acceptance")) return 2;
  if (EX.fields.has("evidence") || EX.fields.has("refs")) return 6;
  if (EX.fields.has("meta") || EX.fields.has("progress")) return 8;
  return 12;
}

const chunk = (xs, n) => {
  const out = [];
  for (let i = 0; i < xs.length; i += n) out.push(xs.slice(i, i + n));
  return out;
};

function exSlideCount(specs) {
  const per = exPerSlide();
  return 1 + SPEC_ORDER.reduce((n, st) =>
    n + chunk(specs.filter(s => s.status === st), per).length, 0);
}

function exRefresh() {
  const b = lastBoard;
  document.getElementById("ex-scope").innerHTML = EX_SCOPES.map(([k, label]) => {
    const note = k === "delivered" && b?.digest_days ? ` · last ${b.digest_days} days` : "";
    return exChip(EX.scope === k, `data-exscope="${k}"`, esc(label + note));
  }).join("");
  const sprints = exSprints();
  const wrap = document.getElementById("ex-sprint-wrap");
  wrap.hidden = EX.scope !== "sprint";
  if (EX.scope === "sprint") {
    if (!sprints.includes(EX.sprint)) EX.sprint = sprints[0] || "";
    document.getElementById("ex-sprint").innerHTML = sprints.length
      ? sprints.map(s => `<option value="${esc(s)}"${s === EX.sprint ? " selected" : ""}>${esc(s)}</option>`).join("")
      : `<option value="">no story declares a sprint yet</option>`;
  }
  document.getElementById("ex-status").innerHTML = SPEC_ORDER.map(st =>
    exChip(EX.statuses.has(st), `data-exstatus="${st}"`,
      `<span style="color:${sv(st)}">${STATUS[st].ico}</span> ${esc(STATUS[st].label)}`)).join("");
  const same = keys => keys.length === EX.fields.size && keys.every(k => EX.fields.has(k));
  document.getElementById("ex-preset").innerHTML = EX_PRESETS.map(([k, label, keys]) =>
    exChip(same(keys), `data-expreset="${k}"`, esc(label))).join("") +
    (EX_PRESETS.some(([, , keys]) => same(keys)) ? "" : `<span class="fchip on" aria-hidden="true">Custom</span>`);
  document.getElementById("ex-fields").innerHTML = EX_FIELDS.map(([k, label]) =>
    exChip(EX.fields.has(k), `data-exfield="${k}"`, esc(label))).join("");

  const specs = exStories();
  const pts = specs.reduce((a, s) => a + (s.points || 0), 0);
  document.getElementById("ex-count").textContent = specs.length
    ? `${specs.length} stor${specs.length === 1 ? "y" : "ies"}${pts ? ` · ${pts} points` : ""} · ${exSlideCount(specs)} slides` +
      (filterActive() ? " · the board's filters apply" : "")
    : EX.scope === "sprint" && !EX.sprint
      ? "No story declares a sprint yet — a sprint deck needs one."
      : "Nothing matches — widen the scope or the statuses.";
  document.getElementById("ex-build").disabled = !specs.length;
}

document.getElementById("export").addEventListener("click", () => {
  document.getElementById("ex-err").textContent = "";
  exRefresh();
  exDlg.showModal();
});
exDlg.addEventListener("click", e => {
  const c = e.target.closest(".fchip");
  if (!c) return;
  if (c.dataset.exscope) {
    EX.scope = c.dataset.exscope;
    // Each scope carries its own honest default: delivered means done —
    // nothing else in that window shipped — while a sprint or a board
    // review is about everything in it, done or not.
    EX.statuses = EX.scope === "delivered" ? new Set(["done"]) : new Set(SPEC_ORDER);
  } else if (c.dataset.exstatus) {
    EX.statuses.has(c.dataset.exstatus) ? EX.statuses.delete(c.dataset.exstatus) : EX.statuses.add(c.dataset.exstatus);
  } else if (c.dataset.expreset) {
    EX.fields = new Set(EX_PRESETS.find(p => p[0] === c.dataset.expreset)[2]);
  } else if (c.dataset.exfield) {
    EX.fields.has(c.dataset.exfield) ? EX.fields.delete(c.dataset.exfield) : EX.fields.add(c.dataset.exfield);
  } else return;
  exRefresh();
});
document.getElementById("ex-sprint").addEventListener("change", e => {
  EX.sprint = e.target.value;
  exRefresh();
});

/* ---- the slides ---- */

// The story body is markdown a human wrote: pull the goal out of it
// without reprinting the acceptance checklist that follows, and fall back
// to whatever prose comes before the first heading when there is no
// "## Goal" at all.
function goalOf(body) {
  const lines = String(body || "").split("\n");
  const upToNextHeading = xs => {
    const end = xs.findIndex(l => /^##+\s/.test(l));
    return (end < 0 ? xs : xs.slice(0, end)).join("\n").trim();
  };
  const goal = lines.findIndex(l => /^##+\s*goal\b/i.test(l));
  return goal >= 0 ? upToNextHeading(lines.slice(goal + 1)) : upToNextHeading(lines);
}
function acceptanceOf(body) {
  return [...(body || "").matchAll(/^\s*[-*]\s+\[([ xX])\]\s+(.*)$/gm)]
    .map(m => ({ done: m[1] !== " ", text: m[2].trim() }));
}

function exStoryCard(s, full) {
  const f = k => EX.fields.has(k);
  const bits = [];
  if (f("meta")) {
    bits.push(
      (s.epic ? `<span class="s-epic"><i style="background:${epicColor(s.epic)}"></i>${esc(s.epic)}</span>` : "") +
      (s.owner ? `<span class="s-own">${esc(s.owner)}</span>` : "") +
      (s.points ? `<span class="s-pts">${esc(s.points)} pt</span>` : "") +
      (s.type && s.type !== "story" ? `<span class="s-type">${esc(s.type)}</span>` : ""));
  }
  if (f("progress") && s.acceptance_total)
    bits.push(`<span class="s-prog">${s.acceptance_done || 0}/${s.acceptance_total} signed off</span>`);
  const meta = bits.filter(Boolean).join("");

  const goal = f("goal") ? goalOf(full?.body) : "";
  const acc = f("acceptance") ? acceptanceOf(full?.body) : [];
  const shown = acc.slice(0, 5);
  // No status on the card: every slide is one status group, and a column
  // of "✓ Done" under a heading reading "Done" is noise. The date it
  // landed is the thing a delivered deck is actually being asked for.
  const when = (lastBoard?.shipped || []).find(x => x.id === s.id)?.date || "";
  return `<article class="s-card" style="--st:${sv(s.status)}">
    <div class="s-top"><span class="s-id">${esc(s.id)}</span>
      ${when ? `<span class="s-when">${esc(when)}</span>` : ""}</div>
    <h3>${esc(s.title)}</h3>
    ${meta ? `<div class="s-meta">${meta}</div>` : ""}
    ${goal ? `<div class="s-goal">${md(goal)}</div>` : ""}
    ${shown.length ? `<ul class="s-acc">` + shown.map(a =>
      `<li class="${a.done ? "on" : ""}">${a.done ? "✓" : "○"} ${esc(a.text)}</li>`).join("") +
      (acc.length > shown.length ? `<li class="more">+ ${acc.length - shown.length} more</li>` : "") + `</ul>` : ""}
    ${f("evidence") && s.evidence ? `<p class="s-ev">${esc(s.evidence)}</p>` : ""}
    ${f("refs") ? `<p class="s-refs">${
      (s.landed ? `landed <code>${esc(s.landed.slice(0, 7))}</code>${s.landed_repo ? ` in ${esc(s.landed_repo)}` : ""}` : "") +
      (s.branches?.length ? `${s.landed ? " · " : ""}${s.branches.map(x => `<code>${esc(x)}</code>`).join(" ")}` : "")
    }</p>` : ""}
  </article>`;
}

function exCover(specs) {
  const b = lastBoard;
  const title = EX.scope === "sprint" ? `Sprint ${EX.sprint || "—"}`
    : EX.scope === "delivered" ? "Delivered"
    : "Where the board stands";
  const sp = (b.sprints || []).find(x => x.name === EX.sprint);
  const sub = EX.scope === "sprint"
    ? (sp?.start && sp?.end ? `${sp.start} → ${sp.end}${sp.state ? ` · ${sp.state}` : ""}` : "no dates declared for this sprint")
    : EX.scope === "delivered" ? `landed in the last ${b.digest_days || 14} days`
    : `every story matching the current view`;
  const pts = specs.reduce((a, s) => a + (s.points || 0), 0);
  // A per-status breakdown of a single-status deck just says the total
  // twice — split the count only when there is something to split.
  const present = SPEC_ORDER.filter(st => specs.some(s => s.status === st));
  const counts = present.length < 2 ? "" : present.map(st =>
    `<div class="c-stat"><b style="color:${sv(st)}">${specs.filter(s => s.status === st).length}</b>
      <span>${esc(STATUS[st].label.toLowerCase())}</span></div>`).join("");
  return `<section class="slide cover">
    <div class="c-eyebrow">${esc(b.repo?.split("/").pop() || "repository")} · ${esc(b.integration_branch || "")}</div>
    <h1>${esc(title)}</h1>
    <p class="c-sub">${esc(sub)}</p>
    <div class="c-stats">
      <div class="c-stat"><b>${specs.length}</b><span>stor${specs.length === 1 ? "y" : "ies"}</span></div>
      ${pts ? `<div class="c-stat"><b>${pts}</b><span>points</span></div>` : ""}
      ${counts}
    </div>
    <div class="c-foot">
      <span>Derived from git — statuses are proven, never typed.</span>
      <span>${esc(new Date().toLocaleDateString())}${filterActive() ? " · filtered view" : ""}</span>
    </div>
  </section>`;
}

function exSlides(specs, bodies) {
  const per = exPerSlide();
  let html = exCover(specs);
  for (const st of SPEC_ORDER) {
    const inSt = specs.filter(s => s.status === st);
    if (!inSt.length) continue;
    const pages = chunk(inSt, per);
    pages.forEach((page, i) => {
      html += `<section class="slide">
        <header class="s-head" style="--st:${sv(st)}">
          <h2>${STATUS[st].ico} ${esc(STATUS[st].label)}</h2>
          <span>${inSt.length} stor${inSt.length === 1 ? "y" : "ies"}${pages.length > 1 ? ` · ${i + 1} of ${pages.length}` : ""}</span>
        </header>
        <div class="s-grid cols${page.length === 1 ? 1 : 2}">${
          page.map(s => exStoryCard(s, bodies[s.id])).join("")}</div>
      </section>`;
    });
  }
  return html;
}

// Slides are laid out in millimetres so the print engine gets a real page;
// on screen they are zoomed to fit whatever window is looking at them.
function exFit() {
  const px = 297 * (96 / 25.4); // one slide, full width
  document.getElementById("deck").style.setProperty("--dz",
    String(Math.min(1, (window.innerWidth - 48) / px)));
}
window.addEventListener("resize", () => { if (!deckWrap.hidden) exFit(); });

// The 16:9 page geometry is armed with the deck and disarmed with it, so
// printing the board itself still uses the printer's own paper.
const deckPage = document.getElementById("deckpage");
function closeDeck() {
  deckWrap.hidden = true;
  deckPage.media = "not all";
  document.body.classList.remove("deck-open");
}
document.getElementById("dk-back").addEventListener("click", closeDeck);
document.getElementById("dk-print").addEventListener("click", () => window.print());
document.addEventListener("keydown", e => { if (e.key === "Escape" && !deckWrap.hidden) closeDeck(); });

document.getElementById("ex-form").addEventListener("submit", async e => {
  if (e.submitter?.value === "cancel") return;
  e.preventDefault();
  const specs = exStories();
  if (!specs.length) return;
  const btn = document.getElementById("ex-build");
  const label = btn.textContent;
  const err = document.getElementById("ex-err");
  btn.disabled = true;
  err.textContent = "";
  const bodies = {};
  try {
    // Only the goal and the checklist need the story file; a titles-only
    // deck never touches the API.
    if (EX.fields.has("goal") || EX.fields.has("acceptance")) {
      btn.textContent = "reading stories…";
      const got = await Promise.all(specs.map(async s => {
        const r = await fetch("/api/specs/" + encodeURIComponent(s.id));
        if (!r.ok) throw new Error(`${s.id}: ${await r.text()}`);
        return r.json();
      }));
      for (const full of got) bodies[full.id] = full;
    }
    document.getElementById("deck").innerHTML = exSlides(specs, bodies);
    document.getElementById("dk-note").textContent =
      `${exSlideCount(specs)} slides · Save as PDF, landscape, no scaling`;
    exDlg.close();
    deckWrap.hidden = false;
    deckPage.media = "print";
    document.body.classList.add("deck-open");
    exFit();
    deckWrap.scrollTop = 0; // the overlay is the scroller, not the page
  } catch (e2) {
    err.textContent = "could not build the deck: " + e2.message;
  } finally {
    btn.disabled = false;
    btn.textContent = label;
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
