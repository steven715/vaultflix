# ADR-0006: Classify playback stalls as starved-vs-codec in the HUD

- Status: Accepted
- Date: 2026-06-21

## Context

Videos are streamed as original files (no transcode/ABR), so a stall can have fundamentally different root causes, and the user needs to see at a glance whether playback is stuck and why. This is also the baseline observation layer before any future HLS/ABR rebuild.

## Decision

- A state machine classifies every stall into two families, read from `video.error` codes with a fixed priority and re-evaluated each tick: **starved** (buffering / `MEDIA_ERR_NETWORK`, recoverable by waiting) vs **codec** (`MEDIA_ERR_DECODE` / `MEDIA_ERR_SRC_NOT_SUPPORTED`, never recovers). A regression test guards that decode/unsupported never falls into the starved family.
- Downlink throughput is estimated from `video.buffered` **edge growth** × average bitrate (`file_size_bytes×8 / duration`), EWMA-smoothed, negative growth clamped to 0 — **not** from Resource Timing, because the backend answers each seek with a single open-ended `Range` 206 streamed over one connection, so the browser emits no per-chunk resource entries in steady state. RTT is reported separately as Resource Timing TTFB with a 10 s freshness gate.

## Alternatives rejected

- **A single generic "buffering" indicator** — would misattribute unrecoverable codec failures as transient, making the user wait forever.
- **Per-chunk Resource Timing throughput** (the original spec) — measurement showed zero steady-state samples; corrected to buffer-edge growth.

## Consequences

- Couples the HUD to HTMLMediaElement `MediaError` semantics.
- The codec path can't be reliably injected in-browser (React-controlled `<video src>` + auto-recovery) → covered by unit tests only.
- Throughput reads ~0 when the buffer is full (browser stops downloading) and spikes on refill — most accurate under sustained network pressure; zero backend cost.
- Relates to the browser-codec limitation that motivates future transcoding (see ROADMAP.md).
