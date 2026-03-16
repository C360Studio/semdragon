/**
 * Entity Color Mapping for Graph Visualization
 *
 * Maps entity types to distinct colors for the graph visualization.
 * Entity IDs have format: org.platform.domain.system.type.instance
 * Color is assigned based on the entity type (5th part of the 6-part ID).
 *
 * Includes both game entity types (quest, agent, party, guild, battle)
 * and semsource knowledge graph types (file, function, class, etc.).
 */

import type { EntityIdParts } from '$lib/api/graph-types';

// =============================================================================
// Game Entity Type Colors
// =============================================================================

/**
 * Color mapping for entity types in the graph visualization.
 * Includes both game entity types and semsource knowledge graph types.
 */
export const ENTITY_TYPE_COLORS: Record<string, string> = {
	// Game entity types
	quest: '#a855f7', // Purple — quest work items
	agent: '#22d3ee', // Cyan — the adventurers
	party: '#22c55e', // Green — temporary groups
	guild: '#eab308', // Gold — specialized collectives
	battle: '#ef4444', // Red — boss battle reviews
	peer_review: '#f97316', // Orange — peer evaluations
	storeitem: '#fb923c', // Orange-light — store catalog items
	execution: '#a78bfa', // Violet-light — execution records
	endpoint: '#38bdf8', // Sky — API endpoints

	// Semsource / knowledge graph entity types
	file: '#3b82f6', // Blue — source files
	doc: '#f59e0b', // Amber — documents (semsource doc entities)
	document: '#f59e0b', // Amber — documentation
	function: '#14b8a6', // Teal — functions/methods
	class: '#6366f1', // Indigo — classes/interfaces
	module: '#8b5cf6', // Violet — modules/packages
	package: '#0ea5e9', // Sky — packages
	config: '#64748b', // Slate — configuration files
	interface: '#818cf8', // Indigo-light — interfaces
	method: '#2dd4bf', // Teal-light — methods
	field: '#94a3b8', // Slate-light — fields
	group: '#78716c', // Stone — hierarchy groups
	container: '#a8a29e', // Stone-light — hierarchy containers

	// Fallback for unknown entity types
	unknown: '#6b7280' // Gray
};

/**
 * Get the visualization color for a game entity type.
 * Returns gray for entity types not in the game domain.
 */
export function getEntityTypeColor(type: string | undefined): string {
	if (!type) return ENTITY_TYPE_COLORS.unknown;
	return ENTITY_TYPE_COLORS[type.toLowerCase()] ?? ENTITY_TYPE_COLORS.unknown;
}

// =============================================================================
// Predicate Colors (Relationship edge types)
// =============================================================================

/**
 * Color mapping for relationship predicates by category.
 * Predicates use 3-part notation: domain.category.property
 * Color is derived from the category (2nd part).
 */
export const PREDICATE_COLORS: Record<string, string> = {
	// Quest lifecycle
	lifecycle: '#a855f7', // Purple — matches quest node color
	progression: '#22d3ee', // Cyan — matches agent node color
	formation: '#22c55e', // Green — matches party node color
	membership: '#eab308', // Gold — matches guild node color
	review: '#ef4444', // Red — matches battle node color

	// Data / state
	data: '#64748b', // Slate
	state: '#64748b',

	// Semsource / knowledge graph predicates
	content: '#3b82f6', // Blue — source content
	ast: '#14b8a6', // Teal — AST structure
	metadata: '#f59e0b', // Amber — metadata
	identity: '#94a3b8', // Slate-light — identity info
	section: '#f59e0b', // Amber — doc sections
	import: '#8b5cf6', // Violet — imports/dependencies

	// Generic fallback
	default: '#6b7280' // Gray
};

/**
 * Get color for a relationship predicate.
 * Extracts the category (2nd part) from 3-part dotted predicate notation.
 */
export function getPredicateColor(predicate: string): string {
	const parts = predicate.split('.');
	// Use second part (category) if available: "quest.lifecycle.claimed" → "lifecycle"
	const category = parts[1] ?? parts[0] ?? '';
	return PREDICATE_COLORS[category.toLowerCase()] ?? PREDICATE_COLORS.default;
}

// =============================================================================
// Community Colors (Cluster assignment)
// =============================================================================

/**
 * Color palette for community clusters.
 * Communities are assigned colors in order from this palette.
 */
export const COMMUNITY_PALETTE: string[] = [
	'#f87171', // Red
	'#fb923c', // Orange
	'#fbbf24', // Amber
	'#a3e635', // Lime
	'#4ade80', // Green
	'#2dd4bf', // Teal
	'#22d3ee', // Cyan
	'#60a5fa', // Blue
	'#a78bfa', // Violet
	'#f472b6', // Pink
	'#94a3b8' // Slate (fallback)
];

/**
 * Assign a color to a community based on its index.
 */
export function getCommunityColor(index: number): string {
	return COMMUNITY_PALETTE[index % COMMUNITY_PALETTE.length];
}

// =============================================================================
// Confidence Colors (Edge opacity)
// =============================================================================

/**
 * Get opacity value based on confidence score.
 * Maps 0.0–1.0 confidence to 0.3–1.0 opacity so edges are never fully invisible.
 */
export function getConfidenceOpacity(confidence: number): number {
	const clamped = Math.max(0, Math.min(1, confidence));
	return 0.3 + clamped * 0.7;
}

/**
 * Convert a hex color to rgba with the given opacity.
 */
function hexToRgba(hex: string, opacity: number): string {
	const h = hex.replace('#', '');
	let r: number, g: number, b: number;

	if (h.length === 3) {
		r = parseInt(h[0] + h[0], 16);
		g = parseInt(h[1] + h[1], 16);
		b = parseInt(h[2] + h[2], 16);
	} else if (h.length === 6) {
		r = parseInt(h.substring(0, 2), 16);
		g = parseInt(h.substring(2, 4), 16);
		b = parseInt(h.substring(4, 6), 16);
	} else {
		return `rgba(156, 163, 175, ${opacity})`;
	}

	return `rgba(${r}, ${g}, ${b}, ${opacity})`;
}

/**
 * Get a hex color adjusted for a confidence score as an rgba string.
 */
export function getColorWithConfidence(baseColor: string, confidence: number): string {
	const opacity = getConfidenceOpacity(confidence);
	if (baseColor.startsWith('var(')) {
		const match = baseColor.match(/var\([^,]+,\s*([^)]+)\)/);
		if (match) return hexToRgba(match[1], opacity);
		return baseColor;
	}
	return hexToRgba(baseColor, opacity);
}

// =============================================================================
// Primary Entry Point
// =============================================================================

/**
 * Get the primary visualization color for an entity based on its parsed ID parts.
 * Semdragons colors by entity type (5th ID part): quest/agent/party/guild/battle.
 */
export function getEntityColor(idParts: EntityIdParts): string {
	return getEntityTypeColor(idParts.type);
}
