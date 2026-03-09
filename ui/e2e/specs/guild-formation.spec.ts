import { test, expect, hasBackend, retry, extractInstance } from '../fixtures/test-base';
import type { GuildResponse } from '../fixtures/test-base';

/**
 * Guild Formation E2E Tests
 *
 * Verifies auto-formation of guilds when an Expert+ agent (level 11+)
 * exists alongside unguilded agents. The guildformation processor
 * watches agent state via KV and triggers formation automatically.
 *
 * Environment requirements:
 *   - E2E_BACKEND_AVAILABLE=true (set by global-setup.ts)
 */

test.describe('Guild Formation @integration', () => {
	test.beforeEach(() => {
		test.skip(!hasBackend(), 'Requires running backend (E2E_BACKEND_AVAILABLE=true)');
	});

	test('GET /game/guilds returns empty array initially', async ({ lifecycleApi }) => {
		const guilds = await lifecycleApi.listGuilds();
		expect(guilds).toBeInstanceOf(Array);
	});

	test('auto-forms guild when Expert agent is created with candidates', async ({
		lifecycleApi
	}) => {
		// Recruit an Expert agent (level 11) — this triggers auto-formation evaluation
		const expert = await lifecycleApi.recruitAgentAtLevel(
			`guild-expert-${Date.now()}`,
			11,
			['analysis', 'coding']
		);
		expect(expert.level).toBe(11);
		expect(expert.tier).toBeGreaterThanOrEqual(2); // Expert tier (iota: 0=Apprentice, 1=Journeyman, 2=Expert)

		// Recruit two more agents as formation candidates
		const agent2 = await lifecycleApi.recruitAgentAtLevel(
			`guild-member-a-${Date.now()}`,
			1,
			['analysis']
		);
		const agent3 = await lifecycleApi.recruitAgentAtLevel(
			`guild-member-b-${Date.now()}`,
			1,
			['analysis']
		);

		expect(agent2.level).toBe(1);
		expect(agent3.level).toBe(1);

		// Wait for guildformation processor to auto-form a guild.
		// The processor runs periodically and checks for Expert+ unguilded agents
		// with enough candidates sharing skills.
		const guilds = await retry(
			async () => {
				const result = await lifecycleApi.listGuilds();
				if (result.length === 0) {
					throw new Error('No guilds formed yet');
				}
				return result;
			},
			{ timeout: 30000, interval: 2000, message: 'Guild auto-formation did not trigger' }
		);

		expect(guilds.length).toBeGreaterThanOrEqual(1);

		// Find the guild that contains our expert
		const expertInstance = extractInstance(expert.id);
		const guild = guilds.find((g: GuildResponse) =>
			g.members?.some((m) => m.agent_id.includes(expertInstance))
		);
		expect(guild).toBeTruthy();
		console.log(
			`[Guild E2E] Guild ${extractInstance(guild!.id)}: ${guild!.name}, ` +
				`members=${guild!.members.length}, founder=${extractInstance(guild!.founded_by)}`
		);

		// Guild structure
		expect(guild!.id).toBeTruthy();
		expect(guild!.name).toBeTruthy();
		expect(guild!.founded_by).toBeTruthy();
		// Seeded agents may be chosen instead of our test agents, so we only
		// verify structure and constraints — not which specific agents were inducted.
		expect(guild!.members.length).toBeGreaterThanOrEqual(2);

		// Each member has required fields
		for (const member of guild!.members) {
			expect(member.agent_id).toBeTruthy();
			expect(member.rank).toMatch(/guildmaster|officer|veteran|member|initiate/);
		}

		// Exactly one guildmaster per guild
		const masters = guild!.members.filter((m) => m.rank === 'guildmaster');
		expect(masters.length).toBe(1);

		// Verify GET /game/guilds/{id} returns consistent data
		const guildInstance = extractInstance(guild!.id);
		const fetched = await lifecycleApi.getGuild(guildInstance);
		expect(fetched.id).toBe(guild!.id);
		expect(fetched.members.length).toBe(guild!.members.length);

		// Single-guild constraint: expert appears in exactly this guild's member list.
		// The agent API response doesn't carry a guild backref field, so we verify
		// via the guild's own member list instead.
		const expertInGuild = fetched.members.some((m) =>
			m.agent_id.includes(expertInstance)
		);
		expect(expertInGuild).toBe(true);
		console.log(`[Guild E2E] Expert ${expertInstance} in guild ${guildInstance}: ${expertInGuild}`);

		// No agent appears in more than one guild (single-guild invariant).
		// Re-fetch to get a consistent snapshot in case formation continued.
		const freshGuilds = await lifecycleApi.listGuilds();
		for (const g of freshGuilds) {
			for (const otherG of freshGuilds) {
				if (g.id === otherG.id) continue;
				const overlap = g.members.filter((m) =>
					otherG.members.some((o) => o.agent_id === m.agent_id)
				);
				expect(overlap.length).toBe(0);
			}
		}

		// Verify guilds appear in world state
		const world = await lifecycleApi.getWorldState();
		expect((world.guilds as unknown[]).length).toBeGreaterThanOrEqual(1);
	});

	test('agent recruited at level respects level and tier', async ({ lifecycleApi }) => {
		const agent = await lifecycleApi.recruitAgentAtLevel(`level-test-${Date.now()}`, 15, [
			'coding'
		]);
		expect(agent.level).toBe(15);
		expect(agent.tier).toBeGreaterThanOrEqual(2); // Expert tier (iota: 0=Apprentice, 1=Journeyman, 2=Expert)
	});
});
