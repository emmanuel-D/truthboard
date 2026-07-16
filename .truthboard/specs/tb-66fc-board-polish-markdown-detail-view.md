---
id: tb-66fc
title: 'Board polish: markdown stories, card detail view, centered editor'
owner: emmanuel
branch: '*/tb-66fc-*'
paths: [internal/web/**, internal/audit/**]
epic: po-experience
priority: 1
---

## Goal

As a PO, the board should feel better than Trello: minimalist but
detailed. Today the editor dialog is pinned to the top-left corner, a
one-column board stretches cards across the whole screen, and a story's
goal/acceptance is raw markdown in a cramped textarea. Stories deserve a
proper detail view with rendered markdown, cards should be compact and
information-dense, and editing should preview what you write.

## Acceptance

- [x] Editor dialog opens centered with a dimmed backdrop, on every screen size
- [x] Clicking a card opens a **detail view**: rendered markdown (headings,
      bold, code, lists, checkboxes), meta chips, and a "derived truth"
      section (status evidence, branches, landing commit) — with an Edit button
- [x] Markdown is rendered safely: content is HTML-escaped before transforming
- [x] Cards are compact and scannable: clamped title, one-line evidence,
      id/priority/epic chips, owner initials — and an acceptance progress
      indicator (n/m checkboxes) computed server-side from the spec body
- [x] The editor has Write / Preview tabs so a PO sees the story as it will read
- [x] Columns keep a sane width when only one or two statuses exist
