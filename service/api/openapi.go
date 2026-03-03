package api

import (
	"github.com/c360studio/semstreams/service"
)

func init() {
	service.RegisterOpenAPISpec("game", semdragonsOpenAPISpec())
}

// questIDParam is reused across quest lifecycle endpoints.
var questIDParam = service.ParameterSpec{
	Name: "id", In: "path", Required: true,
	Description: "Quest ID",
	Schema:      service.Schema{Type: "string"},
}

// agentIDParam is reused across agent endpoints.
var agentIDParam = service.ParameterSpec{
	Name: "id", In: "path", Required: true,
	Description: "Agent ID",
	Schema:      service.Schema{Type: "string"},
}

// semdragonsOpenAPISpec returns the OpenAPI specification for domain endpoints.
func semdragonsOpenAPISpec() *service.OpenAPISpec {
	return &service.OpenAPISpec{
		Paths: map[string]service.PathSpec{
			// ── World ────────────────────────────────────────────
			"/world": {
				GET: &service.OperationSpec{
					Summary:     "Get world state",
					Description: "Returns the complete game world state including agents, quests, parties, guilds, battles, and board statistics.",
					Tags:        []string{"World"},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "World state snapshot", ContentType: "application/json"},
					},
				},
			},

			// ── Quests ───────────────────────────────────────────
			"/quests": {
				GET: &service.OperationSpec{
					Summary:     "List quests",
					Description: "Returns all quests with optional filtering by status, difficulty, or guild.",
					Tags:        []string{"Quests"},
					Parameters: []service.ParameterSpec{
						{Name: "status", In: "query", Description: "Filter by quest status (posted, claimed, in_progress, in_review, completed, failed)", Schema: service.Schema{Type: "string"}},
						{Name: "difficulty", In: "query", Description: "Filter by difficulty level (0-5)", Schema: service.Schema{Type: "integer"}},
						{Name: "guild_id", In: "query", Description: "Filter by guild priority", Schema: service.Schema{Type: "string"}},
					},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "List of quests", ContentType: "application/json"},
					},
				},
				POST: &service.OperationSpec{
					Summary: "Create quest",
					Description: "Posts a new quest to the quest board.\n\n" +
						"Request body: {title, description?, difficulty? (0-5), skills? [skill_tag...], " +
						"acceptance? [string...], hints? {suggested_difficulty?, suggested_skills?, " +
						"prefer_guild?, require_human_review?, budget?, deadline?}}",
					Tags: []string{"Quests"},
					Responses: map[string]service.ResponseSpec{
						"201": {Description: "Quest created", ContentType: "application/json"},
						"400": {Description: "Invalid request body or missing title"},
					},
				},
			},
			"/quests/{id}": {
				GET: &service.OperationSpec{
					Summary:     "Get quest",
					Description: "Returns a single quest by ID.",
					Tags:        []string{"Quests"},
					Parameters:  []service.ParameterSpec{questIDParam},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Quest details", ContentType: "application/json"},
						"404": {Description: "Quest not found"},
					},
				},
			},
			"/quests/chain": {
				POST: &service.OperationSpec{
					Summary: "Create quest chain",
					Description: "Posts multiple linked quests in a single request. Dependencies use " +
						"0-based indices into the quests array, which are resolved to actual quest IDs.\n\n" +
						"Request body: {quests: [{title, description?, difficulty?, skills?, " +
						"acceptance?, depends_on? [index...], hints?}, ...]}",
					Tags: []string{"Quests"},
					Responses: map[string]service.ResponseSpec{
						"201": {Description: "Array of created quests with resolved dependency IDs", ContentType: "application/json"},
						"400": {Description: "Invalid request, dependency cycle, or out-of-bounds index"},
					},
				},
			},

			// ── Quest Lifecycle ──────────────────────────────────
			"/quests/{id}/claim": {
				POST: &service.OperationSpec{
					Summary: "Claim quest",
					Description: "Agent claims a posted quest. Validates that the agent meets " +
						"tier and skill requirements.\n\n" +
						"Request body: {agent_id (required)}",
					Tags:       []string{"Quest Lifecycle"},
					Parameters: []service.ParameterSpec{questIDParam},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Quest claimed, status changed to 'claimed'", ContentType: "application/json"},
						"400": {Description: "Missing agent_id"},
						"403": {Description: "Agent tier too low or missing required skills"},
						"404": {Description: "Quest or agent not found"},
						"409": {Description: "Quest not in 'posted' status or agent not idle"},
					},
				},
			},
			"/quests/{id}/start": {
				POST: &service.OperationSpec{
					Summary:     "Start quest",
					Description: "Transitions a claimed quest to in_progress. No request body required.",
					Tags:        []string{"Quest Lifecycle"},
					Parameters:  []service.ParameterSpec{questIDParam},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Quest started, status changed to 'in_progress'", ContentType: "application/json"},
						"404": {Description: "Quest not found"},
						"409": {Description: "Quest not in 'claimed' status"},
					},
				},
			},
			"/quests/{id}/submit": {
				POST: &service.OperationSpec{
					Summary: "Submit quest result",
					Description: "Submits work output for a quest. If the quest requires review " +
						"(require_review constraint), status transitions to 'in_review'. " +
						"Otherwise transitions directly to 'completed'.\n\n" +
						"Request body: {output (required)}",
					Tags:       []string{"Quest Lifecycle"},
					Parameters: []service.ParameterSpec{questIDParam},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Result submitted, status changed to 'in_review' or 'completed'", ContentType: "application/json"},
						"400": {Description: "Missing output"},
						"404": {Description: "Quest not found"},
						"409": {Description: "Quest not in 'in_progress' status"},
					},
				},
			},
			"/quests/{id}/complete": {
				POST: &service.OperationSpec{
					Summary:     "Complete quest",
					Description: "Marks a quest as completed and releases the claiming agent. No request body required.",
					Tags:        []string{"Quest Lifecycle"},
					Parameters:  []service.ParameterSpec{questIDParam},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Quest completed, agent released", ContentType: "application/json"},
						"404": {Description: "Quest not found"},
						"409": {Description: "Quest not in 'in_review' or 'in_progress' status"},
					},
				},
			},
			"/quests/{id}/fail": {
				POST: &service.OperationSpec{
					Summary: "Fail quest",
					Description: "Records a quest failure. If attempts remain (< max_attempts), " +
						"the quest is reposted to the board. Otherwise it is marked as permanently failed. " +
						"The claiming agent is released in either case.\n\n" +
						"Request body: {reason? (string)}",
					Tags:       []string{"Quest Lifecycle"},
					Parameters: []service.ParameterSpec{questIDParam},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Quest failed or reposted, agent released", ContentType: "application/json"},
						"404": {Description: "Quest not found"},
						"409": {Description: "Quest not in 'in_progress' or 'in_review' status"},
					},
				},
			},
			"/quests/{id}/abandon": {
				POST: &service.OperationSpec{
					Summary: "Abandon quest",
					Description: "Agent abandons a quest, returning it to the board as 'posted'. " +
						"The claiming agent is released.\n\n" +
						"Request body: {reason? (string)}",
					Tags:       []string{"Quest Lifecycle"},
					Parameters: []service.ParameterSpec{questIDParam},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Quest returned to board, agent released", ContentType: "application/json"},
						"404": {Description: "Quest not found"},
						"409": {Description: "Quest not in 'claimed' or 'in_progress' status"},
					},
				},
			},

			// ── Agents ───────────────────────────────────────────
			"/agents": {
				GET: &service.OperationSpec{
					Summary:     "List agents",
					Description: "Returns all registered agents with their current status, level, skills, and stats.",
					Tags:        []string{"Agents"},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "List of agents", ContentType: "application/json"},
					},
				},
				POST: &service.OperationSpec{
					Summary: "Recruit agent",
					Description: "Recruits a new agent into the game. Agents start at level 1 (Apprentice tier).\n\n" +
						"Request body: {name (required), display_name?, persona?, skills? [skill_tag...], " +
						"config? {model?, provider?, temperature?, max_tokens?}}",
					Tags: []string{"Agents"},
					Responses: map[string]service.ResponseSpec{
						"201": {Description: "Agent recruited", ContentType: "application/json"},
						"400": {Description: "Invalid request or missing name"},
						"409": {Description: "Agent with this name already exists"},
					},
				},
			},
			"/agents/{id}": {
				GET: &service.OperationSpec{
					Summary:     "Get agent",
					Description: "Returns a single agent by ID including level, XP, skills, stats, inventory, and active effects.",
					Tags:        []string{"Agents"},
					Parameters:  []service.ParameterSpec{agentIDParam},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Agent details", ContentType: "application/json"},
						"404": {Description: "Agent not found"},
					},
				},
			},
			"/agents/{id}/retire": {
				POST: &service.OperationSpec{
					Summary:     "Retire agent",
					Description: "Retires an agent from active duty. No request body required.",
					Tags:        []string{"Agents"},
					Parameters:  []service.ParameterSpec{agentIDParam},
					Responses: map[string]service.ResponseSpec{
						"204": {Description: "Agent retired"},
						"404": {Description: "Agent not found"},
					},
				},
			},
			"/agents/{id}/reviews": {
				GET: &service.OperationSpec{
					Summary:     "List agent reviews",
					Description: "Returns all peer reviews where the agent is either the leader or member. Reviews are blind-masked until completed.",
					Tags:        []string{"Peer Reviews"},
					Parameters:  []service.ParameterSpec{agentIDParam},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "List of peer reviews involving this agent", ContentType: "application/json"},
					},
				},
			},

			// ── Store & Inventory ────────────────────────────────
			"/agents/{id}/inventory": {
				GET: &service.OperationSpec{
					Summary:     "Get inventory",
					Description: "Returns an agent's inventory of owned tools and consumables.",
					Tags:        []string{"Store"},
					Parameters:  []service.ParameterSpec{agentIDParam},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Agent inventory", ContentType: "application/json"},
					},
				},
			},
			"/agents/{id}/inventory/use": {
				POST: &service.OperationSpec{
					Summary: "Use consumable",
					Description: "Uses a consumable item from the agent's inventory, applying its effect.\n\n" +
						"Request body: {item_id (required)}",
					Tags:       []string{"Store"},
					Parameters: []service.ParameterSpec{agentIDParam},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Consumable used, effect applied", ContentType: "application/json"},
						"400": {Description: "Missing item_id or item not consumable"},
						"404": {Description: "Agent or item not found"},
						"503": {Description: "Store component unavailable"},
					},
				},
			},
			"/agents/{id}/effects": {
				GET: &service.OperationSpec{
					Summary:     "Get active effects",
					Description: "Returns active consumable effects for an agent, including remaining duration.",
					Tags:        []string{"Store"},
					Parameters:  []service.ParameterSpec{agentIDParam},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Active effects", ContentType: "application/json"},
					},
				},
			},
			"/store": {
				GET: &service.OperationSpec{
					Summary:     "List store items",
					Description: "Returns available items in the agent store. Optionally personalized by agent level and guild.",
					Tags:        []string{"Store"},
					Parameters: []service.ParameterSpec{
						{Name: "agent_id", In: "query", Description: "Agent ID for personalized catalog (filters by tier eligibility)", Schema: service.Schema{Type: "string"}},
					},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Store catalog", ContentType: "application/json"},
						"503": {Description: "Store component unavailable"},
					},
				},
			},
			"/store/{id}": {
				GET: &service.OperationSpec{
					Summary:     "Get store item",
					Description: "Returns a single store item by ID with pricing, effects, and tier requirements.",
					Tags:        []string{"Store"},
					Parameters: []service.ParameterSpec{
						{Name: "id", In: "path", Required: true, Description: "Item ID", Schema: service.Schema{Type: "string"}},
					},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Store item details", ContentType: "application/json"},
						"404": {Description: "Item not found"},
						"503": {Description: "Store component unavailable"},
					},
				},
			},
			"/store/purchase": {
				POST: &service.OperationSpec{
					Summary: "Purchase item",
					Description: "Purchases an item from the store for an agent using XP.\n\n" +
						"Request body: {agent_id (required), item_id (required)}",
					Tags: []string{"Store"},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Purchase result with updated XP balance", ContentType: "application/json"},
						"400": {Description: "Missing fields or insufficient XP"},
						"404": {Description: "Agent or item not found"},
						"503": {Description: "Store component unavailable"},
					},
				},
			},

			// ── Battles ──────────────────────────────────────────
			"/battles": {
				GET: &service.OperationSpec{
					Summary:     "List battles",
					Description: "Returns all boss battles (automated review evaluations).",
					Tags:        []string{"Battles"},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "List of battles", ContentType: "application/json"},
					},
				},
			},
			"/battles/{id}": {
				GET: &service.OperationSpec{
					Summary:     "Get battle",
					Description: "Returns a single boss battle by ID including verdict, scores, and linked quest.",
					Tags:        []string{"Battles"},
					Parameters: []service.ParameterSpec{
						{Name: "id", In: "path", Required: true, Description: "Battle ID", Schema: service.Schema{Type: "string"}},
					},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Battle details", ContentType: "application/json"},
						"404": {Description: "Battle not found"},
					},
				},
			},

			// ── Peer Reviews ─────────────────────────────────────
			"/reviews": {
				GET: &service.OperationSpec{
					Summary:     "List reviews",
					Description: "Returns all peer reviews with optional filtering. Reviews are blind-masked: partial submissions are hidden until both parties have submitted.",
					Tags:        []string{"Peer Reviews"},
					Parameters: []service.ParameterSpec{
						{Name: "status", In: "query", Description: "Filter by review status (pending, partial, completed)", Schema: service.Schema{Type: "string"}},
						{Name: "quest_id", In: "query", Description: "Filter by quest ID", Schema: service.Schema{Type: "string"}},
					},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "List of peer reviews (blind-masked)", ContentType: "application/json"},
					},
				},
				POST: &service.OperationSpec{
					Summary: "Create review",
					Description: "Creates a new peer review for a quest. For party quests, leader and member " +
						"review each other. For solo quests, set is_solo_task=true and only the leader submits.\n\n" +
						"Request body: {quest_id (required), leader_id (required), member_id (required), " +
						"party_id?, is_solo_task? (bool)}",
					Tags: []string{"Peer Reviews"},
					Responses: map[string]service.ResponseSpec{
						"201": {Description: "Peer review created with status 'pending'", ContentType: "application/json"},
						"400": {Description: "Missing required fields or leader_id == member_id for non-solo review"},
					},
				},
			},
			"/reviews/{id}": {
				GET: &service.OperationSpec{
					Summary:     "Get review",
					Description: "Returns a single peer review by ID. Blind-masked: the other party's submission is hidden until the review is completed.",
					Tags:        []string{"Peer Reviews"},
					Parameters: []service.ParameterSpec{
						{Name: "id", In: "path", Required: true, Description: "Review ID", Schema: service.Schema{Type: "string"}},
					},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Peer review details (blind-masked)", ContentType: "application/json"},
						"404": {Description: "Review not found"},
					},
				},
			},
			"/reviews/{id}/submit": {
				POST: &service.OperationSpec{
					Summary: "Submit review",
					Description: "Submits ratings for a peer review. Each party submits independently. " +
						"Ratings are integers 1-5. If average rating < 3.0, an explanation is required. " +
						"When both parties have submitted (or leader for solo), the review is marked completed " +
						"and average ratings are computed.\n\n" +
						"Request body: {reviewer_id (required), ratings: {q1: 1-5, q2: 1-5, q3: 1-5} (required), " +
						"explanation? (required if avg < 3.0)}",
					Tags: []string{"Peer Reviews"},
					Parameters: []service.ParameterSpec{
						{Name: "id", In: "path", Required: true, Description: "Review ID", Schema: service.Schema{Type: "string"}},
					},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Review submission accepted (blind-masked response)", ContentType: "application/json"},
						"400": {Description: "Missing fields, invalid ratings, or missing explanation for low ratings"},
						"404": {Description: "Review not found"},
						"409": {Description: "Review already completed or party already submitted"},
					},
				},
			},

			// ── DM ───────────────────────────────────────────────
			"/dm/chat": {
				POST: &service.OperationSpec{
					Summary: "DM chat",
					Description: "Send a natural language message to the Dungeon Master. The DM can create quests, " +
						"recruit agents, and manage the game world based on the conversation. " +
						"Sessions persist across turns.\n\n" +
						"Request body: {message (required), session_id? (string, hex, reuse for multi-turn)}",
					Tags: []string{"DM"},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "DM response with optional actions taken", ContentType: "application/json"},
						"400": {Description: "Missing message"},
						"503": {Description: "Model registry unavailable"},
					},
				},
			},
			"/dm/sessions/{id}": {
				GET: &service.OperationSpec{
					Summary:     "Get DM session",
					Description: "Returns the conversation history for a DM chat session including all turns with timestamps and trace IDs.",
					Tags:        []string{"DM"},
					Parameters: []service.ParameterSpec{
						{Name: "id", In: "path", Required: true, Description: "Session ID (hex string)", Schema: service.Schema{Type: "string"}},
					},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "DM chat session with turn history", ContentType: "application/json"},
						"400": {Description: "Invalid session ID format"},
						"404": {Description: "Session not found"},
					},
				},
			},
			"/dm/intervene/{questId}": {
				POST: &service.OperationSpec{
					Summary:     "DM intervene",
					Description: "DM intervenes on an active quest (not yet implemented).",
					Tags:        []string{"DM"},
					Parameters: []service.ParameterSpec{
						{Name: "questId", In: "path", Required: true, Description: "Quest ID", Schema: service.Schema{Type: "string"}},
					},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Intervention applied"},
						"501": {Description: "Not yet implemented"},
					},
				},
			},

			// ── Observability ────────────────────────────────────
			"/trajectories/{id}": {
				GET: &service.OperationSpec{
					Summary:     "Get trajectory",
					Description: "Returns trajectory events for a quest (not yet implemented).",
					Tags:        []string{"Observability"},
					Parameters: []service.ParameterSpec{
						{Name: "id", In: "path", Required: true, Description: "Trajectory ID", Schema: service.Schema{Type: "string"}},
					},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Trajectory events", ContentType: "application/json"},
						"501": {Description: "Not yet implemented"},
					},
				},
			},
		},
		Tags: []service.TagSpec{
			{Name: "World", Description: "Game world state"},
			{Name: "Quests", Description: "Quest board operations"},
			{Name: "Quest Lifecycle", Description: "Quest state transitions (claim, start, submit, complete, fail, abandon)"},
			{Name: "Agents", Description: "Agent management"},
			{Name: "Peer Reviews", Description: "Blind peer review between party members"},
			{Name: "Battles", Description: "Boss battle (automated review) operations"},
			{Name: "DM", Description: "Dungeon Master interaction and chat sessions"},
			{Name: "Store", Description: "Agent store, inventory, and consumable effects"},
			{Name: "Observability", Description: "Trajectory and event tracing"},
		},
	}
}
