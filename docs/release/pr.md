## Why

The conversion branch starts a conversion and then forgets about it. The
convert operation is discarded, nothing watches it, and the worker woken on
detach is a stub, so a queued conversion never runs and a driver restart during
a conversion leaves the volume unattachable for good.

This builds the tracking around the conversion that was already being started.

## What changed

**The conversion operation is kept.** `ConvertDiskType` now returns the
operation self link instead of discarding it, and it is recorded on the
PersistentVolume. That is what a user inspects to follow a conversion, and what
tells a later call — including one made by a restarted driver — that the volume
is still being converted.

**The detach worker is real.** `conversionWorkerLoop` was a stub that logged one
line. It now starts, cancels, or requeues the conversion based on the
VolumeAttributesClass the claim names at that moment.

**Conversions are completed by whoever sees the volume next.** Because nothing
is guaranteed to be watching when a conversion finishes, attach and expand
compare the disk's current type against the type it was converted from and
record the outcome if it is done. Without this, a driver restart mid-conversion
blocks the volume permanently.

**Only real conversions are recorded.** Previously any modify naming a matching
type stamped `converted-to`, so ordinary IOPS tuning on a disk that was created
with that type claimed a migration that never happened. A conversion that was
only ever queued is likewise dropped rather than recorded.

**The safety checks fail closed.** The attach and expand guards skipped
themselves entirely when the PersistentVolume could not be read, allowing an
attach during a conversion. They now refuse instead. A volume with no
PersistentVolume is still allowed, so statically provisioned volumes are
unaffected, and the guards are gated on the conversion feature flag so clusters
not using it are unchanged.

**Annotation writes are sound.** Removal used a JSON patch `remove`, which fails
when the annotation is absent and burned the full retry backoff on a no-op; it
now uses a merge patch. Requests the API server has already refused are no
longer retried, the underlying error survives the backoff instead of being
replaced by a timeout, and failed writes are logged rather than discarded.

**Terminal cases are rejected up front.** Multi-writer disks are refused before
calling the API, alongside the existing regional check. Retriable convert
failures — quota, rate limit, stockout, instant snapshot present — are explained
rather than reported as unspecified failures.

**Events.** `DiskTypeConversionStart`, `Complete`, `Retry` and `Cancelled` are
emitted on the PersistentVolume.

**VolumeAttributesClass is read from v1 or v1beta1.** It reached
storage.k8s.io/v1 in Kubernetes 1.34; earlier clusters serve v1beta1 only.
Asking for an unserved version returns the same not-found as a deleted class,
which the worker treats as "the user cancelled" — so on a 1.33 cluster every
queued conversion would have been silently withdrawn.

## Design notes

**`converted-from` is written when the conversion starts**, not when it
completes. Once the disk reports its new type the original is unrecoverable, and
it is what later lets the driver tell a finished conversion from a running one.
This differs from the PRD examples, which show both annotations appearing
together at the end.

**Target configuration is read from the live class, not recorded.** Work picked
up at detach acts on current intent, so changing the class on the claim
withdraws a conversion that has not started. Facts that cannot be recovered —
the operation self link and the original type — are the only things stored.

## Testing

Unit tests for the conversion trigger, error classification, annotation
handling, the detach worker's decisions, the completion fallback, event
emission, the fail-closed guards, and the API version fallback.

Verified on a 3-node kubeadm cluster (v1.33.13): applying a class to an attached
volume queues the conversion, detaching starts it, attaches are refused while
queued, regional disks are rejected without blocking the volume, and a matching
type writes no conversion state.

The conversion itself could not complete there because the project's
`compute.disks.convert` allowlist has lapsed and the API returns
`412 conditionNotMet` before any disk work begins.

## Follow-up

Background polling of the conversion operation, so completion is recorded
promptly rather than at the next volume operation. Conversions complete
correctly without it via the attach-time fallback.
