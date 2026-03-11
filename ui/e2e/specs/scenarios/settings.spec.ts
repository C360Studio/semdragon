import { test, expect, hasBackend } from '../../fixtures/test-base';

test.describe('Settings - Page Structure', () => {
	test('displays page title', async ({ settingsPage }) => {
		await settingsPage.goto();
		await expect(settingsPage.heading).toContainText('Settings');
	});

	test('shows refresh button', async ({ settingsPage }) => {
		await settingsPage.goto();
		await expect(settingsPage.refreshBtn).toBeVisible();
	});

	test('shows backend unavailable when no backend', async ({ settingsPage }) => {
		test.skip(hasBackend(), 'Only runs without backend');

		await settingsPage.goto();
		await expect(settingsPage.unavailableState).toBeVisible();
		await expect(settingsPage.unavailableState).toContainText('Backend Unavailable');
	});
});

test.describe('Settings - Sections', () => {
	test.beforeEach(async ({ settingsPage }) => {
		test.skip(!hasBackend(), 'Requires backend');
		await settingsPage.goto();
	});

	test('displays connection section', async ({ settingsPage }) => {
		await expect(settingsPage.sectionConnection).toBeVisible();
		const badge = settingsPage.getSectionBadge('settings-section-connection');
		await expect(badge).toHaveText(/Connected|Disconnected/);
	});

	test('displays platform identity section', async ({ settingsPage }) => {
		await expect(settingsPage.sectionPlatformIdentity).toBeVisible();
		// Should show org, platform, board values
		await expect(settingsPage.sectionPlatformIdentity.locator('.kv-value').first()).not.toBeEmpty();
	});

	test('displays LLM providers section with endpoint count', async ({ settingsPage }) => {
		await expect(settingsPage.sectionLLMProviders).toBeVisible();
		const badge = settingsPage.getSectionBadge('settings-section-llm-providers');
		await expect(badge).toHaveText(/\d+ endpoints?/);
	});

	test('displays capability routing section', async ({ settingsPage }) => {
		await expect(settingsPage.sectionCapabilityRouting).toBeVisible();
		const badge = settingsPage.getSectionBadge('settings-section-capability-routing');
		await expect(badge).toHaveText(/\d+ capabilit/);
	});

	test('displays components section with running count', async ({ settingsPage }) => {
		await expect(settingsPage.sectionComponents).toBeVisible();
		const badge = settingsPage.getSectionBadge('settings-section-components');
		await expect(badge).toHaveText(/\d+\/\d+ running/);
	});

	test('displays web search section', async ({ settingsPage }) => {
		await expect(settingsPage.sectionWebSearch).toBeVisible();
		const badge = settingsPage.getSectionBadge('settings-section-web-search');
		await expect(badge).toHaveText(/Configured|Key missing|Not configured/);
	});

	test('displays websocket input section', async ({ settingsPage }) => {
		await expect(settingsPage.sectionWebsocketInput).toBeVisible();
		const badge = settingsPage.getSectionBadge('settings-section-websocket-input');
		await expect(badge).toHaveText(/Connected|Disconnected|Disabled/);
	});

	test('displays workspace section', async ({ settingsPage }) => {
		await expect(settingsPage.sectionWorkspace).toBeVisible();
		const badge = settingsPage.getSectionBadge('settings-section-workspace');
		await expect(badge).toHaveText(/OK|Missing/);
	});

	test('displays token budget section', async ({ settingsPage }) => {
		await expect(settingsPage.sectionTokenBudget).toBeVisible();
		// Should show either "Unlimited" or a number
		await expect(
			settingsPage.sectionTokenBudget.locator('.editable-value .mono')
		).toHaveText(/Unlimited|\d/);
	});

	test('displays onboarding checklist', async ({ settingsPage }) => {
		await expect(settingsPage.sectionGettingStarted).toBeVisible();
	});
});

