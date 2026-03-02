<script lang="ts">
	/**
	 * VerticalResizeHandle - Horizontal bar for vertical panel resizing
	 *
	 * Same pattern as ResizeHandle.svelte but for Y-axis dragging.
	 * Used to resize the chat panel height.
	 */

	interface VerticalResizeHandleProps {
		onResize?: (delta: number) => void;
		onResizeEnd?: () => void;
		disabled?: boolean;
	}

	let { onResize, onResizeEnd, disabled = false }: VerticalResizeHandleProps = $props();

	let isDragging = $state(false);
	let startY = $state(0);

	function handleMouseDown(event: MouseEvent) {
		if (disabled) return;
		event.preventDefault();
		isDragging = true;
		startY = event.clientY;

		document.addEventListener('mousemove', handleMouseMove);
		document.addEventListener('mouseup', handleMouseUp);
	}

	function handleMouseMove(event: MouseEvent) {
		if (!isDragging) return;
		const delta = startY - event.clientY;
		startY = event.clientY;
		onResize?.(delta);
	}

	function handleMouseUp() {
		isDragging = false;
		document.removeEventListener('mousemove', handleMouseMove);
		document.removeEventListener('mouseup', handleMouseUp);
		onResizeEnd?.();
	}

	function handleTouchStart(event: TouchEvent) {
		if (disabled) return;
		event.preventDefault();
		isDragging = true;
		startY = event.touches[0].clientY;

		document.addEventListener('touchmove', handleTouchMove, { passive: false });
		document.addEventListener('touchend', handleTouchEnd);
	}

	function handleTouchMove(event: TouchEvent) {
		if (!isDragging) return;
		event.preventDefault();
		const delta = startY - event.touches[0].clientY;
		startY = event.touches[0].clientY;
		onResize?.(delta);
	}

	function handleTouchEnd() {
		isDragging = false;
		document.removeEventListener('touchmove', handleTouchMove);
		document.removeEventListener('touchend', handleTouchEnd);
		onResizeEnd?.();
	}

	function handleKeyDown(event: KeyboardEvent) {
		if (disabled) return;

		if (event.key === 'Escape') {
			(event.currentTarget as HTMLElement)?.blur();
			return;
		}

		const step = event.shiftKey ? 50 : 10;
		let delta = 0;

		switch (event.key) {
			case 'ArrowUp':
				delta = step;
				break;
			case 'ArrowDown':
				delta = -step;
				break;
			default:
				return;
		}

		event.preventDefault();
		onResize?.(delta);
		onResizeEnd?.();
	}
</script>

<button
	type="button"
	class="vertical-resize-handle"
	class:dragging={isDragging}
	aria-label="Resize chat panel"
	{disabled}
	onmousedown={handleMouseDown}
	ontouchstart={handleTouchStart}
	onkeydown={handleKeyDown}
	data-testid="vertical-resize-handle"
>
	<span class="handle-line"></span>
</button>

<style>
	.vertical-resize-handle {
		position: relative;
		height: var(--panel-resize-handle-width, 4px);
		width: 100%;
		cursor: row-resize;
		background: var(--panel-resize-handle-bg, transparent);
		transition: background-color 150ms ease;
		flex-shrink: 0;
		z-index: 10;
		border: none;
		padding: 0;
		margin: 0;
		appearance: none;
	}

	.vertical-resize-handle:hover,
	.vertical-resize-handle:focus-visible {
		height: var(--panel-resize-handle-hover-width, 6px);
		background: var(--panel-resize-handle-hover-bg, var(--ui-interactive-primary));
	}

	.vertical-resize-handle.dragging {
		background: var(--panel-resize-handle-active-bg, var(--ui-interactive-primary-active));
	}

	.vertical-resize-handle:focus-visible {
		outline: 2px solid var(--ui-focus-ring);
		outline-offset: -2px;
	}

	.vertical-resize-handle:disabled {
		cursor: default;
		pointer-events: none;
		opacity: 0.5;
	}

	/* Invisible hit area */
	.vertical-resize-handle::before {
		content: '';
		position: absolute;
		left: 0;
		right: 0;
		top: -4px;
		bottom: -4px;
	}

	/* Visual line indicator */
	.handle-line {
		position: absolute;
		top: 50%;
		left: 50%;
		transform: translate(-50%, -50%);
		height: 2px;
		width: 24px;
		background: var(--ui-border-subtle);
		border-radius: 1px;
		opacity: 0;
		transition: opacity 150ms ease;
	}

	.vertical-resize-handle:hover .handle-line,
	.vertical-resize-handle:focus-visible .handle-line,
	.vertical-resize-handle.dragging .handle-line {
		opacity: 1;
		background: var(--ui-text-on-primary);
	}
</style>
