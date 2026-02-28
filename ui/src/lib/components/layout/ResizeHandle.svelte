<script lang="ts">
	/**
	 * ResizeHandle - Draggable divider between panels
	 *
	 * Features:
	 * - Mouse drag to resize
	 * - Touch support
	 * - Keyboard arrow keys for accessibility
	 * - Visual feedback on hover/active
	 * - Respects min/max constraints via callback
	 */

	interface ResizeHandleProps {
		/** Which direction the resize affects */
		direction: 'left' | 'right';
		/** Callback with delta pixels during drag */
		onResize?: (delta: number) => void;
		/** Callback when drag ends */
		onResizeEnd?: () => void;
		/** Disable interactions */
		disabled?: boolean;
	}

	let { direction, onResize, onResizeEnd, disabled = false }: ResizeHandleProps = $props();

	let isDragging = $state(false);
	let startX = $state(0);

	// Handle mouse down - start drag
	function handleMouseDown(event: MouseEvent) {
		if (disabled) return;
		event.preventDefault();
		isDragging = true;
		startX = event.clientX;

		// Add document-level listeners for drag
		document.addEventListener('mousemove', handleMouseMove);
		document.addEventListener('mouseup', handleMouseUp);
	}

	// Handle mouse move during drag
	function handleMouseMove(event: MouseEvent) {
		if (!isDragging) return;

		const delta = event.clientX - startX;
		startX = event.clientX;

		// For left panel, positive delta = grow; for right panel, negative delta = grow
		const adjustedDelta = direction === 'left' ? delta : -delta;
		onResize?.(adjustedDelta);
	}

	// Handle mouse up - end drag
	function handleMouseUp() {
		isDragging = false;
		document.removeEventListener('mousemove', handleMouseMove);
		document.removeEventListener('mouseup', handleMouseUp);
		onResizeEnd?.();
	}

	// Handle touch start
	function handleTouchStart(event: TouchEvent) {
		if (disabled) return;
		event.preventDefault();
		isDragging = true;
		startX = event.touches[0].clientX;

		document.addEventListener('touchmove', handleTouchMove, { passive: false });
		document.addEventListener('touchend', handleTouchEnd);
	}

	// Handle touch move
	function handleTouchMove(event: TouchEvent) {
		if (!isDragging) return;
		event.preventDefault();

		const delta = event.touches[0].clientX - startX;
		startX = event.touches[0].clientX;

		const adjustedDelta = direction === 'left' ? delta : -delta;
		onResize?.(adjustedDelta);
	}

	// Handle touch end
	function handleTouchEnd() {
		isDragging = false;
		document.removeEventListener('touchmove', handleTouchMove);
		document.removeEventListener('touchend', handleTouchEnd);
		onResizeEnd?.();
	}

	// Handle keyboard for accessibility
	function handleKeyDown(event: KeyboardEvent) {
		if (disabled) return;

		// Escape to cancel and blur
		if (event.key === 'Escape') {
			(event.currentTarget as HTMLElement)?.blur();
			return;
		}

		const step = event.shiftKey ? 50 : 10; // Larger steps with Shift
		let delta = 0;

		switch (event.key) {
			case 'ArrowLeft':
				delta = direction === 'left' ? -step : step;
				break;
			case 'ArrowRight':
				delta = direction === 'left' ? step : -step;
				break;
			default:
				return;
		}

		event.preventDefault();
		onResize?.(delta);
		onResizeEnd?.();
	}

	const ariaLabel = $derived(
		direction === 'left' ? 'Resize left panel' : 'Resize right panel'
	);
</script>

<button
	type="button"
	class="resize-handle"
	class:dragging={isDragging}
	aria-label={ariaLabel}
	{disabled}
	onmousedown={handleMouseDown}
	ontouchstart={handleTouchStart}
	onkeydown={handleKeyDown}
	data-testid="resize-handle-{direction}"
>
	<span class="handle-line"></span>
</button>

<style>
	.resize-handle {
		position: relative;
		width: var(--panel-resize-handle-width, 4px);
		cursor: col-resize;
		background: var(--panel-resize-handle-bg, transparent);
		transition: background-color 150ms ease;
		flex-shrink: 0;
		z-index: 10;
		border: none;
		padding: 0;
		margin: 0;
		appearance: none;
	}

	.resize-handle:hover,
	.resize-handle:focus-visible {
		width: var(--panel-resize-handle-hover-width, 6px);
		background: var(--panel-resize-handle-hover-bg, var(--ui-interactive-primary));
	}

	.resize-handle.dragging {
		background: var(--panel-resize-handle-active-bg, var(--ui-interactive-primary-active));
	}

	.resize-handle:focus-visible {
		outline: 2px solid var(--ui-focus-ring);
		outline-offset: -2px;
	}

	.resize-handle:disabled {
		cursor: default;
		pointer-events: none;
		opacity: 0.5;
	}

	/* Invisible hit area for easier grabbing */
	.resize-handle::before {
		content: '';
		position: absolute;
		top: 0;
		bottom: 0;
		left: -4px;
		right: -4px;
	}

	/* Visual line indicator */
	.handle-line {
		position: absolute;
		top: 50%;
		left: 50%;
		transform: translate(-50%, -50%);
		width: 2px;
		height: 24px;
		background: var(--ui-border-subtle);
		border-radius: 1px;
		opacity: 0;
		transition: opacity 150ms ease;
	}

	.resize-handle:hover .handle-line,
	.resize-handle:focus-visible .handle-line,
	.resize-handle.dragging .handle-line {
		opacity: 1;
		background: var(--ui-text-on-primary);
	}
</style>