test.describe('Settings - Section Collapsibility', () => {
	test.beforeEach(async ({ settingsPage }) => {
		test.skip(!hasBackend(), 'Requires backend');
		await settingsPage.goto();
	});

	test('sections start expanded by default', async ({ settingsPage }) => {
		const expanded = await settingsPage.isSectionExpanded('settings-section-connection');
		expect(expanded).toBe(true);
	});

	test('clicking section header collapses it', async ({ settingsPage }) => {
		await settingsPage.toggleSection('settings-section-connection');
		const expanded = await settingsPage.isSectionExpanded('settings-section-connection');
		expect(expanded).toBe(false);

		// Section body should be hidden
		await expect(
			settingsPage.sectionConnection.locator('.section-body')
		).not.toBeVisible();
	});

	test('clicking collapsed section re-expands it', async ({ settingsPage }) => {
		// Collapse
		await settingsPage.toggleSection('settings-section-connection');
		expect(await settingsPage.isSectionExpanded('settings-section-connection')).toBe(false);

		// Re-expand
		await settingsPage.toggleSection('settings-section-connection');
		expect(await settingsPage.isSectionExpanded('settings-section-connection')).toBe(true);

		await expect(
			settingsPage.sectionConnection.locator('.section-body')
		).toBeVisible();
	});
});

test.describe('Settings - Help Panel', () => {
	test.beforeEach(async ({ settingsPage }) => {
		test.skip(!hasBackend(), 'Requires backend');
		await settingsPage.goto();
	});

	test('default state shows placeholder text', async ({ settingsPage }) => {
		await expect(settingsPage.helpPlaceholder).toBeVisible();
		await expect(settingsPage.helpPlaceholder).toContainText('Select a settings section');
	});

	test('hovering connection shows Connection help', async ({ settingsPage }) => {
		await settingsPage.hoverSection('settings-section-connection');
		await expect(settingsPage.helpTitle).toHaveText('Connection');
	});

	test('hovering providers shows LLM Providers help', async ({ settingsPage }) => {
		await settingsPage.hoverSection('settings-section-llm-providers');
		await expect(settingsPage.helpTitle).toHaveText('LLM Providers');
	});

	test('hovering websocket shows WebSocket Input help', async ({ settingsPage }) => {
		await settingsPage.hoverSection('settings-section-websocket-input');
		await expect(settingsPage.helpTitle).toHaveText('WebSocket Input');
	});
});

test.describe('Settings - WebSocket Controls', () => {
	test.beforeEach(async ({ settingsPage }) => {
		test.skip(!hasBackend(), 'Requires backend');
		await settingsPage.goto();
	});

	test('websocket toggle button is visible', async ({ settingsPage }) => {
		await expect(settingsPage.wsToggleBtn).toBeVisible();
		await expect(settingsPage.wsToggleBtn).toHaveText(/Enabled|Disabled/);
	});

	test('clicking toggle changes state', async ({ settingsPage }) => {
		const initialText = await settingsPage.wsToggleBtn.textContent();
		await settingsPage.wsToggleBtn.click();

		// Wait for the toggle text to change
		if (initialText?.trim() === 'Enabled') {
			await expect(settingsPage.wsToggleBtn).toHaveText('Disabled');
		} else {
			await expect(settingsPage.wsToggleBtn).toHaveText('Enabled');
		}

		// Toggle back to restore original state
		await settingsPage.wsToggleBtn.click();
		await expect(settingsPage.wsToggleBtn).toHaveText(initialText!.trim());
	});

	test('clicking URL shows edit form', async ({ settingsPage }) => {
		await settingsPage.wsUrlDisplay.click();
		await expect(settingsPage.wsUrlInput).toBeVisible();
		await expect(settingsPage.wsUrlSave).toBeVisible();
		await expect(settingsPage.wsUrlCancel).toBeVisible();
	});

	test('cancel editing hides form', async ({ settingsPage }) => {
		await settingsPage.wsUrlDisplay.click();
		await expect(settingsPage.wsUrlInput).toBeVisible();

		await settingsPage.wsUrlCancel.click();
		await expect(settingsPage.wsUrlInput).not.toBeVisible();
		await expect(settingsPage.wsUrlDisplay).toBeVisible();
	});

	test('escape key cancels editing', async ({ settingsPage }) => {
		await settingsPage.wsUrlDisplay.click();
		await expect(settingsPage.wsUrlInput).toBeVisible();

		await settingsPage.wsUrlInput.press('Escape');
		await expect(settingsPage.wsUrlInput).not.toBeVisible();
		await expect(settingsPage.wsUrlDisplay).toBeVisible();
	});

	test('save URL updates display', async ({ settingsPage }) => {
		const newUrl = 'ws://test-host:9999/ws';

		// Get original URL for cleanup
		const originalUrl = await settingsPage.wsUrlDisplay.locator('.mono').textContent();

		// Edit and save
		await settingsPage.wsUrlDisplay.click();
		await settingsPage.wsUrlInput.clear();
		await settingsPage.wsUrlInput.fill(newUrl);
		await settingsPage.wsUrlSave.click();

		// Verify new URL is displayed
		await expect(settingsPage.wsUrlDisplay).toBeVisible();
		await expect(settingsPage.wsUrlDisplay.locator('.mono')).toHaveText(newUrl);

		// Restore original URL
		if (originalUrl && originalUrl !== '(not configured)') {
			await settingsPage.wsUrlDisplay.click();
			await settingsPage.wsUrlInput.clear();
			await settingsPage.wsUrlInput.fill(originalUrl);
			await settingsPage.wsUrlSave.click();
		}
	});

	test('enter key saves URL', async ({ settingsPage }) => {
		const newUrl = 'ws://enter-test:8888/ws';
		const originalUrl = await settingsPage.wsUrlDisplay.locator('.mono').textContent();

		await settingsPage.wsUrlDisplay.click();
		await settingsPage.wsUrlInput.clear();
		await settingsPage.wsUrlInput.fill(newUrl);
		await settingsPage.wsUrlInput.press('Enter');

		await expect(settingsPage.wsUrlDisplay).toBeVisible();
		await expect(settingsPage.wsUrlDisplay.locator('.mono')).toHaveText(newUrl);

		// Restore
		if (originalUrl && originalUrl !== '(not configured)') {
			await settingsPage.wsUrlDisplay.click();
			await settingsPage.wsUrlInput.clear();
			await settingsPage.wsUrlInput.fill(originalUrl);
			await settingsPage.wsUrlSave.click();
		}
	});
});

