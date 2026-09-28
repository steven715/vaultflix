# ADR-0010: Performers and Makers are entities, not Tags; Enrichment only produces Suggestions

- Status: Accepted
- Date: 2026-09-28

## Context

Two parallel models describe the same things today:

- Tag categories `actor` (人物) and `studio` (工作室), hand-applied to Videos.
- Scraped data from the Enrichment pipeline (PR #17/#18): an `actresses` table (Japanese name,
  avatar) linked via `video_actresses`, and `videos.maker` / `videos.label` columns.

A Scraper-found performer never becomes a Tag, and an `actor` Tag never links to an actress record.
The roadmap's next Enrichment stage (an LLM suggesting classifications from Metadata) cannot be
designed until it is clear whether "who is in this video" is a Tag or something else.

The roadmap also called the whole pipeline「自動標籤 / auto-tagging」, while the implementation stages
every scraped change as a `metadata_suggestions` row that the user must accept or reject.

## Decision

1. **Performer** (the person) and **Maker** (the company) are first-class concepts with their own
   identity — Performer has an avatar and aliases and can be scraped. They are not Tags.
2. **Tag** is reduced to the user's own classification: categories `genre` and `custom`. The `actor`
   and `studio` categories are deprecated and will be migrated into Performer / Maker.
3. The canonical name is **Performer**, not Actress: gender-neutral and not JAV-specific.
4. **Enrichment** is the umbrella for every external Metadata source (Scraper now; LLM and
   frame analysis later). Every source produces **Metadata Suggestions** that do nothing until the
   user accepts them. Nothing is auto-applied; the term「自動標籤」is retired.

Vocabulary is defined in [CONTEXT.md](../../CONTEXT.md).

## Alternatives rejected

- **Keep both models** (Tags as the user's view, actresses as scraped fact): an LLM stage asked to
  "suggest tags" would not know whether a performer belongs in Tag or Actress, and the UI would
  keep showing two disconnected lists of the same people.
- **Make performers a Tag category** and drop the `actresses` table: loses avatar, Japanese name and
  alias data that Scrapers already provide, and makes a Tag carry fields other categories don't have.
- **Auto-apply high-confidence Enrichment results**: scraped and LLM data is wrong often enough
  (wrong Code match, variant titles) that silent writes would corrupt Metadata with no audit trail.

## Consequences

- A data migration is required (`actor`/`studio` Tags → Performer/Maker) plus renaming
  `actresses` → performers in code, DB and API. Tracked in ROADMAP; not yet done.
- Code currently says `Actress`; until the rename lands, docs use Performer and treat `Actress` as the
  legacy identifier.
- Every new Enrichment source must write Suggestions, never Metadata directly — slower to populate
  the Library, but every change is reviewable and reversible.
