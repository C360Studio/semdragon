/**
 * Knowledge Graph Types — Semdragons
 *
 * Types for visualizing the semantic knowledge graph via the graph-gateway
 * GraphQL endpoint at /graph-gateway/graphql.
 *
 * The graph uses RDF-like triples (Subject-Predicate-Object) where:
 * - Entities have 6-part IDs: org.platform.domain.system.type.instance
 *   Example: "c360.prod.game.board1.quest.abc123"
 * - Relationships are triples where the object references another entity
 * - Properties are triples where the object is a literal value
 *
 * Semdragons entity types: quest, agent, party, guild, battle
 */

// =============================================================================
// Entity ID Types
// =============================================================================

/**
 * Parsed components of a 6-part entity ID.
 * Format: org.platform.domain.system.type.instance
 * Example: "c360.prod.game.board1.agent.dragon"
 */
export interface EntityIdParts {
  org: string;
  platform: string;
  domain: string;
  system: string;
  /** Semdragons entity types: quest | agent | party | guild | battle */
  type: string;
  instance: string;
}

// =============================================================================
// Core Graph Types
// =============================================================================

/**
 * A triple property representing a fact about an entity.
 * When object is a literal value, it is a property.
 * When object is another entity ID, it becomes a relationship.
 */
export interface TripleProperty {
  /** 3-part dotted notation: domain.category.property (e.g. "quest.lifecycle.status") */
  predicate: string;
  /** Literal value (number, string, boolean) or entity ID reference */
  object: unknown;
  /** 0.0 - 1.0 */
  confidence: number;
  /** Origin component that created this fact */
  source: string;
  /** Unix milliseconds */
  timestamp: number;
}

/**
 * A relationship between two entities (edge in the graph).
 * Created from triples where the object references another entity.
 */
export interface GraphRelationship {
  /** Unique relationship ID — "sourceId:predicate:targetId" */
  id: string;
  sourceId: string;
  targetId: string;
  /** Relationship type (e.g. "party.formation.lead") */
  predicate: string;
  /** 0.0 - 1.0 */
  confidence: number;
  /** Unix milliseconds */
  timestamp: number;
}

/**
 * A graph entity (node in the graph).
 */
export interface GraphEntity {
  /** Full 6-part entity ID */
  id: string;
  /** Parsed ID components */
  idParts: EntityIdParts;
  /** Literal-valued triples */
  properties: TripleProperty[];
  /** Relationships where this entity is source */
  outgoing: GraphRelationship[];
  /** Relationships where this entity is target */
  incoming: GraphRelationship[];
}

// =============================================================================
// API Response Types (raw from GraphQL)
// =============================================================================

/**
 * Backend triple structure from GraphQL API.
 */
export interface BackendTriple {
  subject: string;
  predicate: string;
  object: unknown;
}

/**
 * Backend entity structure from GraphQL API.
 */
export interface BackendEntity {
  id: string;
  triples: BackendTriple[];
}

/**
 * Backend edge structure from GraphQL API.
 */
export interface BackendEdge {
  subject: string;
  predicate: string;
  object: string;
}

/**
 * GraphQL pathSearch query result.
 */
export interface PathSearchResult {
  entities: BackendEntity[];
  edges: BackendEdge[];
}

/**
 * Community summary returned by a global (NLQ) search.
 */
export interface CommunitySummary {
  communityId: string;
  text: string;
  keywords: string[];
}

/**
 * Explicit relationship returned by a global (NLQ) search.
 */
export interface SearchRelationship {
  from: string;
  to: string;
  predicate: string;
}

/**
 * NLQ classification metadata returned via GraphQL extensions.
 * Available in semstreams alpha.17+.
 */
export interface ClassificationMeta {
  tier: number;
  confidence: number;
  intent: string;
}

/**
 * Parsed result from the globalSearch GraphQL operation.
 */
export interface GlobalSearchResult {
  entities: BackendEntity[];
  communitySummaries: CommunitySummary[];
  relationships: SearchRelationship[];
  count: number;
  durationMs: number;
  classification?: ClassificationMeta;
}

