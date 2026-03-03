/**
 * Page Context Store - Reactive store for current page entity context
 *
 * Pages set their entity context (e.g., which quest/agent is being viewed)
 * and ChatPanel displays these as context chips. Setting page context also
 * pins items to the chat context (without auto-opening it) so the DM
 * knows what the user is looking at.
 */

import { chatStore } from '$stores/chatStore.svelte';

export interface PageContextItem {
	type: 'agent' | 'quest' | 'battle' | 'guild' | 'party';
	id: string;
	label: string;
}

let items = $state<PageContextItem[]>([]);

function set(newItems: PageContextItem[]) {
	items = newItems;
	// Auto-pin to chat context so DM has awareness of what user is viewing
	for (const item of newItems) {
		chatStore.addContextQuiet(item);
	}
}

function clear() {
	items = [];
}

export const pageContext = {
	get items() {
		return items;
	},
	set,
	clear
};
