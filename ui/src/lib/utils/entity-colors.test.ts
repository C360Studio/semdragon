/**
 * Tests for entity-colors utility.
 *
 * Covers getEntityColor, getPredicateColor, getCommunityColor,
 * getColorWithConfidence, and the underlying palette constants.
 */

import { describe, it, expect } from 'vitest';
import {
	getEntityColor,
	getPredicateColor,
	getCommunityColor,
	getColorWithConfidence,
	ENTITY_TYPE_COLORS,
	PREDICATE_COLORS,
	COMMUNITY_PALETTE
} from './entity-colors';
import type { EntityIdParts } from '$lib/api/graph-types';

// =============================================================================
// Test helpers
// =============================================================================

function idParts(type: string): EntityIdParts {
	return {
		org: 'c360',
		platform: 'prod',
		domain: 'game',
		system: 'board1',
		type,
		instance: 'test-instance'
	};
}

// =============================================================================
// getEntityColor
// =============================================================================

describe('getEntityColor', () => {
	it('returns the correct hex color for each known game entity type', () => {
		expect(getEntityColor(idParts('quest'))).toBe(ENTITY_TYPE_COLORS.quest);
		expect(getEntityColor(idParts('agent'))).toBe(ENTITY_TYPE_COLORS.agent);
		expect(getEntityColor(idParts('party'))).toBe(ENTITY_TYPE_COLORS.party);
		expect(getEntityColor(idParts('guild'))).toBe(ENTITY_TYPE_COLORS.guild);
		expect(getEntityColor(idParts('battle'))).toBe(ENTITY_TYPE_COLORS.battle);
		expect(getEntityColor(idParts('peer_review'))).toBe(ENTITY_TYPE_COLORS.peer_review);
	});

	it('returns the specific hex values for quest and agent', () => {
		// Quest is purple, agent is cyan per source comments
		expect(getEntityColor(idParts('quest'))).toBe('#a855f7');
		expect(getEntityColor(idParts('agent'))).toBe('#22d3ee');
	});

	it('returns gray (#6b7280) for an unknown entity type', () => {
		expect(getEntityColor(idParts('npc'))).toBe('#6b7280');
		expect(getEntityColor(idParts('unknown'))).toBe('#6b7280');
	});

	it('returns gray for an empty type string', () => {
		expect(getEntityColor(idParts(''))).toBe('#6b7280');
	});
});

// =============================================================================
// getPredicateColor
// =============================================================================

describe('getPredicateColor', () => {
	it('returns the correct color for known predicate categories', () => {
		// category is the 2nd part of 3-part predicate notation
		expect(getPredicateColor('quest.lifecycle.claimed')).toBe(PREDICATE_COLORS.lifecycle);
		expect(getPredicateColor('agent.progression.xp')).toBe(PREDICATE_COLORS.progression);
		expect(getPredicateColor('party.formation.created')).toBe(PREDICATE_COLORS.formation);
		expect(getPredicateColor('guild.membership.joined')).toBe(PREDICATE_COLORS.membership);
		expect(getPredicateColor('battle.review.verdict')).toBe(PREDICATE_COLORS.review);
	});

	it('returns slate color for data and state categories', () => {
		expect(getPredicateColor('quest.data.output')).toBe(PREDICATE_COLORS.data);
		expect(getPredicateColor('agent.state.active')).toBe(PREDICATE_COLORS.state);
	});

	it('returns default gray for an unrecognised predicate category', () => {
		expect(getPredicateColor('foo.bar.baz')).toBe(PREDICATE_COLORS.default);
	});

	it('handles a 1-part predicate (no dots) by using the whole string as category', () => {
		// parts[1] is undefined, falls back to parts[0]
		const result = getPredicateColor('lifecycle');
		expect(result).toBe(PREDICATE_COLORS.lifecycle);
	});

	it('handles a 2-part predicate by using the 2nd part as category', () => {
		const result = getPredicateColor('quest.lifecycle');
		expect(result).toBe(PREDICATE_COLORS.lifecycle);
	});

	it('returns default gray for an empty predicate string', () => {
		// empty string → parts[1] undefined, parts[0] = '', no match
		expect(getPredicateColor('')).toBe(PREDICATE_COLORS.default);
	});
});

