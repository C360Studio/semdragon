/**
 * Shared formatting utilities for the Semdragons UI.
 */

/** Format a token count for compact display (e.g., "125K", "1.2M") */
export function formatTokenCount(n: number): string {
	if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
	if (n >= 1_000) return `${(n / 1_000).toFixed(0)}K`;
	return n.toString();
}

/** Format a USD cost for compact display */
export function formatCostUSD(usd: number): string {
	if (usd >= 1) return `$${usd.toFixed(2)}`;
	if (usd >= 0.01) return `$${usd.toFixed(2)}`;
	if (usd >= 0.001) return `$${usd.toFixed(3)}`;
	if (usd > 0) return '<$0.001';
	return '$0.00';
}
