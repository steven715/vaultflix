# ADR-0009: Position streaming as Jellyfin-scale on-the-fly, not YouTube-scale pre-processing

- Status: Accepted
- Date: 2026-08-24

## Context

The library is ~864GB across three playback classes: direct 285 videos / 521GB (MP4+h264+aac,
native `<video>` + Range), remux 124 / 200GB (MKV/AVI with browser-compatible codecs, served as
on-the-fly HLS), transcode 89 / 143GB (incompatible codecs, not yet playable — Phase 2).

Until now the scale premise was never stated as a first-class constraint — only an offhand
"單人使用同時 session 數有限，風險低" buried in the video-compat Phase 1 design's risk section, and
the word「個人」in CLAUDE.md's overview. Debugging a remux stutter (mkv `-ss` keyframe-seek landing
one GOP early, producing segments misaligned with the manifest) forced the question: keep investing
in the on-the-fly pipeline, or switch to YouTube-style ingest-time pre-processing (transcode/remux
everything once into a canonical format, serve statically, zero runtime compute)?

The pre-processing route is affordable at this scale (~+350GB disk, one-time CPU) and would delete
the entire class of on-the-fly bugs. But the operator's actual usage is: single user (occasionally a
few via ngrok), LAN-first, originals on read-only-mounted disks as the source of truth.

## Decision

- **The operating scenario is Jellyfin's**: personal media server, single-digit concurrent streams,
  LAN-first with occasional ngrok exposure. All streaming-architecture decisions are sized against
  this premise, and「對目前專案規模是否過度設計」in CLAUDE.md review guidance refers to it.
- **Stay on the on-the-fly route** (keep originals untouched, remux/transcode at play time),
  same as Jellyfin's DirectPlay > DirectStream > Transcode ladder. Fix the remux alignment bug
  within this architecture rather than abandoning it.
- Buffer policy is business config, not player-hardcoded: execution necessarily lives in each
  player (browser for direct, hls.js for remux), but tunables should flow from the backend per the
  runtime-adjustability principle. LAN bandwidth cost ≈ 0 ⇒ prefer larger forward buffers
  (jellyfin-web's 6-second-buffer regression is the cautionary tale).

## Alternatives rejected

- **YouTube-style pre-processing now** (pre-remux the 124, batch-transcode the 89, serve all as
  direct) — architecturally simpler at runtime and cheap at 1TB scale, but: derived-file management
  (media disks are `:ro`; derivatives need separate storage and re-generation on source change),
  loses "original file is the truth", and solves a concurrency problem the scenario doesn't have.
  Revisit if remote multi-user access becomes routine instead of occasional — the bottleneck order
  is upload bandwidth first, transcode CPU second (see docs/streaming.md §5).
- **Scaling the on-the-fly pipeline for many users** (transcode farm, more cores) — wrong tool;
  at real scale the industry answer is pre-processing + CDN, not bigger live encoders.

## Consequences

- On-the-fly complexity (keyframe indexing, segment alignment, caches) remains ours to maintain;
  the 2026-08 mkv seek-misalignment bug must be fixed, not routed around.
- Phase 2 transcode inherits the premise: hardware acceleration and/or concurrent-transcode limits
  are the scaling levers, matching Jellyfin — never fan-out.
- ngrok sharing accepts the home-uplink bandwidth ceiling (~5–8 concurrent 1080p on 40 Mbps up);
  no CDN/object-storage distribution is planned.
- The scenario premise is now recorded here and summarized in CLAUDE.md's overview; design reviews
  can cite it instead of re-deriving it.
- Reference knowledge distilled from this discussion lives in [docs/streaming.md](../streaming.md).
