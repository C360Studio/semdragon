# Proposal: cut over SemStreams beta.158 rule contracts

## Why

Semdragon issue
[#59](https://github.com/C360Studio/semdragon/issues/59) is the next bounded
implementation packet under issue #4. The repository is prepared for
SemStreams beta.158 but remains pinned to beta.135. The latest beta.158 tag
also predates SemStreams PR #708, which removes duplicate rule-pack projection
authoring by deriving least authority from statically provable
`replace_owned` actions.

The historical `adopt-semstreams-prev1-contract` change is the beta.135
pre-adoption record. Rewriting it would blur completed preparation with the
new dependency and runtime cutover. This successor change owns only the exact
revision, rule configuration, retry expression, entity-reference, gated-DAG
default, and supported prefix-boundary adaptations required by issue #59.

## What Changes

- Pin SemStreams to
  `v1.0.0-beta.158.0.20260728003411-89617f32dc36`, merge
  `89617f32dc3657409c2294d75ffcdc74decf014e`.
- Migrate all three shipped rule processors to stable pack identity, explicit
  graph-integration intent, and supported `ENTITY_STATES` watch configuration.
- Omit `projection_contracts` for the current lifecycle-only pack. It has no
  `replace_owned` action from which rule-pack ownership should derive.
- Compare retry attempts with each quest's `quest.attempts.max` value instead
  of the literal `3`.
- Replace copied entity-reference values with the SemStreams constant.
- Let `gateddagexec.DefaultConfig()` supply canonical gated-DAG markers.
- Adopt SemStreams entity-prefix validation and matching only at their
  supported full-ID and query-prefix boundaries.

## Capabilities

### New Capabilities

- `semstreams-rule-cutover`: exact dependency, rule-pack authoring, canonical
  framework defaults, and verification requirements for issue #59.

## Impact

- **Dependency**: `go.mod` and `go.sum`.
- **Configuration**: the default, SemSource, and tier-1 E2E profiles.
- **Rules**: quest failure retry and triage conditions.
- **Go integration**: entity-reference constants, questdag executor
  configuration, and graph-source prefix boundaries.
- **Tests and operations**: focused RED/GREEN contracts, full repository
  gates, and execution of the reviewed fresh-state/replay procedure.

This packet does not close issue #4 or migrate any deferred authoritative
product writer.
