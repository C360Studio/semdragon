/**
 * Chat Store - Global state for DM chat panel
 *
 * Manages conversation history, context injection, and quest brief extraction.
 * Uses Svelte 5 runes for reactive state management.
 * Persists to localStorage for instant UI restore on page refresh.
 */

import type { Quest, QuestDifficulty, SkillTag, DMChatSession } from '$types';
import { browser } from '$app/environment';
import { sendDMChat, createQuest, postQuestChain, getDMSession, ApiError } from '$lib/services/api';
import { pageContext } from '$lib/stores/pageContext.svelte';

// =============================================================================
// TYPES
// =============================================================================

export interface ChatMessage {
	role: 'user' | 'dm';
	content: string;
	questBrief?: QuestBrief | null;
	questChain?: QuestChainBrief | null;
	timestamp: number;
}

export interface ChatContextItem {
	type: 'agent' | 'quest' | 'battle' | 'guild' | 'party';
	id: string;
	label: string;
}

export interface QuestBrief {
	title: string;
	description?: string;
	difficulty?: QuestDifficulty;
	skills?: SkillTag[];
	acceptance?: string[];
}

export interface QuestChainBrief {
	quests: QuestChainEntry[];
}

export interface QuestChainEntry {
	title: string;
	description?: string;
	difficulty?: QuestDifficulty;
	skills?: SkillTag[];
	acceptance?: string[];
	depends_on?: number[];
}

// =============================================================================
// LOCALSTORAGE PERSISTENCE
// =============================================================================

const STORAGE_KEY = 'semdragons-dm-chat';
const MAX_STORAGE_AGE_MS = 24 * 60 * 60 * 1000; // 24h

interface PersistedChatState {
	messages: ChatMessage[];
	sessionId: string | null;
	savedAt: number;
}

function saveToLocalStorage() {
	try {
		const state: PersistedChatState = {
			messages,
			sessionId,
			savedAt: Date.now()
		};
		localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
	} catch {
		// localStorage full or unavailable — silently ignore
	}
}

function loadFromLocalStorage(): PersistedChatState | null {
	try {
		const raw = localStorage.getItem(STORAGE_KEY);
		if (!raw) return null;

		const parsed = JSON.parse(raw);

		// Shape validation
		if (!parsed || !Array.isArray(parsed.messages) || typeof parsed.savedAt !== 'number') {
			return null;
		}

		if (typeof parsed.sessionId !== 'string' && parsed.sessionId !== null) {
			return null;
		}

		const validMessages = parsed.messages.every(
			(m: unknown) =>
				typeof m === 'object' &&
				m !== null &&
				typeof (m as Record<string, unknown>).role === 'string' &&
				typeof (m as Record<string, unknown>).content === 'string'
		);
		if (!validMessages) return null;

		// Staleness check
		if (Date.now() - parsed.savedAt > MAX_STORAGE_AGE_MS) {
			localStorage.removeItem(STORAGE_KEY);
			return null;
		}

		return parsed as PersistedChatState;
	} catch {
		return null;
	}
}

function clearLocalStorage() {
	try {
		localStorage.removeItem(STORAGE_KEY);
	} catch {
		// silently ignore
	}
}

// =============================================================================
// REACTIVE STATE
// =============================================================================

let messages = $state<ChatMessage[]>([]);
let contextItems = $state<ChatContextItem[]>([]);
let open = $state(false);
let height = $state(250);
let loading = $state(false);
let error = $state<string | null>(null);
let sessionId = $state<string | null>(null);

// Restore from localStorage on module load — browser guard prevents SSR errors
if (browser) {
	const restored = loadFromLocalStorage();
	if (restored) {
		messages = restored.messages;
		sessionId = restored.sessionId;
		if (messages.length > 0) open = true;
	}
}

// =============================================================================
// ACTIONS
// =============================================================================

function addContext(item: ChatContextItem) {
	// Don't add duplicates
	if (!contextItems.some((c) => c.type === item.type && c.id === item.id)) {
		contextItems = [...contextItems, item];
	}
	// Always open chat when explicitly pinning context
	if (!open) open = true;
}

function addContextQuiet(item: ChatContextItem) {
	// Add without auto-opening chat — used by page navigation
	if (contextItems.some((c) => c.type === item.type && c.id === item.id)) return;
	contextItems = [...contextItems, item];
}

function removeContext(id: string) {
	contextItems = contextItems.filter((c) => c.id !== id);
}