// =============================================================================
// Utility Functions
// =============================================================================

/**
 * Parse a 6-part entity ID into its components.
 * Returns partial/unknown parts for malformed IDs rather than throwing.
 */
export function parseEntityId(id: string): EntityIdParts {
  const parts = id.split('.');
  if (parts.length !== 6) {
    return {
      org: parts[0] || 'unknown',
      platform: parts[1] || 'unknown',
      domain: parts[2] || 'unknown',
      system: parts[3] || 'unknown',
      type: parts[4] || 'unknown',
      instance: parts[5] || 'unknown',
    };
  }
  return {
    org: parts[0],
    platform: parts[1],
    domain: parts[2],
    system: parts[3],
    type: parts[4],
    instance: parts[5],
  };
}

/**
 * Generate a unique relationship ID from its three components.
 */
export function createRelationshipId(
  sourceId: string,
  predicate: string,
  targetId: string,
): string {
  return `${sourceId}:${predicate}:${targetId}`;
}

/**
 * Check if a triple's object is an entity reference (vs literal value).
 * Entity IDs have exactly 6 dot-separated parts.
 */
export function isEntityReference(object: unknown): object is string {
  if (typeof object !== 'string') return false;
  return object.split('.').length === 6;
}

/**
 * Get display label for an entity — prefers human-readable names from triples,
 * falls back to the instance part of the ID.
 */
export function getEntityLabel(entity: GraphEntity): string {
  const fallback = entity.idParts.instance || entity.id;
  const type = entity.idParts.type;

  const val = (pred: string): string => {
    const t = entity.properties.find((tr) => tr.predicate === pred);
    if (!t || t.object == null) return '';
    return String(t.object);
  };

  switch (type) {
    case 'agent':
      return val('agent.identity.display_name') || val('agent.identity.name') || fallback;
    case 'quest':
      return val('quest.identity.name') || val('quest.identity.title') || fallback;
    case 'battle':
      return val('battle.identity.name') || fallback;
    case 'party':
      return val('party.identity.name') || fallback;
    case 'guild':
      return val('guild.identity.name') || fallback;
    default:
      return fallback;
  }
}

/**
 * Get the entity type from its parsed ID (quest | agent | party | guild | battle).
 */
export function getEntityTypeLabel(entity: GraphEntity): string {
  return entity.idParts.type || 'unknown';
}

// =============================================================================
// Semdragons Game Entity Types
// =============================================================================

/**
 * Game entity types for the Semdragons domain.
 * These are the valid values for the 5th part of a game entity ID.
 */
export type GameEntityType =
  | 'quest'
  | 'agent'
  | 'party'
  | 'guild'
  | 'battle'
  | 'peer_review'
  | 'unknown';

/**
 * Check if an entity ID belongs to the game domain (3rd part === "game").
 * Used to filter semdragons game entities from other domain entities.
 */
export function isGameEntity(entityId: string): boolean {
  const parts = entityId.split('.');
  return parts.length === 6 && parts[2] === 'game';
}

/**
 * Get the typed game entity type from an entity's parsed ID.
 * Returns 'unknown' for entity types not in the semdragons game domain.
 */
export function getGameEntityType(entity: GraphEntity): GameEntityType {
  const t = entity.idParts.type.toLowerCase();
  const known: GameEntityType[] = ['quest', 'agent', 'party', 'guild', 'battle', 'peer_review'];
  return (known.includes(t as GameEntityType) ? t : 'unknown') as GameEntityType;
}

// =============================================================================
// Filter Types
// =============================================================================

/**
 * Filters for the game entity graph visualization.
 */
export interface GraphFilters {
  search: string;
  /** Entity types to show (from 6-part ID type segment). Empty = show all. */
  types: string[];
  /** Hide edges below this confidence score (0.0 - 1.0). */
  minConfidence: number;
  /** Unix ms range, null = all time. */
  timeRange: [number, number] | null;
}

/**
 * Default filter values — show all game entities with no restrictions.
 */
export const DEFAULT_GRAPH_FILTERS: GraphFilters = {
  search: '',
  types: [],
  minConfidence: 0,
  timeRange: null,
};
