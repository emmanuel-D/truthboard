---
id: tb-1be0
title: 'Embedded LLM: draft a backlog and narrate the sprint review'
owner: emmanuel
branch: '*/tb-1be0-*'
paths:
    - internal/llm/**
    - cmd/**
epic: agent-loop
priority: 3
---

## Goal

Blueprint §6.2: with an API key (Anthropic) or a local Ollama host, the
binary itself can run LLM workflows without an external agent.
`truthboard draft "concept"` turns a product concept into an epic plus
fully-formed stories (goal + Gherkin-style acceptance) written through the
same path as create_spec. `truthboard review [sprint]` reads what landed
(specs + commits) and synthesizes a human-readable sprint review narrative
— the digest's facts, told as a story. Both are additive: MCP-driven
external agents remain the primary loop.

## Acceptance

- [ ] Provider configured via env (ANTHROPIC_API_KEY or OLLAMA_HOST); a clear error when neither is set
- [ ] `truthboard draft` writes real spec files (never placeholders) and prints their ids
- [ ] `truthboard review` produces a narrative summary of a sprint from specs + landed commits
- [ ] No LLM calls happen unless one of these commands is explicitly invoked
