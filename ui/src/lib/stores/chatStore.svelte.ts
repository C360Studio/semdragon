/**
 * Chat Store - Global state for DM chat panel
 *
 * Manages conversation history, context injection, and quest brief extraction.
 * Uses Svelte 5 runes for reactive state management.
 * Persists to localStorage for instant UI restore on page refresh.
 *
 * Slash commands:
 *   /quest <text>  — sends message in quest mode (structured quest output)
 *   /help          — shows client-side help (no API call)
 *   (no prefix)    — sends in converse mode (natural language Q&A)
 */

import type {
	Quest, QuestID, QuestDifficulty, SkillTag, DMChatSession, ChatMode,
	QuestBrief, QuestChainBrief, QuestChainEntry, QuestHints
} from '$types';
import { browser } from '$app/environment';
import { sendDMChat, postQuestChain, getDMSession, repostEscalation, triageQuest, ApiError } from '$lib/services/api';
import { worldStore } from '$lib/stores/worldStore.svelte';

export type { ChatMode, QuestBrief, QuestChainBrief, QuestChainEntry };
export type QuestHintsBrief = QuestHints;
import { pageContext } from '$lib/stores/pageContext.svelte';

// QuestScenario — matches the generated schema from domain.QuestScenario.
export interface QuestScenario {
	name: string;
	description: string;
	skills?: string[];
	depends_on?: string[];
}

// =============================================================================
// TYPES
// =============================================================================

export interface AttentionCard {
	type: 'escalation' | 'triage';
	questId: QuestID;
	questTitle: string;
	agentName?: string;
	question?: string;
	failureReason?: string;
	failureAnalysis?: string;
	failureType?: string;
	attempts?: number;
	maxAttempts?: number;
	resolved: boolean;
	resolvedAnswer?: string;
	resolvedPath?: string;
}

export interface ChatMessage {
	role: 'user' | 'dm' | 'system';
	content: string;
	questBrief?: QuestBrief | null;
	questChain?: QuestChainBrief | null;
	attentionCard?: AttentionCard | null;
	timestamp: number;
}

export interface ChatContextItem {
	type: 'agent' | 'quest' | 'battle' | 'guild' | 'party';
	id: string;
	label: string;
}

// =============================================================================
// HELP TEXT
// =============================================================================