test.describe('Settings - Token Budget Editing', () => {
	test.beforeEach(async ({ settingsPage }) => {
		test.skip(!hasBackend(), 'Requires backend');
		await settingsPage.goto();
	});

	test('clicking budget value shows edit form', async ({ settingsPage }) => {
		await settingsPage.budgetDisplay.click();
		await expect(settingsPage.budgetInput).toBeVisible();
		await expect(settingsPage.budgetSave).toBeVisible();
		await expect(settingsPage.budgetCancel).toBeVisible();
	});

	test('cancel editing hides form', async ({ settingsPage }) => {
		await settingsPage.budgetDisplay.click();
		await expect(settingsPage.budgetInput).toBeVisible();

		await settingsPage.budgetCancel.click();
		await expect(settingsPage.budgetInput).not.toBeVisible();
		await expect(settingsPage.budgetDisplay).toBeVisible();
	});

	test('save budget updates display', async ({ settingsPage }) => {
		// Read original value for cleanup
		const originalText = await settingsPage.budgetDisplay.locator('.mono').textContent();

		await settingsPage.budgetDisplay.click();
		await settingsPage.budgetInput.clear();
		await settingsPage.budgetInput.fill('500000');
		await settingsPage.budgetSave.click();

		// Wait for the edit form to disappear (save round-trip complete)
		await expect(settingsPage.budgetInput).not.toBeVisible({ timeout: 10000 });
		await expect(settingsPage.budgetDisplay).toBeVisible();
		await expect(settingsPage.budgetDisplay.locator('.mono')).toHaveText('500,000', { timeout: 5000 });

		// Restore original
		await settingsPage.budgetDisplay.click();
		await settingsPage.budgetInput.clear();
		const restoreValue = originalText === 'Unlimited' ? '0' : originalText!.replace(/,/g, '');
		await settingsPage.budgetInput.fill(restoreValue);
		await settingsPage.budgetSave.click();
	});
});

test.describe('Settings - LLM Provider Editing', () => {
	test.beforeEach(async ({ settingsPage }) => {
		test.skip(!hasBackend(), 'Requires backend');
		await settingsPage.goto();
	});

	test('edit button visible on provider rows', async ({ settingsPage }) => {
		const editBtn = settingsPage.sectionLLMProviders.locator('.row-btn.edit').first();
		await expect(editBtn).toBeVisible();
	});

	test('clicking edit shows inline form', async ({ settingsPage }) => {
		const editBtn = settingsPage.sectionLLMProviders.locator('.row-btn.edit').first();
		await editBtn.click();
		await expect(settingsPage.sectionLLMProviders.locator('.editing-row')).toBeVisible();
	});

	test('cancel edit hides form', async ({ settingsPage }) => {
		const editBtn = settingsPage.sectionLLMProviders.locator('.row-btn.edit').first();
		await editBtn.click();

		const cancelBtn = settingsPage.sectionLLMProviders.locator('.editing-row .row-btn.cancel');
		await cancelBtn.click();
		await expect(settingsPage.sectionLLMProviders.locator('.editing-row')).not.toBeVisible();
	});

	test('add endpoint button visible', async ({ settingsPage }) => {
		const addBtn = settingsPage.sectionLLMProviders.locator('.add-btn');
		await expect(addBtn).toBeVisible();
		await expect(addBtn).toHaveText(/Add Endpoint/);
	});
});

