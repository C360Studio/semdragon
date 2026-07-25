# framework-migration Specification

## Purpose

Define the sequencing and evidence required to adopt a breaking SemStreams
pre-v1 release while keeping framework and product responsibilities separate.

## ADDED Requirements

### Requirement: Contract-first dependency adoption

The system SHALL make the product graph corpus valid for the target SemStreams
contract before changing the module dependency.

#### Scenario: Dependency-only bump is rejected

- **WHEN** predicate, entity-ID, relationship, or ownership audits have findings
- **THEN** the beta.158 dependency task remains blocked

### Requirement: Framework primitives replace framework duplication

Semdragons SHALL use SemStreams for graph mutation/validation, lifecycle
transitions/history, rule evaluation, gated-DAG scheduling/recovery, and generic
agentic tool execution. Semdragons SHALL retain only product semantics and thin
composition adapters in those areas.

#### Scenario: Missing framework primitive

- **WHEN** a required framework-shaped capability is missing or defective
- **THEN** the team files or reuses a verified SemStreams issue
- **AND** any temporary downstream adapter is minimal, isolated, and linked to
  the upstream issue

### Requirement: PR-gated SDD delivery

Each migration slice SHALL have an OpenSpec task boundary, a linked GitHub
issue, tests appropriate to its risk, and required role reviews before
integration.

#### Scenario: Gate is not ready

- **WHEN** a required architect, Go reviewer, Svelte reviewer, or
  technical-writer sign-off requests changes
- **THEN** the pull request remains draft or blocked

### Requirement: Exact target and evidence

The adoption gate SHALL record the exact SemStreams tag/revision, Go toolchain,
rendered configuration, fresh-state procedure, and verification results.

#### Scenario: Latest tag moves during migration

- **WHEN** SemStreams publishes a newer tag before the adoption PR is finalized
- **THEN** the team re-runs the delta audit
- **AND** either updates the target with evidence or records why the newer tag
  is deferred to a new issue and gate
