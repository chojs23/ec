# Plan: Sidecar State File for Cross-Session Resolution Tracking

## Problem

`ec` currently expresses all per-conflict state through the presence or absence
of conflict markers in the merged file:

- A conflict marker block → unresolved
- No marker → resolved (bytes are just text)

This matches git's model, but it has a UX cost. If a user resolves some hunks,
writes the file (`w`), quits, then relaunches `ec`, the resolved hunks are
invisible: only the remaining marker blocks show up. The user loses visibility
into what they already decided.

See the prior discussion around the neo-script rebase for context: pure option
A correctly fixes the algorithm-divergence bug, but it cannot also preserve
cross-session resolution history because there is nowhere to store that
history.

## Goal

After an interactive session ends (quit, crash, or intentional pause), the
next `ec` launch on the same merged file should:

1. Show every hunk that was in the original conflict — resolved and unresolved
   together.
2. Mark each resolved hunk with its resolution kind (Ours / Theirs / Both /
   None / Manual).
3. Allow the user to change their mind (flip a resolved hunk to a different
   resolution or revert it).
4. Leave git unaffected: `git status`, `git diff`, and downstream tooling keep
   seeing the file exactly as `ec` would have written it without this feature.

## Non-Goals

- Replacing conflict markers with a custom encoding inside the merged file.
  Git must keep working; the merged file stays valid.
- Persisting UI state other than resolution decisions (no cursor position, no
  scroll state — just the resolution intent).
- Sharing state across machines or users. The sidecar is local to one working
  copy.
- Handling concurrent `ec` sessions on the same file. First-writer-wins is
  acceptable; optional file lock is an open question.

## Approach

Store resolution state in a sidecar JSON file next to the merged file.

- Path: `<merged-path>.ec-state.json`
- Lifecycle: created on first `w`, updated on every subsequent `w`, deleted
  when the file becomes fully resolved.
- Format: stable JSON keyed by a fingerprint of the canonical 3-way inputs so
  stale sidecars from a previous rebase/merge cannot pollute a new one.

The canonical document is still built from the on-disk merged file plus the
diff3 view (current behavior). The sidecar overlays resolution intent on top,
and `ec` uses the overlay to decide which hunks to show as resolved.

## Sidecar File Format

```json
{
  "version": 1,
  "fingerprint": "sha256:<hex>",
  "canonical_hunks": [
    { "index": 0, "ours_len": 917, "theirs_len": 1138, "base_len": 1111 },
    ...
  ],
  "resolutions": [
    { "index": 0, "kind": "ours" },
    { "index": 3, "kind": "manual", "bytes_b64": "..." }
  ]
}
```

- `version`: schema version. Bumped on breaking changes; old sidecars with
  unknown versions are ignored (and deleted) rather than refused.
- `fingerprint`: SHA-256 over the concatenation of base, local, remote stage
  bytes (in that order, with NUL separators). Guards against reuse after the
  rebase is aborted and restarted.
- `canonical_hunks`: lightweight structural snapshot of the diff3 view at
  save time. If the current diff3 view disagrees (different hunk count or
  sizes), the sidecar is discarded. Keeps us safe when stage files change.
- `resolutions`: sparse list of resolved hunks by canonical index. `kind` is
  one of `ours | theirs | both | none | manual`. `manual` resolutions include
  `bytes_b64` (base64-encoded resolution bytes).

## Canonical Index Semantics

The canonical index is the index of the hunk in the diff3 view (the "real"
set of conflicts from the 3-way merge). That's the one stable numbering we
have; the disk-view index shifts every time a hunk is resolved. Using diff3
indices means sidecar entries survive intermediate writes.

Caveat: the `LoadCanonicalDocument` changes in the last fix use the on-disk
doc as canonical when counts match, and the diff3 doc as canonical only in
fallback. For the sidecar, we always reference indices in the diff3 view
regardless of which document the TUI displays — the sidecar loader resolves
indices back to display positions at load time.

## Implementation Steps

### 1. State module

- New package `internal/sidecar` with:
  - `type State struct { Version int; Fingerprint string; CanonicalHunks []HunkShape; Resolutions []Resolution }`
  - `Load(mergedPath string) (*State, error)` — reads and parses the sidecar
    next to `mergedPath`, returns nil when missing.
  - `Save(mergedPath string, s *State) error` — atomic write (tempfile +
    rename).
  - `Delete(mergedPath string) error` — removes the sidecar.
  - `Fingerprint(base, local, remote []byte) string` — SHA-256 of the three.

### 2. Engine hooks

- `engine.State` gains:
  - `ExportResolutions() []sidecar.Resolution` — returns the per-hunk
    resolution intent as a list keyed by diff3 index.
  - `ApplyResolutions(rs []sidecar.Resolution) error` — replays each entry
    onto the appropriate conflict. Validates that the target canonical hunk
    still exists and the shape matches; returns an error otherwise so the
    caller can discard the sidecar.