test.describe('Settings - Capability Routing Editing', () => {
	test.beforeEach(async ({ settingsPage }) => {
		test.skip(!hasBackend(), 'Requires backend');
		await settingsPage.goto();
	});

	test('edit button visible on capability cards', async ({ settingsPage }) => {
		const editBtn = settingsPage.sectionCapabilityRouting.locator('.cap-edit-btn').first();
		await expect(editBtn).toBeVisible();
	});

	test('clicking edit shows chain editor', async ({ settingsPage }) => {
		const editBtn = settingsPage.sectionCapabilityRouting.locator('.cap-edit-btn').first();
		await editBtn.click();
		await expect(settingsPage.sectionCapabilityRouting.locator('.chain-editor')).toBeVisible();
	});

	test('cancel edit hides chain editor', async ({ settingsPage }) => {
		const editBtn = settingsPage.sectionCapabilityRouting.locator('.cap-edit-btn').first();
		await editBtn.click();

		const cancelBtn = settingsPage.sectionCapabilityRouting.locator('.chain-cancel-btn');
		await cancelBtn.click();
		await expect(
			settingsPage.sectionCapabilityRouting.locator('.chain-editor')
		).not.toBeVisible();
	});
});

test.describe('Settings - Web Search Config', () => {
	test.beforeEach(async ({ settingsPage }) => {
		test.skip(!hasBackend(), 'Requires backend');
		await settingsPage.goto();
	});

	test('web search section shows provider status', async ({ settingsPage }) => {
		await expect(settingsPage.sectionWebSearch).toBeVisible();
		// Should show provider name or "None"
		await expect(settingsPage.sectionWebSearch.locator('.kv-value').first()).toBeVisible();
	});

	test('configure/edit button visible', async ({ settingsPage }) => {
		const btn = settingsPage.sectionWebSearch.locator('.section-edit-btn');
		await expect(btn).toBeVisible();
		await expect(btn).toHaveText(/Configure|Edit/);
	});

	test('clicking configure shows edit form', async ({ settingsPage }) => {
		const btn = settingsPage.sectionWebSearch.locator('.section-edit-btn');
		await btn.click();
		await expect(settingsPage.sectionWebSearch.locator('.search-edit')).toBeVisible();
	});

	test('cancel editing hides form', async ({ settingsPage }) => {
		const btn = settingsPage.sectionWebSearch.locator('.section-edit-btn');
		await btn.click();

		const cancelBtn = settingsPage.sectionWebSearch.locator('.inline-btn.cancel');
		await cancelBtn.click();
		await expect(settingsPage.sectionWebSearch.locator('.search-edit')).not.toBeVisible();
	});
});

test.describe('Settings - Help Panel (new sections)', () => {
	test.beforeEach(async ({ settingsPage }) => {
		test.skip(!hasBackend(), 'Requires backend');
		await settingsPage.goto();
	});

	test('hovering web search shows Web Search help', async ({ settingsPage }) => {
		await settingsPage.hoverSection('settings-section-web-search');
		await expect(settingsPage.helpTitle).toHaveText('Web Search');
	});
});

test.describe('Settings - Navigation', () => {
	test('refresh button reloads data', async ({ settingsPage }) => {
		test.skip(!hasBackend(), 'Requires backend');

		await settingsPage.goto();
		await expect(settingsPage.sectionsContainer).toBeVisible();

		// Click refresh
		await settingsPage.refreshBtn.click();

		// Sections should still be visible after refresh
		await expect(settingsPage.sectionsContainer).toBeVisible();
	});

	test('can navigate to settings from dashboard', async ({ dashboardPage, page }) => {
		await dashboardPage.goto();

		// Click settings nav link
		await page.locator('[data-testid="nav-settings"]').click();
		await expect(page).toHaveURL(/.*\/settings/);
	});
});
