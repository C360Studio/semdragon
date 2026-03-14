<script lang="ts">
	/**
	 * ToastContainer - Fixed overlay for quest attention toasts
	 *
	 * Renders active toasts from notificationStore in the top-right corner.
	 * Color-coded by type: magenta (escalation), orange (triage), red (failure).
	 * Escalation/triage toasts have a "View in Chat" button.
	 * Failure toasts show retry attempt count.
	 */

	import { notificationStore, type Toast } from '$lib/stores/notificationStore.svelte';
	import { chatStore } from '$lib/stores/chatStore.svelte';

	function handleViewInChat() {
		chatStore.open = true;
	}

	function handleDismiss(id: string) {
		notificationStore.dismissToast(id);
	}

	function toastTypeLabel(toast: Toast): string {
		switch (toast.type) {
			case 'escalation': return 'Escalation';
			case 'triage': return 'Needs Triage';
			case 'failure': return 'Failed';
		}
	}
</script>

{#if notificationStore.toasts.length > 0}
	<div class="toast-container" aria-live="polite" aria-label="Notifications">
		{#each notificationStore.toasts as toast (toast.id)}
			<div
				class="toast"
				data-type={toast.type}
				role="alert"
				data-testid="toast-{toast.type}"
			>
				<div class="toast-header">
					<span class="toast-type">{toastTypeLabel(toast)}</span>
					<button
						class="toast-dismiss"
						onclick={() => handleDismiss(toast.id)}
						aria-label="Dismiss"
					>&times;</button>
				</div>
				<div class="toast-title">{toast.questTitle}</div>
				<div class="toast-message">{toast.message}</div>
				{#if toast.type !== 'failure'}
					<button
						class="toast-action"
						onclick={handleViewInChat}
						aria-label="View {toast.questTitle} in chat"
						data-testid="toast-view-chat"
					>View in Chat</button>
				{/if}
			</div>
		{/each}
	</div>
{/if}

<style>
	.toast-container {
		position: fixed;
		top: 48px;
		right: var(--spacing-md);
		z-index: 1000;
		display: flex;
		flex-direction: column;
		gap: var(--spacing-sm);
		max-width: 360px;
		pointer-events: none;
	}

	.toast {
		pointer-events: auto;
		background: var(--ui-surface-secondary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-md);
		padding: var(--spacing-sm) var(--spacing-md);
		box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
		animation: toast-slide-in 200ms ease-out;
		border-left: 3px solid var(--ui-border-subtle);
	}

	.toast[data-type='escalation'] {
		border-left-color: var(--quest-escalated);
	}

	.toast[data-type='triage'] {
		border-left-color: var(--status-warning, #ff832b);
	}

	.toast[data-type='failure'] {
		border-left-color: var(--quest-failed);
	}

	.toast-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 2px;
	}

	.toast-type {
		font-size: 0.625rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}

	.toast[data-type='escalation'] .toast-type {
		color: var(--quest-escalated);
	}

	.toast[data-type='triage'] .toast-type {
		color: var(--status-warning, #ff832b);
	}

	.toast[data-type='failure'] .toast-type {
		color: var(--quest-failed);
	}

	.toast-dismiss {
		border: none;
		background: none;
		color: var(--ui-text-tertiary);
		font-size: 1rem;
		cursor: pointer;
		padding: 0 2px;
		line-height: 1;
	}

	.toast-dismiss:hover {
		color: var(--ui-text-primary);
	}

	.toast-title {
		font-size: 0.8125rem;
		font-weight: 600;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		margin-bottom: 2px;
	}

	.toast-message {
		font-size: 0.75rem;
		color: var(--ui-text-secondary);
		line-height: 1.4;
		overflow: hidden;
		text-overflow: ellipsis;
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
	}

	.toast-action {
		margin-top: var(--spacing-xs);
		padding: 2px var(--spacing-sm);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-sm);
		background: transparent;
		color: var(--ui-text-secondary);
		font-size: 0.6875rem;
		cursor: pointer;
		transition: all 150ms ease;
	}

	.toast-action:hover {
		color: var(--ui-text-primary);
		border-color: var(--ui-border-interactive);
	}

	@keyframes toast-slide-in {
		from {
			transform: translateX(100%);
			opacity: 0;
		}
		to {
			transform: translateX(0);
			opacity: 1;
		}
	}
</style>
