import { test, expect, hasBackend, extractInstance } from '../fixtures/test-base';

/**
 * Peer Review Lifecycle
 *
 * Exercises the full peer review API: creation, submission, blind enforcement,
 * completion, and query filters.
 *
 * Endpoints covered:
 *   POST   /game/reviews              (create)
 *   POST   /game/reviews/{id}/submit  (submit rating)
 *   GET    /game/reviews/{id}         (get single review)
 *   GET    /game/reviews              (list with filters)
 *   GET    /game/agents/{id}/reviews  (reviews by agent)
 *
 * Note: createReview stores the IDs exactly as passed. Since getAgentReviews
 * compares the path-extracted short instance ID against stored IDs, we must
 * pass short instance IDs to createReview/submitReview for consistency.
 */

// =============================================================================
// SOLO TASK LIFECYCLE
// =============================================================================

test.describe('Peer Review - Solo Task Lifecycle', () => {
	test('create solo review returns pending status', async ({ lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');

		const quest = await lifecycleApi.createQuest('E2E solo review quest', 1);
		const leader = await lifecycleApi.recruitAgent('solo-review-leader');
		const questInstance = extractInstance(quest.id);
		const leaderInstance = extractInstance(leader.id);

		const review = await lifecycleApi.createReview(
			questInstance,
			leaderInstance,
			leaderInstance, // solo task: leader and member are the same
			true
		);

		expect(review.id).toBeTruthy();
		expect(review.status).toBe('pending');
		expect(review.is_solo_task).toBe(true);
		expect(review.quest_id).toBe(questInstance);
	});

	test('leader submission completes solo review', async ({ lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');

		const quest = await lifecycleApi.createQuest('E2E solo complete quest', 1);
		const leader = await lifecycleApi.recruitAgent('solo-complete-leader');
		const questInstance = extractInstance(quest.id);
		const leaderInstance = extractInstance(leader.id);

		const review = await lifecycleApi.createReview(
			questInstance,
			leaderInstance,
			leaderInstance,
			true
		);
		const reviewInstance = extractInstance(review.id);

		// Leader submits — solo tasks complete with only the leader's submission
		const submitted = await lifecycleApi.submitReview(
			reviewInstance,
			leaderInstance,
			{ q1: 4, q2: 5, q3: 3 },
			'Good solo work'
		);

		expect(submitted.status).toBe('completed');
		expect(submitted.completed_at).toBeTruthy();
		expect(submitted.member_avg_rating).toBeGreaterThan(0);
	});

	test('get completed solo review shows ratings', async ({ lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');

		const quest = await lifecycleApi.createQuest('E2E solo get quest', 1);
		const leader = await lifecycleApi.recruitAgent('solo-get-leader');
		const questInstance = extractInstance(quest.id);
		const leaderInstance = extractInstance(leader.id);

		const review = await lifecycleApi.createReview(
			questInstance,
			leaderInstance,
			leaderInstance,
			true
		);
		const reviewInstance = extractInstance(review.id);

		await lifecycleApi.submitReview(reviewInstance, leaderInstance, { q1: 5, q2: 4, q3: 5 });

		// GET on completed review should show full ratings (blind enforcement lifted)
		const fetched = await lifecycleApi.getReview(reviewInstance);
		expect(fetched.status).toBe('completed');
		expect(fetched.leader_review).toBeTruthy();
		expect(fetched.leader_review!.ratings.q1).toBe(5);
	});

	test('list reviews filtered by quest_id returns the review', async ({ lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');

		const quest = await lifecycleApi.createQuest('E2E filter quest', 1);
		const leader = await lifecycleApi.recruitAgent('filter-leader');
		const questInstance = extractInstance(quest.id);
		const leaderInstance = extractInstance(leader.id);

		await lifecycleApi.createReview(questInstance, leaderInstance, leaderInstance, true);

		const reviews = await lifecycleApi.listReviews(undefined, questInstance);
		expect(reviews.length).toBeGreaterThanOrEqual(1);
		expect(reviews.some((r) => r.quest_id === questInstance)).toBe(true);
	});
});

// =============================================================================
// DUAL REVIEW LIFECYCLE
// =============================================================================

test.describe('Peer Review - Dual Review Lifecycle', () => {
	test('create dual review returns pending status', async ({ lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');

		const quest = await lifecycleApi.createQuest('E2E dual review quest', 1);
		const leader = await lifecycleApi.recruitAgent('dual-review-leader');
		const member = await lifecycleApi.recruitAgent('dual-review-member');
		const questInstance = extractInstance(quest.id);
		const leaderInstance = extractInstance(leader.id);
		const memberInstance = extractInstance(member.id);

		const review = await lifecycleApi.createReview(questInstance, leaderInstance, memberInstance);

		expect(review.id).toBeTruthy();
		expect(review.status).toBe('pending');
		expect(review.is_solo_task).toBe(false);
		expect(review.leader_id).toBe(leaderInstance);
		expect(review.member_id).toBe(memberInstance);
	});

	test('leader submission transitions to partial', async ({ lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');

		const quest = await lifecycleApi.createQuest('E2E dual partial quest', 1);
		const leader = await lifecycleApi.recruitAgent('dual-partial-leader');
		const member = await lifecycleApi.recruitAgent('dual-partial-member');
		const questInstance = extractInstance(quest.id);
		const leaderInstance = extractInstance(leader.id);
		const memberInstance = extractInstance(member.id);

		const review = await lifecycleApi.createReview(questInstance, leaderInstance, memberInstance);
		const reviewInstance = extractInstance(review.id);

		const submitted = await lifecycleApi.submitReview(
			reviewInstance,
			leaderInstance,
			{ q1: 4, q2: 3, q3: 5 },
			'Leader review of member'
		);

		expect(submitted.status).toBe('partial');
		// Blind enforcement: leader should see their own review but not member's
		expect(submitted.leader_review).toBeTruthy();
		expect(submitted.member_review).toBeFalsy();
	});

	test('member submission completes dual review with avg ratings', async ({ lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');

		const quest = await lifecycleApi.createQuest('E2E dual complete quest', 1);
		const leader = await lifecycleApi.recruitAgent('dual-complete-leader');
		const member = await lifecycleApi.recruitAgent('dual-complete-member');
		const questInstance = extractInstance(quest.id);
		const leaderInstance = extractInstance(leader.id);
		const memberInstance = extractInstance(member.id);

		const review = await lifecycleApi.createReview(questInstance, leaderInstance, memberInstance);
		const reviewInstance = extractInstance(review.id);

		// Leader submits first
		await lifecycleApi.submitReview(reviewInstance, leaderInstance, { q1: 4, q2: 3, q3: 5 });

		// Member submits — should complete the review
		const completed = await lifecycleApi.submitReview(
			reviewInstance,
			memberInstance,
			{ q1: 5, q2: 5, q3: 4 },
			'Member review of leader'
		);

		expect(completed.status).toBe('completed');
		expect(completed.completed_at).toBeTruthy();
		// Both avg ratings should be computed
		expect(completed.leader_avg_rating).toBeGreaterThan(0);
		expect(completed.member_avg_rating).toBeGreaterThan(0);
	});

	test('get completed dual review shows both submissions', async ({ lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');

		const quest = await lifecycleApi.createQuest('E2E dual get quest', 1);
		const leader = await lifecycleApi.recruitAgent('dual-get-leader');
		const member = await lifecycleApi.recruitAgent('dual-get-member');
		const questInstance = extractInstance(quest.id);
		const leaderInstance = extractInstance(leader.id);
		const memberInstance = extractInstance(member.id);

		const review = await lifecycleApi.createReview(questInstance, leaderInstance, memberInstance);
		const reviewInstance = extractInstance(review.id);

		await lifecycleApi.submitReview(reviewInstance, leaderInstance, { q1: 3, q2: 4, q3: 5 });
		await lifecycleApi.submitReview(reviewInstance, memberInstance, { q1: 5, q2: 4, q3: 3 });

		const fetched = await lifecycleApi.getReview(reviewInstance);
		expect(fetched.status).toBe('completed');
		expect(fetched.leader_review).toBeTruthy();
		expect(fetched.member_review).toBeTruthy();
	});
});

// =============================================================================
// BLIND ENFORCEMENT
// =============================================================================

test.describe('Peer Review - Blind Enforcement', () => {
	test('GET partial review strips both submissions', async ({ lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');

		const quest = await lifecycleApi.createQuest('E2E blind quest', 1);
		const leader = await lifecycleApi.recruitAgent('blind-leader');
		const member = await lifecycleApi.recruitAgent('blind-member');
		const questInstance = extractInstance(quest.id);
		const leaderInstance = extractInstance(leader.id);
		const memberInstance = extractInstance(member.id);

		const review = await lifecycleApi.createReview(questInstance, leaderInstance, memberInstance);
		const reviewInstance = extractInstance(review.id);

		// Leader submits — review is now partial
		await lifecycleApi.submitReview(reviewInstance, leaderInstance, { q1: 4, q2: 3, q3: 5 });

		// GET (unauthenticated) should strip both submissions for blind enforcement
		const fetched = await lifecycleApi.getReview(reviewInstance);
		expect(fetched.status).toBe('partial');
		// stripPartialSubmissions redacts both submissions on unauthenticated GET
		expect(fetched.leader_review).toBeFalsy();
		expect(fetched.member_review).toBeFalsy();
	});

	test('submit by non-participant returns error', async ({ lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');

		const quest = await lifecycleApi.createQuest('E2E nonpart quest', 1);
		const leader = await lifecycleApi.recruitAgent('nonpart-leader');
		const member = await lifecycleApi.recruitAgent('nonpart-member');
		const outsider = await lifecycleApi.recruitAgent('nonpart-outsider');
		const questInstance = extractInstance(quest.id);
		const leaderInstance = extractInstance(leader.id);
		const memberInstance = extractInstance(member.id);
		const outsiderInstance = extractInstance(outsider.id);

		const review = await lifecycleApi.createReview(questInstance, leaderInstance, memberInstance);
		const reviewInstance = extractInstance(review.id);

		// Non-participant submission should fail with 403.
		// Use ratings >= 3 to avoid triggering the "explanation required" validation (400)
		// before the reviewer check runs.
		try {
			await lifecycleApi.submitReview(reviewInstance, outsiderInstance, {
				q1: 4,
				q2: 4,
				q3: 4
			});
			expect(true, 'Expected submitReview to throw for non-participant').toBe(false);
		} catch (e) {
			const msg = (e as Error).message;
			expect(msg).toContain('403');
		}
	});
});

// =============================================================================
// VALIDATION & QUERIES
// =============================================================================

test.describe('Peer Review - Validation & Queries', () => {
	test('create review with missing quest_id returns 400', async ({ lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');

		const leader = await lifecycleApi.recruitAgent('validate-leader');
		const leaderInstance = extractInstance(leader.id);

		try {
			await lifecycleApi.createReview('', leaderInstance, leaderInstance, true);
			expect(true, 'Expected createReview with empty quest_id to throw').toBe(false);
		} catch (e) {
			const msg = (e as Error).message;
			expect(msg).toContain('400');
		}
	});

	test('agent reviews endpoint returns reviews where agent participates', async ({
		lifecycleApi
	}) => {
		test.skip(!hasBackend(), 'Requires running backend');

		const quest = await lifecycleApi.createQuest('E2E agent-reviews quest', 1);
		const leader = await lifecycleApi.recruitAgent('agentrev-leader');
		const member = await lifecycleApi.recruitAgent('agentrev-member');
		const questInstance = extractInstance(quest.id);
		const leaderInstance = extractInstance(leader.id);
		const memberInstance = extractInstance(member.id);

		await lifecycleApi.createReview(questInstance, leaderInstance, memberInstance);

		const reviews = await lifecycleApi.getAgentReviews(leaderInstance);

		expect(Array.isArray(reviews)).toBe(true);
		expect(reviews.length).toBeGreaterThanOrEqual(1);
		expect(
			reviews.some(
				(r) => r.leader_id === leaderInstance || r.member_id === leaderInstance
			)
		).toBe(true);
	});
});
