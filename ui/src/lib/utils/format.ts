/**
 * Shared formatting utilities for the Semdragons UI.
 */

/** Format a token count for compact display (e.g., "125K", "1.2M") */
export function formatTokenCount(n: number): string {
	if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
	if (n >= 1_000) return `${(n / 1_000).toFixed(0)}K`;
	return n.toString();
}

/**
 * Format a date string for compact display.
 * Shows relative time for recent dates ("just now", "5m ago", "2h ago")
 * and falls back to locale date string for older ones.
 */
export function formatDate(dateStr: string | null | undefined): string {
	if (!dateStr) return '—';
	const d = new Date(dateStr);
	if (isNaN(d.getTime())) return '—';
	const now = Date.now();
	const diff = now - d.getTime();
	if (diff < 60000) return 'just now';
	if (diff < 3600000) return `${Math.floor(diff / 60000)}m ago`;
	if (diff < 86400000) return `${Math.floor(diff / 3600000)}h ago`;
	return d.toLocaleDateString();
}

/** Format a USD cost for compact display */
export function formatCostUSD(usd: number): string {
	if (usd >= 1) return `$${usd.toFixed(2)}`;
	if (usd >= 0.01) return `$${usd.toFixed(2)}`;
	if (usd >= 0.001) return `$${usd.toFixed(3)}`;
	if (usd > 0) return '<$0.001';
	return '$0.00';
}
