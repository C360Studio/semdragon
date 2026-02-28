<script lang="ts">
	/**
	 * Guild Registry - View of all guilds
	 */

	import { worldStore } from '$stores/worldStore.svelte';
</script>

<svelte:head>
	<title>Guilds - Semdragons</title>
</svelte:head>

<div class="guilds-page">
	<header class="page-header">
		<h1>Guild Registry</h1>
		<span class="guild-count">{worldStore.guildList.length} guilds</span>
	</header>

	<div class="guilds-grid">
		{#each worldStore.guildList as guild}
			<div class="guild-card" data-status={guild.status}>
				<div class="guild-header">
					<h2>{guild.name}</h2>
					<span class="guild-status">{guild.status}</span>
				</div>

				<p class="guild-description">{guild.description}</p>

				<div class="guild-domain">
					<span class="domain-label">Domain:</span>
					<span class="domain-value">{guild.domain}</span>
				</div>

				<div class="guild-stats">
					<div class="stat">
						<span class="stat-value">{guild.members.length}</span>
						<span class="stat-label">Members</span>
					</div>
					<div class="stat">
						<span class="stat-value">{(guild.reputation * 100).toFixed(0)}%</span>
						<span class="stat-label">Reputation</span>
					</div>
					<div class="stat">
						<span class="stat-value">{guild.quests_handled}</span>
						<span class="stat-label">Quests</span>
					</div>
					<div class="stat">
						<span class="stat-value">{(guild.success_rate * 100).toFixed(0)}%</span>
						<span class="stat-label">Success Rate</span>
					</div>
				</div>

				<div class="guild-skills">
					{#each guild.skills.slice(0, 4) as skill}
						<span class="skill-tag">{skill.replace('_', ' ')}</span>
					{/each}
					{#if guild.skills.length > 4}
						<span class="skill-more">+{guild.skills.length - 4}</span>
					{/if}
				</div>
			</div>
		{:else}
			<div class="empty-state">No guilds found</div>
		{/each}
	</div>
</div>

<style>
	.guilds-page {
		height: 100%;
		overflow-y: auto;
		padding: var(--spacing-lg);
		background: var(--ui-surface-primary);
	}

	.page-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: var(--spacing-lg);
	}

	.page-header h1 {
		margin: 0;
	}

	.guild-count {
		color: var(--ui-text-tertiary);
		font-size: 0.875rem;
	}

	.guilds-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
		gap: var(--spacing-lg);
	}

	.guild-card {
		background: var(--ui-surface-secondary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-lg);
		padding: var(--spacing-lg);
	}

	.guild-card[data-status='active'] {
		border-left: 4px solid var(--guild-active);
	}

	.guild-card[data-status='inactive'] {
		border-left: 4px solid var(--guild-inactive);
		opacity: 0.7;
	}

	.guild-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: var(--spacing-sm);
	}

	.guild-header h2 {
		margin: 0;
		font-size: 1.125rem;
	}

	.guild-status {
		font-size: 0.75rem;
		padding: 2px 8px;
		border-radius: var(--radius-full);
		text-transform: capitalize;
		background: var(--ui-surface-tertiary);
	}

	.guild-description {
		color: var(--ui-text-secondary);
		font-size: 0.875rem;
		margin-bottom: var(--spacing-md);
	}

	.guild-domain {
		font-size: 0.875rem;
		margin-bottom: var(--spacing-md);
	}

	.domain-label {
		color: var(--ui-text-tertiary);
	}

	.domain-value {
		font-weight: 500;
		text-transform: capitalize;
	}

	.guild-stats {
		display: grid;
		grid-template-columns: repeat(4, 1fr);
		gap: var(--spacing-sm);
		margin-bottom: var(--spacing-md);
		padding: var(--spacing-sm);
		background: var(--ui-surface-tertiary);
		border-radius: var(--radius-md);
	}

	.stat {
		text-align: center;
	}

	.stat-value {
		display: block;
		font-size: 1.125rem;
		font-weight: 600;
		color: var(--ui-interactive-primary);
	}

	.stat-label {
		font-size: 0.625rem;
		color: var(--ui-text-tertiary);
		text-transform: uppercase;
	}

	.guild-skills {
		display: flex;
		flex-wrap: wrap;
		gap: var(--spacing-xs);
	}

	.skill-tag {
		font-size: 0.75rem;
		padding: 2px 8px;
		background: var(--ui-surface-primary);
		border-radius: var(--radius-sm);
		text-transform: capitalize;
	}

	.skill-more {
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
	}

	.empty-state {
		grid-column: 1 / -1;
		text-align: center;
		color: var(--ui-text-tertiary);
		padding: var(--spacing-xl);
	}
</style>