// =============================================================================
// getCommunityColor
// =============================================================================

describe('getCommunityColor', () => {
	it('returns the first palette color for index 0', () => {
		expect(getCommunityColor(0)).toBe(COMMUNITY_PALETTE[0]);
	});

	it('returns the correct color for each index within the palette length', () => {
		for (let i = 0; i < COMMUNITY_PALETTE.length; i++) {
			expect(getCommunityColor(i)).toBe(COMMUNITY_PALETTE[i]);
		}
	});

	it('cycles back to the start when the index exceeds the palette length', () => {
		const len = COMMUNITY_PALETTE.length;
		expect(getCommunityColor(len)).toBe(COMMUNITY_PALETTE[0]);
		expect(getCommunityColor(len + 1)).toBe(COMMUNITY_PALETTE[1]);
	});

	it('handles large index values via modulo cycling', () => {
		const len = COMMUNITY_PALETTE.length;
		const largeIndex = len * 100 + 3;
		expect(getCommunityColor(largeIndex)).toBe(COMMUNITY_PALETTE[3]);
	});
});

// =============================================================================
// getColorWithConfidence
// =============================================================================

describe('getColorWithConfidence', () => {
	it('returns an rgba string for a 6-digit hex color', () => {
		const result = getColorWithConfidence('#a855f7', 1.0);
		expect(result).toMatch(/^rgba\(\d+,\s*\d+,\s*\d+,\s*[\d.]+\)$/);
	});

	it('opacity is 1.0 (i.e. 0.3 + 1.0*0.7 = 1.0) when confidence is 1.0', () => {
		const result = getColorWithConfidence('#ffffff', 1.0);
		// opacity = 0.3 + 1.0 * 0.7 = 1.0
		expect(result).toBe('rgba(255, 255, 255, 1)');
	});

	it('opacity is 0.3 (i.e. 0.3 + 0.0*0.7) when confidence is 0.0', () => {
		const result = getColorWithConfidence('#ffffff', 0.0);
		expect(result).toBe('rgba(255, 255, 255, 0.3)');
	});

	it('clamps confidence above 1.0 to 1.0', () => {
		const resultClamped = getColorWithConfidence('#ffffff', 1.0);
		const resultOver = getColorWithConfidence('#ffffff', 2.0);
		expect(resultClamped).toBe(resultOver);
	});

	it('clamps confidence below 0.0 to 0.0', () => {
		const resultClamped = getColorWithConfidence('#ffffff', 0.0);
		const resultUnder = getColorWithConfidence('#ffffff', -1.0);
		expect(resultClamped).toBe(resultUnder);
	});

	it('correctly converts a 6-digit hex color to its rgb components', () => {
		// #a855f7 → r=168, g=85, b=247
		const result = getColorWithConfidence('#a855f7', 1.0);
		expect(result).toBe('rgba(168, 85, 247, 1)');
	});

	it('handles a 3-digit shorthand hex color', () => {
		// #fff → expanded to #ffffff → rgba(255, 255, 255, ...)
		const result = getColorWithConfidence('#fff', 1.0);
		expect(result).toBe('rgba(255, 255, 255, 1)');
	});

	it('returns a fallback rgba for an invalid hex string', () => {
		// Non-3 and non-6 length after stripping # triggers the fallback path
		const result = getColorWithConfidence('#gg', 0.5);
		// Fallback uses hardcoded gray (156, 163, 175)
		expect(result).toMatch(/^rgba\(156, 163, 175,/);
	});

	it('handles a CSS var() color with fallback hex', () => {
		// var(--color-quest, #a855f7) → extracts #a855f7 from fallback
		const result = getColorWithConfidence('var(--color-quest, #a855f7)', 1.0);
		expect(result).toBe('rgba(168, 85, 247, 1)');
	});

	it('returns the original var() string when no fallback hex is present', () => {
		const result = getColorWithConfidence('var(--color-quest)', 1.0);
		expect(result).toBe('var(--color-quest)');
	});
});
