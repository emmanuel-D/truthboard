---
id: tb-b41b
title: A team arrives with a backlog it already has
owner: emmanuel
branch: '*/tb-b41b-*'
paths:
    - internal/adopt/**
    - internal/spec/**
    - cmd/truthboard/**
    - docs/**
epic: dev-experience
priority: 3
type: story
---

## Goal

Adoption assumes an empty backlog. `init --agents` and `init
--workspace` wire a repo beautifully and then hand over a specs
directory with nothing in it — while the team's actual work sits in
GitHub Issues, Jira or Linear, hundreds of items deep. The honest paths
available today are retyping it or abandoning it, and both mean the
board tells the truth about a fraction of the work while the real
backlog stays where it was. A tracker that only knows about the stories
filed after Tuesday is not a tracker anyone runs their week on.

What makes this tractable is that import is a *one-way, one-time* move
into files: read the source, write `.truthboard/specs/*.md`, commit as
intent. No sync, no live integration, no second source of truth — the
moment the markdown exists, git derives everything and the origin stops
mattering. Anything that would keep the external tracker authoritative
is out of scope, and should stay out.

The interesting question is not the transport, it is fidelity: what an
imported item *lacks*. Most of them have no acceptance criteria and no
scope paths, so a naive import produces hundreds of stories that can
never be verified and permanently muddy the drift report. Import must
land them visibly incomplete rather than quietly fake.

## Acceptance

- [x] `truthboard import` reads an existing backlog — GitHub Issues via `gh` and a documented file format (CSV or JSON export) covering Jira and Linear — and writes one spec file per item — proof: `TestCSVExportIsRead`
- [x] Title, description, owner, labels-as-epic and priority are carried across where the source has them, and mapping choices are stated rather than guessed silently — proof: `TestMappingIsStated`
- [x] Imported stories with no acceptance criteria are marked as such and are excluded from unverified-acceptance drift until someone writes them, so a large import cannot turn the board red overnight — proof: `TestImportedStoriesArriveVisiblyIncomplete`
- [x] `--dry-run` reports exactly what would be written, with the count and a sample, and writes nothing — proof: `internal/importer/importer.go`
- [x] Re-running the import does not duplicate what it already wrote, and never overwrites a spec a human has since edited — proof: `TestReImportDoesNotDuplicateOrOverwrite`
- [x] Statuses are not imported: an item that the source called "done" is derived from git like everything else, and the docs say so plainly — proof: `TestImportedStoriesArriveVisiblyIncomplete`
- [x] The import is a single reviewable intent commit, and closed or cancelled source items are skipped by default — proof: `TestClosedItemsAreSkippedByDefault`
- [x] `go test ./...` passes — proof: `ci:build`
