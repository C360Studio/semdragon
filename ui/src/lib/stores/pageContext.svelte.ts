/**
 * Page Context Store - Reactive store for current page entity context
 *
 * Pages set their entity context (e.g., which quest/agent is being viewed)
 * and ChatPanel displays these as context chips. Items are automatically
 * cleared when navigating away via $effect cleanup.
 */

export interface PageContextItem {
	type: 'agent' | 'quest' | 'battle' | 'guild' | 'party';
	id: string;
	label: string;
}

let items = $state<PageContextItem[]>([]);

function set(newItems: PageContextItem[]) {
	items = newItems;
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
