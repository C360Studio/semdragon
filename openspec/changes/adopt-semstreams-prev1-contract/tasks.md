# Tasks: adopt-semstreams-prev1-contract

## 1. Contract inventory and TDD gates

- [x] 1.1 Create the reviewed predicate rename ledger, including producer,
  consumers, cardinality, owner, write mode, UI mapping, and test evidence.
- [x] 1.2 Create the reviewed writer-owner matrix for every
  `ENTITY_STATES` mutation path; classify framework, product, and temporary
  migration writers.
- [x] 1.3 Inventory relationship-valued triples and add a failing contract test
  requiring `message.EntityReferenceDatatype` for graph edges.
- [x] 1.4 Add beta.158-compatible predicate and entity-ID audit tasks. Use
  scoped roots until upstream SemStreams #667 is fixed.
- [x] 1.5 Capture the baseline findings and classify intentional negative
  fixtures without broad file/directory exemptions.

## 2. Canonical graph corpus

- [ ] 2.1 Rename stored predicates to exact three-part lower-kebab names across
  domain vocabulary, graph producers, reconstruction, rule JSON, queries,
  fixtures, and documentation.
- [ ] 2.2 Replace dynamic predicate names with repeated canonical relationships,
  bounded structured values, or first-class entities as approved in the ledger.
- [ ] 2.3 Replace trailing-dot and malformed query prefixes with canonical
  one-through-six-segment prefixes.
- [ ] 2.4 Type every relationship triple as `@id`; update reconstruction and
  relationship query tests.
- [ ] 2.5 Remove copied gated-DAG marker overrides and use the framework's
  canonical default vocabulary.
- [ ] 2.6 Update UI predicate projections and graph relationship consumers;
  regenerate entity schema, OpenAPI, and TypeScript types.

## 3. Verification and handoff to beta.158

- [ ] 3.1 Predicate audit has zero findings.
- [ ] 3.2 Entity-ID audit has zero findings.
- [ ] 3.3 Relationship coverage test proves every graph edge is typed `@id`.
- [ ] 3.4 `go build ./...`, unit tests, UI checks, and schema freshness gates
  pass while still pinned to beta.135.
- [ ] 3.5 Write and review the literal fresh-volume wipe/reseed and
  restart-without-write replay runbook.
- [ ] 3.6 Record the approved dependency handoff to
  [semdragon#4](https://github.com/C360Studio/semdragon/issues/4); do not bump
  to beta.158 in this change.