const HELP_TEXT = `**Available commands:**

**/quest <description>** — Describe work and the DM will draft a quest or quest chain.
Example: \`/quest Write a hello world function in Go\`

**/help** — Show this help message.

**No prefix** — Chat with the DM about the game world, agents, quests, or strategies.

**Tips:**
- Pin entities (agents, quests) as context chips for targeted questions.
- Quest previews include a "Post Quest" button to add them to the board.
- Agents autonomously claim quests based on their skills and trust tier.`;

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
let open = $state(true);
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

	// Parse slash command prefix
	let effectiveMode: ChatMode = 'converse';
	let messageText = text.trim();

	if (messageText.startsWith('/quest ')) {
		effectiveMode = 'quest';
		messageText = messageText.slice(7).trim();
		if (!messageText) return; // nothing after /quest
	} else if (messageText.startsWith('/help')) {
		// Client-side help — inject DM-style help message, don't call API
		messages = [
			...messages,
			{ role: 'user', content: text.trim(), timestamp: Date.now() },
			{ role: 'dm', content: HELP_TEXT, timestamp: Date.now() }
		];
		saveToLocalStorage();
		return;
	}

	const userMsg: ChatMessage = {
		role: 'user',
		content: text.trim(),
		timestamp: Date.now()
	};
	messages = [...messages, userMsg];
	loading = true;
	error = null;

	try {
		// Build history from prior messages (exclude system messages and quest brief/chain metadata)
		const history = messages.slice(0, -1)
			.filter((m) => m.role !== 'system')
			.map((m) => ({
				role: m.role as 'user' | 'dm',
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

		const response = await sendDMChat(messageText, effectiveMode, context, history, sessionId ?? undefined);

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
		// If the server doesn't recognize our session, clear stale state
		// so the next message starts fresh instead of referencing old data.
		if (e instanceof ApiError && (e.status === 404 || e.status === 503)) {
			sessionId = null;
		}
		// Remove the user message on failure so they can retry
		messages = messages.slice(0, -1);
	} finally {
		loading = false;
	}
}

async function postQuest(brief: QuestBrief): Promise<Quest | null> {
	// Route through the chain endpoint — it handles all structured spec fields
	// (goal, requirements, scenarios, hints). A single brief is a 1-entry chain.
	const chain: QuestChainBrief = {
		quests: [{
			title: brief.title,
			goal: brief.goal,
			requirements: brief.requirements,
			scenarios: brief.scenarios as QuestScenario[],
			difficulty: brief.difficulty,
			skills: brief.skills,
			hints: brief.hints,
		}]
	};
	const quests = await postChain(chain);
	return quests?.[0] ?? null;
}

async function postChain(chain: QuestChainBrief): Promise<Quest[] | null> {
	try {
		const quests = await postQuestChain(chain);
		// Optimistic update — show quests immediately instead of waiting for SSE
		for (const quest of quests) {
			worldStore.upsertQuest(quest);
		}
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

function dismissQuestBrief(msgIndex: number) {
	if (msgIndex >= 0 && msgIndex < messages.length) {
		messages[msgIndex] = { ...messages[msgIndex], questBrief: null };
		messages = [...messages];
		saveToLocalStorage();
	}
}

function dismissQuestChain(msgIndex: number) {
	if (msgIndex >= 0 && msgIndex < messages.length) {
		messages[msgIndex] = { ...messages[msgIndex], questChain: null };
		messages = [...messages];
		saveToLocalStorage();
	}
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
// ATTENTION CARDS
// =============================================================================

function buildAttentionMessage(card: AttentionCard): string {
	if (card.type === 'escalation') {
		const agent = card.agentName ? ` (${card.agentName})` : '';
		const context = card.failureReason ?? card.failureAnalysis ?? 'Quest exceeded retry limit.';
		return `**Quest escalated**: ${card.questTitle}${agent}\n\n${context}`;
	}
	const attempt = card.attempts && card.maxAttempts ? ` (attempt ${card.attempts}/${card.maxAttempts})` : '';
	return `**Quest needs triage**: ${card.questTitle}${attempt}\n\n${card.failureReason ?? 'Retries exhausted.'}`;
}

function injectAttentionCard(card: AttentionCard) {
	// Dedup — don't inject if there's already an unresolved card for this quest+type
	const existing = messages.find(
		(m) => m.attentionCard && m.attentionCard.questId === card.questId && m.attentionCard.type === card.type && !m.attentionCard.resolved
	);
	if (existing) return;

	const systemMsg: ChatMessage = {
		role: 'system',
		content: buildAttentionMessage(card),
		attentionCard: card,
		timestamp: Date.now()
	};
	messages = [...messages, systemMsg];
	if (!open) open = true;
	saveToLocalStorage();
}

function resolveAttentionCard(questId: QuestID) {
	let changed = false;
	for (let i = 0; i < messages.length; i++) {
		const card = messages[i].attentionCard;
		if (card && card.questId === questId && !card.resolved) {
			messages[i] = { ...messages[i], attentionCard: { ...card, resolved: true } };
			changed = true;
		}
	}
	if (changed) {
		messages = [...messages];
		saveToLocalStorage();
	}
}

async function respondToEscalation(questId: QuestID, answer: string): Promise<boolean> {
	try {
		loading = true;
		error = null;
		// Empty answer = plain repost; non-empty = repost with DM guidance
		await repostEscalation(questId, answer || undefined);
		// Mark card resolved and record the action
		const resolvedLabel = answer ? `Reposted with guidance: ${answer}` : 'Reposted';
		for (let i = 0; i < messages.length; i++) {
			const card = messages[i].attentionCard;
			if (card && card.questId === questId && card.type === 'escalation' && !card.resolved) {
				messages[i] = { ...messages[i], attentionCard: { ...card, resolved: true, resolvedAnswer: resolvedLabel } };
			}
		}
		messages = [...messages];
		saveToLocalStorage();
		return true;
	} catch (e) {
		error = e instanceof Error ? e.message : 'Failed to repost quest';
		return false;
	} finally {
		loading = false;
	}
}

async function submitTriage(
	questId: QuestID,
	decision: { path: 'salvage' | 'tpk' | 'escalate' | 'terminal'; analysis: string; salvaged_output?: unknown; anti_patterns?: string[] }
): Promise<boolean> {
	try {
		loading = true;
		error = null;
		await triageQuest(questId, decision);
		// Mark card resolved and record the path
		for (let i = 0; i < messages.length; i++) {
			const card = messages[i].attentionCard;
			if (card && card.questId === questId && card.type === 'triage' && !card.resolved) {
				messages[i] = { ...messages[i], attentionCard: { ...card, resolved: true, resolvedPath: decision.path } };
			}
		}
		messages = [...messages];
		saveToLocalStorage();
		return true;
	} catch (e) {
		error = e instanceof Error ? e.message : 'Failed to submit triage decision';
		return false;
	} finally {
		loading = false;
	}
}

/** Count of unresolved attention cards in the chat. */
const unresolvedAttentionCount = $derived(
	messages.filter((m) => m.attentionCard && !m.attentionCard.resolved).length
);

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
	dismissQuestBrief,
	dismissQuestChain,
	restoreFromServer,
	injectAttentionCard,
	resolveAttentionCard,
	respondToEscalation,
	submitTriage,
	get unresolvedAttentionCount() { return unresolvedAttentionCount; }
};
