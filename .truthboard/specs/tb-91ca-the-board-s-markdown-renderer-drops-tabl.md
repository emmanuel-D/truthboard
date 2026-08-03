---
id: tb-91ca
title: The board's markdown renderer drops tables into a run-on paragraph
owner: emmanuel
branch: '*/tb-91ca-*'
paths:
    - internal/web/static/**
epic: po-experience
priority: 1
type: bug
---

## Goal

`md()` in `internal/web/static/app.js` handles headings, lists, task
checkboxes, fenced code and inline marks — but has no branch for tables.
A pipe row therefore falls through to `para.push(line.trim())` and every
row is joined with spaces into one paragraph:

    | derived | said as | |---|---| | done | delivered | | in-progress |
    being worked on | | stalled | paused | …

That is tb-346d's own body, whose table *is* the substance of the story:
the vocabulary contract between derived statuses and the words a
stakeholder reads. The board renders it as noise.

Tables are ordinary in a spec body — acceptance matrices, state
mappings, option comparisons — and a PO writing one in the intent editor
sees the same mess in the preview tab, which is where they would expect
to catch it.

Support the GitHub-flavoured form: a header row, a delimiter row, then
body rows, with `:` alignment honoured. A pipe line *without* a delimiter
row under it must stay a paragraph — pipes appear in prose and in code,
and promoting them to a table would be a worse bug than this one.

Wide tables must scroll inside their own container. The detail view is a
dialog; a table that widens it would push the Close button off-screen on
a phone, which is the one control that must always be reachable.

## Acceptance

- [x] A header row followed by a delimiter row renders as a real table in both the story detail view and the editor's preview tab
- [x] Column alignment from `:---`, `---:` and `:---:` is applied
- [x] A pipe-bearing line with no delimiter row beneath it still renders as a paragraph, not a table
- [x] Cell contents go through the same inline marks as everything else (code, bold, italic) and stay escaped — a table is not an HTML injection route
- [x] A wide table scrolls inside its own container; the dialog never widens and its footer controls stay reachable
- [x] Rows with fewer cells than the header do not shift content into the wrong column
