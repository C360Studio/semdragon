import { test, expect, hasBackend } from '../fixtures/test-base';

test.describe('Workspace', () => {
	test('displays page title', async ({ page }) => {
		await page.goto('/workspace');
		await expect(page.locator('h1')).toContainText('Workspace');
	});

	test('shows refresh button', async ({ page }) => {
		await page.goto('/workspace');
		const refresh = page.locator('button[aria-label="Refresh"]');
		await expect(refresh.first()).toBeVisible();
	});
});

test.describe('Workspace - Navigation', () => {
	test('can navigate to workspace from nav', async ({ page }) => {
		await page.goto('/');
		await page.locator('[data-testid="nav-workspace"]').click();
		await expect(page).toHaveURL(/.*\/workspace/);
		await expect(page.locator('h1')).toContainText('Workspace');
	});

	test('direct navigation to /workspace works', async ({ page }) => {
		await page.goto('/workspace');
		await expect(page.locator('h1')).toContainText('Workspace');
	});

	test('quest param auto-selects quest', async ({ page }) => {
		test.skip(!hasBackend(), 'Requires backend for quest data');

		// Navigate with a bogus quest ID — should show error or empty tree
		await page.goto('/workspace?quest=nonexistent');
		// Should be in file browser view (has back button)
		const backBtn = page.locator('button[aria-label="Back to quest list"]');
		await expect(backBtn).toBeVisible();
	});
});

test.describe('Workspace - With Backend', () => {
	test('loads quest list from artifact store', async ({ page, lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires backend for artifact store');

		await page.goto('/workspace');

		// Wait for loading to finish (either quests appear or empty state)
		await expect(async () => {
			const loading = await page.locator('text=Loading workspace').isVisible();
			expect(loading).toBe(false);
		}).toPass({ timeout: 5000 });
	});

	test('quest cards show title and file count', async ({ page, lifecycleApi, seedQuests }) => {
		test.skip(!hasBackend(), 'Requires backend for artifact store');

		await page.goto('/workspace');

		// Wait for loading to complete
		await expect(async () => {
			const loading = await page.locator('text=Loading workspace').isVisible();
			expect(loading).toBe(false);
		}).toPass({ timeout: 5000 });

		// If there are quest cards, verify they have expected structure
		const cards = page.locator('.quest-card');
		const count = await cards.count();
		if (count > 0) {
			const firstCard = cards.first();
			await expect(firstCard.locator('.quest-card-title')).toBeVisible();
			await expect(firstCard.locator('.quest-card-meta')).toBeVisible();
		}
	});

	test('clicking quest card opens file browser', async ({ page }) => {
		test.skip(!hasBackend(), 'Requires backend for artifact store');

		await page.goto('/workspace');

		// Wait for loading to complete
		await expect(async () => {
			const loading = await page.locator('text=Loading workspace').isVisible();
			expect(loading).toBe(false);
		}).toPass({ timeout: 5000 });

		const cards = page.locator('.quest-card');
		const count = await cards.count();
		if (count > 0) {
			await cards.first().click();
			// Should show back button (file browser view)
			const backBtn = page.locator('button[aria-label="Back to quest list"]');
			await expect(backBtn).toBeVisible();
		}
	});

	test('file browser shows tree and preview areas', async ({ page }) => {
		test.skip(!hasBackend(), 'Requires backend for artifact store');

		await page.goto('/workspace');

		// Wait for loading to complete
		await expect(async () => {
			const loading = await page.locator('text=Loading workspace').isVisible();
			expect(loading).toBe(false);
		}).toPass({ timeout: 5000 });

		const cards = page.locator('.quest-card');
		const count = await cards.count();
		if (count > 0) {
			await cards.first().click();

			// Wait for tree to load
			await expect(async () => {
				const loading = await page.locator('text=Loading artifacts').isVisible();
				expect(loading).toBe(false);
			}).toPass({ timeout: 5000 });

			// Should have tree and preview areas
			const tree = page.locator('[data-testid="workspace-tree"]');
			const preview = page.locator('[data-testid="workspace-preview"]');
			await expect(tree).toBeVisible();
			await expect(preview).toBeVisible();
		}
	});

	test('back button returns to quest list', async ({ page }) => {
		test.skip(!hasBackend(), 'Requires backend for artifact store');

		await page.goto('/workspace');

		await expect(async () => {
			const loading = await page.locator('text=Loading workspace').isVisible();
			expect(loading).toBe(false);
		}).toPass({ timeout: 5000 });

		const cards = page.locator('.quest-card');
		const count = await cards.count();
		if (count > 0) {
			await cards.first().click();
			const backBtn = page.locator('button[aria-label="Back to quest list"]');
			await expect(backBtn).toBeVisible();
			await backBtn.click();
			// Should be back to quest list view
			await expect(page.locator('h1')).toContainText('Workspace');
		}
	});

	test('parent quests with sub-quests show expand toggle', async ({ page }) => {
		test.skip(!hasBackend(), 'Requires backend with party quest data');

		await page.goto('/workspace');

		await expect(async () => {
			const loading = await page.locator('text=Loading workspace').isVisible();
			expect(loading).toBe(false);
		}).toPass({ timeout: 5000 });

		// If there are parent quests with children, they should have an expand toggle
		const expandToggles = page.locator('.expand-toggle');
		const toggleCount = await expandToggles.count();
		if (toggleCount > 0) {
			// Click expand toggle — should show sub-quest list
			await expandToggles.first().click();
			const subList = page.locator('.sub-quest-list');
			await expect(subList.first()).toBeVisible();
		}
	});
});