function clearContext() {
	contextItems = [];
}

function toggle() {
	open = !open;
}

function setHeight(h: number) {
	height = Math.max(150, Math.min(h, window.innerHeight * 0.6));
}

async function sendMessage(text: string) {
	if (!text.trim() || loading) return;

	const userMsg: ChatMessage = {
		role: 'user',
		content: text.trim(),
		timestamp: Date.now()
	};
	messages = [...messages, userMsg];
	loading = true;
	error = null;

	try {
		// Build history from prior messages (exclude quest brief/chain metadata)
		const history = messages.slice(0, -1).map((m) => ({
			role: m.role,
			content: m.content
		}));

		// Merge pinned context + page context, dedup by type+id
		const seen = new Set<string>();
		const context: { type: string; id: string }[] = [];
		for (const c of contextItems) {
			const key = `${c.type}:${c.id}`;
			if (!seen.has(key)) {
				seen.add(key);
				context.push({ type: c.type, id: c.id });
			}
		}
		for (const c of pageContext.items) {
			const key = `${c.type}:${c.id}`;
			if (!seen.has(key)) {
				seen.add(key);
				context.push({ type: c.type, id: c.id });
			}
		}

		const response = await sendDMChat(text.trim(), context, history, sessionId ?? undefined);

		// Track session for trace continuity across turns
		if (response.session_id) {
			sessionId = response.session_id;
		}

		const dmMsg: ChatMessage = {
			role: 'dm',
			content: response.message,
			questBrief: (response.quest_brief as QuestBrief) ?? null,
			questChain: (response.quest_chain as QuestChainBrief) ?? null,
			timestamp: Date.now()
		};
		messages = [...messages, dmMsg];

		// Persist after successful DM response
		saveToLocalStorage();
	} catch (e) {
		error = e instanceof Error ? e.message : 'Failed to send message';
		// Remove the user message on failure so they can retry
		messages = messages.slice(0, -1);
	} finally {
		loading = false;
	}
}

async function postQuest(brief: QuestBrief): Promise<Quest | null> {
	try {
		const quest = await createQuest(brief.title, {
			suggested_difficulty: brief.difficulty,
			suggested_skills: brief.skills,
			require_human_review: false,
			budget: 0
		});
		return quest;
	} catch (e) {
		error = e instanceof Error ? e.message : 'Failed to post quest';
		return null;
	}
}

async function postChain(chain: QuestChainBrief): Promise<Quest[] | null> {
	try {
		const quests = await postQuestChain(chain);
		return quests;
	} catch (e) {
		error = e instanceof Error ? e.message : 'Failed to post quest chain';
		return null;
	}
}

function clearMessages() {
	messages = [];
	error = null;
	sessionId = null;
	clearLocalStorage();
}

/**
 * Restore chat from server-side KV session.
 * Reconstructs ChatMessage[] from DMChatTurns.
 * Returns true if restoration succeeded.
 */
async function restoreFromServer(serverSessionId: string): Promise<boolean> {
	try {
		const session: DMChatSession | null = await getDMSession(serverSessionId);
		if (!session || session.turns.length === 0) return false;

		const restored: ChatMessage[] = [];
		for (const turn of session.turns) {
			const ts = new Date(turn.timestamp).getTime();
			restored.push({
				role: 'user',
				content: turn.user_message,
				timestamp: ts - 1
			});
			restored.push({
				role: 'dm',
				content: turn.dm_response,
				// quest_brief and quest_chain cannot be restored from server turns;
				// the backend does not persist parsed quest data in DMChatTurn.
				timestamp: ts
			});
		}

		messages = restored;
		sessionId = serverSessionId;
		if (messages.length > 0) open = true;
		saveToLocalStorage();
		return true;
	} catch (e) {
		if (e instanceof ApiError && e.status === 503) {
			console.warn('DM session store unavailable, skipping restore');
		}
		return false;
	}
}

// =============================================================================
// EXPORT
// =============================================================================

export const chatStore = {
	get messages() { return messages; },
	get contextItems() { return contextItems; },
	get open() { return open; },
	set open(v: boolean) { open = v; },
	get height() { return height; },
	get loading() { return loading; },
	get error() { return error; },
	get sessionId() { return sessionId; },

	addContext,
	addContextQuiet,
	removeContext,
	clearContext,
	toggle,
	setHeight,
	sendMessage,
	postQuest,
	postChain,
	clearMessages,
	restoreFromServer
};