- `engine.State` already exposes `ApplyResolution` and `ManualResolved`; these
  are the lower-level primitives the new helpers wrap.

### 3. Loader integration

- `mergeview.LoadCanonicalDocument` keeps its current behavior.
- `tui.loadResolverDocumentState`:
  - After building the runtime state, call `sidecar.Load(opts.MergedPath)`.
  - If a sidecar exists and its fingerprint + hunk shape match, call
    `state.ApplyResolutions(sidecar.Resolutions)`.
  - If the sidecar is stale or fingerprint mismatches, delete it and proceed
    without overlay.

### 4. Writer integration

- `tui.writeResolved`:
  - After writing the merged file, snapshot current resolution intent via
    `state.ExportResolutions()`.
  - If there is anything to save AND the file still has unresolved conflicts,
    save the sidecar.
  - If the file is fully resolved (no conflicts left), delete the sidecar.

### 5. `--apply-all` interaction

- `engine.ApplyAllAndWrite` deletes the sidecar after a successful write. No
  state carried over once a bulk resolution has committed.

### 6. Cleanup paths

- `engine.CheckResolvedFile` (used by `--check` and by the file picker to show
  resolved-state) ignores sidecars: conflict-marker presence is still the
  authoritative signal.
- After a rebase/merge is completed by git (`git rebase --continue`), sidecars
  for files that are no longer in `git ls-files -u` should be cleaned up
  opportunistically. Simplest hook: on `ec` startup, any sidecars under the
  repo root whose merged file has zero conflicts get deleted.

## Display Semantics

The UI renders all hunks from the diff3 view when a sidecar-matched overlay is
applied. Each hunk shows its resolution kind. The "Ours" / "Theirs" panes
continue to read from the disk-preferred canonical so the bytes the user sees
match what's on disk.

If the disk view and diff3 view disagree in count (user resolved one inline
without using `ec`), the sidecar still works but the pre-resolved inline hunk
stays invisible (we can't reliably align — same limitation as the prior fix).
The sidecar entries correspond to hunks `ec` was aware of when the save
happened; bytes resolved without `ec` are out of scope.

## Edge Cases

- **Sidecar older than current stage files**: fingerprint check catches this,
  discard and proceed.
- **User reverts a resolution in-session**: export should reflect current
  state (no lingering entry for a cleared resolution).
- **User runs `ec` with a different `$EDITOR` that modifies text outside
  conflicts**: sidecar's hunk-shape check (Ours/Theirs/Base byte lengths)
  acts as a cheap integrity guard. Any mismatch → discard overlay, fall back
  to plain load.
- **Concurrent `ec` sessions on the same file**: last `w` wins. Consider an
  advisory lock (`flock`) on the sidecar if this becomes a real issue. Open
  question.
- **Merged file deleted then recreated**: on load, absent merged file falls
  through the existing code paths; sidecar discard is driven by the
  fingerprint check the next time we do have a file.

## Testing

- `internal/sidecar`:
  - Unit tests for Load/Save/Delete round-trips.
  - Fingerprint stability across byte-identical inputs, change on any
    diverging byte.
- `internal/engine`:
  - `ExportResolutions` / `ApplyResolutions` round-trip equality.
  - Resolution application rejects shape-mismatched entries.
- `internal/tui`:
  - Integration test: resolve one hunk, write, reload from disk → hunk shows
    as resolved with correct kind and bytes.
  - Integration test: stale sidecar (fingerprint mismatch) is silently
    discarded.
  - Integration test: fully resolved file causes sidecar deletion on write.
- `cmd/ec` end-to-end: smoke test using `testscript` that simulates a
  quit-and-resume.

## Rollout

- Hidden behind `EC_SIDECAR_STATE=1` env var for one release, default off.
- After a release with no regressions, flip default to on.
- Document in README under a "Resumable sessions" heading.

## Open Questions

- Should we encrypt / obfuscate `manual` resolution bytes in the sidecar?
  They are already on disk in the merged file during editing, so probably
  not worth the hassle.
- Should the sidecar path be `<merged>.ec-state.json` or hidden
  `.<merged>.ec-state.json`? Hidden variant avoids cluttering `ls`; visible
  variant is easier to inspect and delete manually. Leaning visible.
- Gitignore: recommend users add `*.ec-state.json` to a repo-wide gitignore
  or a global gitignore. Should `ec` ever auto-modify `.gitignore`? No — too
  invasive.

## Out of Scope / Follow-ups

- A richer "session view" that lists all resolved hunks across multiple files
  in the rebase/merge.
- Sharing resolutions across machines (would require syncing stage
  fingerprints too, which are derived from commit hashes).
- Integrating with `git rerere`. `rerere` solves a different problem
  (remembering resolutions across unrelated merges) and has its own storage;
  bolting onto it is not straightforward.
