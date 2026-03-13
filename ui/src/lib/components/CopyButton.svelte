<script lang="ts">
	/**
	 * CopyButton - Click-to-copy icon button for text content.
	 *
	 * Shows a clipboard icon that becomes a checkmark briefly after copying.
	 * Can be used inline next to IDs or positioned over pre/code blocks.
	 */

	interface CopyButtonProps {
		/** The text to copy to clipboard */
		text: string;
		/** Optional label for accessibility */
		label?: string;
		/** Visual variant: 'inline' for next to text, 'overlay' for positioned over blocks */
		variant?: 'inline' | 'overlay';
	}

	let { text, label = 'Copy to clipboard', variant = 'overlay' }: CopyButtonProps = $props();

	let copied = $state(false);
	let timeout: ReturnType<typeof setTimeout> | undefined;

	function handleCopy(e: MouseEvent) {
		e.stopPropagation();
		navigator.clipboard.writeText(text).then(() => {
			copied = true;
			clearTimeout(timeout);
			timeout = setTimeout(() => (copied = false), 1500);
		});
	}
</script>

<button
	class="copy-btn"
	class:copied
	class:inline={variant === 'inline'}
	class:overlay={variant === 'overlay'}
	onclick={handleCopy}
	title={copied ? 'Copied!' : label}
	aria-label={label}
>
	{#if copied}
		<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
			<polyline points="20 6 9 17 4 12" />
		</svg>
	{:else}
		<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
			<rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
			<path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
		</svg>
	{/if}
</button>

<style>
	.copy-btn {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		background: none;
		border: none;
		padding: 4px;
		border-radius: var(--radius-sm, 4px);
		color: var(--ui-text-tertiary);
		cursor: pointer;
		transition: color 150ms ease, background 150ms ease;
		flex-shrink: 0;
	}

	.copy-btn:hover {
		color: var(--ui-text-primary);
		background: var(--ui-surface-tertiary);
	}

	.copy-btn.copied {
		color: var(--status-success, #4caf50);
	}

	/* Overlay variant: absolute positioned in top-right of a relative parent.
	   Parent must have position:relative and the .copyable class for hover reveal. */
	.copy-btn.overlay {
		position: absolute;
		top: 4px;
		right: 4px;
		opacity: 0;
		background: var(--ui-surface-secondary);
		border: 1px solid var(--ui-border-subtle);
	}

	.copy-btn.overlay.copied {
		opacity: 1;
	}

	:global(.copyable:hover) .copy-btn.overlay {
		opacity: 1;
	}

	/* Inline variant: sits next to text */
	.copy-btn.inline {
		opacity: 0.5;
	}

	.copy-btn.inline:hover,
	.copy-btn.inline.copied {
		opacity: 1;
	}
</style>
