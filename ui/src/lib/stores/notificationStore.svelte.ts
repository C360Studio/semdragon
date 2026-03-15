/**
 * Notification Store - DM attention system for quest failures, escalations, and triage
 *
 * Watches worldStore for quest status transitions and fires toasts + chat attention cards.
 * Uses Svelte 5 runes for reactive state management.
 *
 * Transition handling:
 *   → escalated:       toast + chat attention card (auto-opens chat)
 *   → pending_triage:  toast + chat attention card (auto-opens chat)
 *   → failed:          toast only (shows retry attempt count)
 */

import type { QuestID, Quest } from '$types';
import type { AttentionCard } from '$lib/stores/chatStore.svelte';
import { browser } from '$app/environment';
import { worldStore } from '$lib/stores/worldStore.svelte';
import { chatStore } from '$lib/stores/chatStore.svelte';

// =============================================================================
// TYPES
// =============================================================================

export interface Toast {
	id: string;
	type: 'escalation' | 'triage' | 'failure';
	questId: QuestID;
	questTitle: string;
	message: string;
	timestamp: number;
}

type AttentionStatus = 'escalated' | 'pending_triage' | 'failed';

const TOAST_DURATION_MS = 8_000;
const MAX_VISIBLE_TOASTS = 5;

// =============================================================================
// REACTIVE STATE
// =============================================================================

let toasts = $state<Toast[]>([]);
// Internal tracking state — NOT reactive to avoid infinite $effect loops.
// These are written inside watchQuests() which runs inside a layout $effect;
// if they were $state, reading+writing them would re-trigger the effect.
let previousStatuses = new Map<QuestID, string>();
let seen = new Set<string>();
let initialized = false;

// =============================================================================
// DERIVED
// =============================================================================

const needsAttentionQuests = $derived(
	worldStore.questList.filter(
		(q) => q.status === 'escalated' || (q.status as string) === 'pending_triage'
	)
);

const needsAttentionCount = $derived(needsAttentionQuests.length);

// =============================================================================
// TOAST MANAGEMENT
// =============================================================================

const toastTimers = new Map<string, ReturnType<typeof setTimeout>>();

function addToast(toast: Toast) {
	toasts = [toast, ...toasts].slice(0, MAX_VISIBLE_TOASTS);

	// Auto-dismiss after duration, tracking handle for cancellation
	if (browser) {
		const handle = setTimeout(() => {
			dismissToast(toast.id);
		}, TOAST_DURATION_MS);
		toastTimers.set(toast.id, handle);
	}
}

function dismissToast(id: string) {
	const handle = toastTimers.get(id);
	if (handle !== undefined) {
		clearTimeout(handle);
		toastTimers.delete(id);
	}
	toasts = toasts.filter((t) => t.id !== id);
}

function clearToasts() {
	for (const handle of toastTimers.values()) {
		clearTimeout(handle);
	}
	toastTimers.clear();
	toasts = [];
}

// =============================================================================
// TRANSITION DETECTION
// =============================================================================

function handleTransition(quest: Quest, newStatus: AttentionStatus) {
	const key = `${quest.id}-${newStatus}`;
	if (seen.has(key)) return;
	seen.add(key);

	const title = quest.title ?? String(quest.id).split('.').pop() ?? 'Unknown quest';
	const agentName = quest.claimed_by ? worldStore.agentName(quest.claimed_by) : undefined;

	if (newStatus === 'escalated') {
		const failureContext = quest.failure_reason ?? quest.failure_analysis ?? 'Quest exceeded retry limit';
		const toast: Toast = {
			id: crypto.randomUUID(),
			type: 'escalation',
			questId: quest.id,
			questTitle: title,
			message: failureContext,
			timestamp: Date.now()
		};
		addToast(toast);

		const card: AttentionCard = {
			type: 'escalation',
			questId: quest.id,
			questTitle: title,
			agentName,
			failureReason: quest.failure_reason ?? undefined,
			failureAnalysis: quest.failure_analysis ?? undefined,
			failureType: quest.failure_type ?? undefined,
			attempts: quest.attempts,
			maxAttempts: quest.max_attempts,
			resolved: false
		};
		chatStore.injectAttentionCard(card);
	} else if (newStatus === 'pending_triage') {
		const toast: Toast = {
			id: crypto.randomUUID(),
			type: 'triage',
			questId: quest.id,
			questTitle: title,
			message: quest.failure_reason ?? 'Retries exhausted — needs triage',
			timestamp: Date.now()
		};
		addToast(toast);

		const card: AttentionCard = {
			type: 'triage',
			questId: quest.id,
			questTitle: title,
			agentName,
			failureReason: quest.failure_reason ?? undefined,
			failureType: quest.failure_type ?? undefined,
			attempts: quest.attempts,
			maxAttempts: quest.max_attempts,
			resolved: false
		};
		chatStore.injectAttentionCard(card);
	} else if (newStatus === 'failed') {
		const attempt = quest.attempts && quest.max_attempts
			? `Attempt ${quest.attempts}/${quest.max_attempts}`
			: 'Failed';
		const toast: Toast = {
			id: crypto.randomUUID(),
			type: 'failure',
			questId: quest.id,
			questTitle: title,
			message: `${attempt}${quest.failure_reason ? ` — ${quest.failure_reason}` : ''}`,
			timestamp: Date.now()
		};
		addToast(toast);
		// No chat card for plain failures
	}
}

// =============================================================================
// WATCHER — call from layout $effect
// =============================================================================

/**
 * Scan quest list for status transitions. Call this from a Svelte $effect
 * that depends on worldStore.questList.
 */
function watchQuests(questList: Quest[], synced: boolean) {
	// Skip during initial SSE sync to avoid flooding
	if (!synced) return;

	// First call after sync: snapshot current statuses without firing
	if (!initialized) {
		const snapshot = new Map<QuestID, string>();
		for (const q of questList) {
			snapshot.set(q.id, q.status);
		}
		previousStatuses = snapshot;
		initialized = true;
		return;
	}

	const nextStatuses = new Map<QuestID, string>();

	for (const quest of questList) {
		nextStatuses.set(quest.id, quest.status);
		const prev = previousStatuses.get(quest.id);

		// Only fire on transitions TO attention statuses
		if (prev === quest.status) continue;

		const status = quest.status as string;
		if (status === 'escalated' || status === 'pending_triage' || status === 'failed') {
			handleTransition(quest, status as AttentionStatus);
		}
	}

	previousStatuses = nextStatuses;
}

// Auto-resolve attention cards when quest leaves escalated/pending_triage
function reconcileAttentionCards(questList: Quest[]) {
	// Only check quests that have unresolved attention cards
	const unresolvedQuestIds = new Set(
		chatStore.messages
			.filter((m) => m.attentionCard && !m.attentionCard.resolved)
			.map((m) => m.attentionCard!.questId)
	);
	if (unresolvedQuestIds.size === 0) return;

	for (const quest of questList) {
		if (!unresolvedQuestIds.has(quest.id)) continue;
		const status = quest.status as string;
		if (status !== 'escalated' && status !== 'pending_triage') {
			chatStore.resolveAttentionCard(quest.id);
		}
	}
}

// =============================================================================
// EXPORT
// =============================================================================

export const notificationStore = {
	get toasts() { return toasts; },
	get needsAttentionQuests() { return needsAttentionQuests; },
	get needsAttentionCount() { return needsAttentionCount; },

	dismissToast,
	clearToasts,
	watchQuests,
	reconcileAttentionCards
};
