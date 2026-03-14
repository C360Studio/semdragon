package api

import (
	"reflect"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semdragons/processor/agentstore"
	"github.com/c360studio/semdragons/processor/bossbattle"
	"github.com/c360studio/semdragons/processor/partycoord"
	"github.com/c360studio/semdragons/processor/tokenbudget"
	"github.com/c360studio/semdragons/service/agentsheet"
	"github.com/c360studio/semstreams/agentic"
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
						"200": {Description: "World state snapshot", ContentType: "application/json", SchemaRef: "#/components/schemas/WorldStateResponse"},
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
						"200": {Description: "List of quests", ContentType: "application/json", SchemaRef: "#/components/schemas/Quest", IsArray: true},
					},
				},
				POST: &service.OperationSpec{
					Summary:     "Create quest",
					Description: "Posts a new quest to the quest board. Requires title and goal. Scenarios are optional but recommended — their dependency graph drives automatic party vs solo routing.",
					Tags:        []string{"Quests"},
					RequestBody: &service.RequestBodySpec{
						Description: "Quest creation parameters",
						SchemaRef:   "#/components/schemas/CreateQuestRequest",
						Required:    true,
					},
					Responses: map[string]service.ResponseSpec{
						"201": {Description: "Quest created", ContentType: "application/json", SchemaRef: "#/components/schemas/Quest"},
						"400": {Description: "Invalid request body, missing title or goal, or invalid scenario dependencies"},
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
						"200": {Description: "Quest details", ContentType: "application/json", SchemaRef: "#/components/schemas/Quest"},
						"404": {Description: "Quest not found"},
					},
				},
			},
			"/quests/chain": {
				POST: &service.OperationSpec{
					Summary:     "Create quest chain",
					Description: "Posts multiple linked quests in a single request. Dependencies use 0-based indices into the quests array, which are resolved to actual quest IDs.",
					Tags:        []string{"Quests"},
					RequestBody: &service.RequestBodySpec{
						Description: "Quest chain with interdependencies",
						SchemaRef:   "#/components/schemas/QuestChainBrief",
						Required:    true,
					},
					Responses: map[string]service.ResponseSpec{
						"201": {Description: "Array of created quests with resolved dependency IDs", ContentType: "application/json", SchemaRef: "#/components/schemas/Quest", IsArray: true},
						"400": {Description: "Invalid request, dependency cycle, or out-of-bounds index"},
					},
				},
			},

			// ── Quest Lifecycle ──────────────────────────────────
			"/quests/{id}/claim": {
				POST: &service.OperationSpec{
					Summary:     "Claim quest",
					Description: "Agent claims a posted quest. Validates that the agent meets tier and skill requirements.",
					Tags:        []string{"Quest Lifecycle"},
					Parameters:  []service.ParameterSpec{questIDParam},
					RequestBody: &service.RequestBodySpec{
						Description: "Agent claiming the quest",
						SchemaRef:   "#/components/schemas/ClaimQuestRequest",
						Required:    true,
					},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Quest claimed, status changed to 'claimed'", ContentType: "application/json", SchemaRef: "#/components/schemas/Quest"},
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
						"200": {Description: "Quest started, status changed to 'in_progress'", ContentType: "application/json", SchemaRef: "#/components/schemas/Quest"},
						"404": {Description: "Quest not found"},
						"409": {Description: "Quest not in 'claimed' status"},
					},
				},
			},
			"/quests/{id}/submit": {
				POST: &service.OperationSpec{
					Summary:     "Submit quest result",
					Description: "Submits work output for a quest. If the quest requires review (require_review constraint), status transitions to 'in_review'. Otherwise transitions directly to 'completed'.",
					Tags:        []string{"Quest Lifecycle"},
					Parameters:  []service.ParameterSpec{questIDParam},
					RequestBody: &service.RequestBodySpec{
						Description: "Quest result output",
						SchemaRef:   "#/components/schemas/SubmitQuestRequest",
						Required:    true,
					},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Result submitted, status changed to 'in_review' or 'completed'", ContentType: "application/json", SchemaRef: "#/components/schemas/Quest"},
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
						"200": {Description: "Quest completed, agent released", ContentType: "application/json", SchemaRef: "#/components/schemas/Quest"},
						"404": {Description: "Quest not found"},
						"409": {Description: "Quest not in 'in_review' or 'in_progress' status"},
					},
				},
			},
			"/quests/{id}/fail": {
				POST: &service.OperationSpec{
					Summary:     "Fail quest",
					Description: "Records a quest failure. If attempts remain (< max_attempts), the quest is reposted to the board. Otherwise it is marked as permanently failed. The claiming agent is released in either case.",
					Tags:        []string{"Quest Lifecycle"},
					Parameters:  []service.ParameterSpec{questIDParam},
					RequestBody: &service.RequestBodySpec{
						Description: "Failure reason",
						SchemaRef:   "#/components/schemas/FailQuestRequest",
					},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Quest failed or reposted, agent released", ContentType: "application/json", SchemaRef: "#/components/schemas/Quest"},
						"404": {Description: "Quest not found"},
						"409": {Description: "Quest not in 'in_progress' or 'in_review' status"},
					},
				},
			},
			"/quests/{id}/abandon": {
				POST: &service.OperationSpec{
					Summary:     "Abandon quest",
					Description: "Agent abandons a quest, returning it to the board as 'posted'. The claiming agent is released.",
					Tags:        []string{"Quest Lifecycle"},
					Parameters:  []service.ParameterSpec{questIDParam},
					RequestBody: &service.RequestBodySpec{
						Description: "Abandonment reason",
						SchemaRef:   "#/components/schemas/AbandonQuestRequest",
					},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Quest returned to board, agent released", ContentType: "application/json", SchemaRef: "#/components/schemas/Quest"},
						"404": {Description: "Quest not found"},
						"409": {Description: "Quest not in 'claimed' or 'in_progress' status"},
					},
				},
			},
			"/quests/{id}/cancel": {
				POST: &service.OperationSpec{
					Summary:     "Cancel quest",
					Description: "Cancels an in-progress quest by sending a cancel signal to the active agentic loop. The quest transitions to 'failed' and the agent is released. For DAG parent quests, all active sub-quest loops are also cancelled.",
					Tags:        []string{"Quest Lifecycle"},
					Parameters:  []service.ParameterSpec{questIDParam},
					RequestBody: &service.RequestBodySpec{
						Description: "Cancellation reason (optional)",
						SchemaRef:   "#/components/schemas/CancelQuestRequest",
					},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Quest cancelled and agent released", ContentType: "application/json", SchemaRef: "#/components/schemas/Quest"},
						"404": {Description: "Quest not found"},
						"409": {Description: "Quest not in 'in_progress' status"},
					},
				},
			},

			// ── Quest Artifacts ──────────────────────────────────
			"/quests/{id}/artifacts": {
				GET: &service.OperationSpec{
					Summary:     "Download quest artifacts",
					Description: "Downloads all artifact files for a quest as a zip archive. Includes a manifest.json with quest metadata at the root of the archive.",
					Tags:        []string{"Quest Artifacts"},
					Parameters:  []service.ParameterSpec{questIDParam},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Zip archive of quest artifacts", ContentType: "application/zip"},
						"404": {Description: "Quest or artifacts not found"},
						"503": {Description: "Artifact storage not available"},
					},
				},
			},
			"/quests/{id}/artifacts/list": {
				GET: &service.OperationSpec{
					Summary:     "List quest artifact files",
					Description: "Returns a JSON list of artifact file paths for a quest.",
					Tags:        []string{"Quest Artifacts"},
					Parameters:  []service.ParameterSpec{questIDParam},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "List of artifact files", ContentType: "application/json"},
						"503": {Description: "Artifact storage not available"},
					},
				},
			},
			"/quests/{id}/artifacts/{path}": {
				GET: &service.OperationSpec{
					Summary:     "Get single artifact file",
					Description: "Serves a single artifact file by path within the quest's artifact storage.",
					Tags:        []string{"Quest Artifacts"},
					Parameters: []service.ParameterSpec{
						questIDParam,
						{Name: "path", In: "path", Required: true, Description: "File path within quest artifacts", Schema: service.Schema{Type: "string"}},
					},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Artifact file content"},
						"404": {Description: "Artifact not found"},
						"503": {Description: "Artifact storage not available"},
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
						"200": {Description: "List of agents", ContentType: "application/json", SchemaRef: "#/components/schemas/Agent", IsArray: true},
					},
				},
				POST: &service.OperationSpec{
					Summary:     "Recruit agent",
					Description: "Recruits a new agent into the game. Agents start at level 1 (Apprentice tier) unless an initial level is specified.",
					Tags:        []string{"Agents"},
					RequestBody: &service.RequestBodySpec{
						Description: "Agent recruitment parameters",
						SchemaRef:   "#/components/schemas/RecruitAgentRequest",
						Required:    true,
					},
					Responses: map[string]service.ResponseSpec{
						"201": {Description: "Agent recruited", ContentType: "application/json", SchemaRef: "#/components/schemas/Agent"},
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
						"200": {Description: "Agent details", ContentType: "application/json", SchemaRef: "#/components/schemas/Agent"},
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
						"200": {Description: "List of peer reviews involving this agent", ContentType: "application/json", SchemaRef: "#/components/schemas/PeerReview", IsArray: true},
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
						"200": {Description: "Agent inventory", ContentType: "application/json", SchemaRef: "#/components/schemas/AgentInventory"},
					},
				},
			},
			"/agents/{id}/inventory/use": {
				POST: &service.OperationSpec{
					Summary:     "Use consumable",
					Description: "Uses a consumable item from the agent's inventory, applying its effect.",
					Tags:        []string{"Store"},
					Parameters:  []service.ParameterSpec{agentIDParam},
					RequestBody: &service.RequestBodySpec{
						Description: "Consumable to use",
						SchemaRef:   "#/components/schemas/UseConsumableRequest",
						Required:    true,
					},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Consumable used, effect applied", ContentType: "application/json", SchemaRef: "#/components/schemas/UseConsumableResponse"},
						"400": {Description: "Missing consumable_id or item not consumable"},
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
						"200": {Description: "Active effects", ContentType: "application/json", SchemaRef: "#/components/schemas/ActiveEffect", IsArray: true},
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
						"200": {Description: "Store catalog", ContentType: "application/json", SchemaRef: "#/components/schemas/StoreItem", IsArray: true},
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
						"200": {Description: "Store item details", ContentType: "application/json", SchemaRef: "#/components/schemas/StoreItem"},
						"404": {Description: "Item not found"},
						"503": {Description: "Store component unavailable"},
					},
				},
			},
			"/store/purchase": {
				POST: &service.OperationSpec{
					Summary:     "Purchase item",
					Description: "Purchases an item from the store for an agent using XP.",
					Tags:        []string{"Store"},
					RequestBody: &service.RequestBodySpec{
						Description: "Purchase parameters",
						SchemaRef:   "#/components/schemas/PurchaseItemRequest",
						Required:    true,
					},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Purchase result with updated XP balance", ContentType: "application/json", SchemaRef: "#/components/schemas/PurchaseResponse"},
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
						"200": {Description: "List of battles", ContentType: "application/json", SchemaRef: "#/components/schemas/BossBattle", IsArray: true},
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
						"200": {Description: "Battle details", ContentType: "application/json", SchemaRef: "#/components/schemas/BossBattle"},
						"404": {Description: "Battle not found"},
					},
				},
			},

			// ── Parties ──────────────────────────────────────────
			"/parties": {
				GET: &service.OperationSpec{
					Summary:     "List parties",
					Description: "Returns all parties including members, quest assignments, and formation status.",
					Tags:        []string{"Parties"},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "List of parties", ContentType: "application/json", SchemaRef: "#/components/schemas/Party", IsArray: true},
					},
				},
			},
			"/parties/{id}": {
				GET: &service.OperationSpec{
					Summary:     "Get party",
					Description: "Returns a single party by ID including members, roles, and quest assignment.",
					Tags:        []string{"Parties"},
					Parameters: []service.ParameterSpec{
						{Name: "id", In: "path", Required: true, Description: "Party ID", Schema: service.Schema{Type: "string"}},
					},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Party details", ContentType: "application/json", SchemaRef: "#/components/schemas/Party"},
						"404": {Description: "Party not found"},
					},
				},
			},

			// ── Guilds ───────────────────────────────────────────
			"/guilds": {
				GET: &service.OperationSpec{
					Summary:     "List guilds",
					Description: "Returns all guilds including members, founder, and specialization.",
					Tags:        []string{"Guilds"},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "List of guilds", ContentType: "application/json", SchemaRef: "#/components/schemas/Guild", IsArray: true},
					},
				},
			},
			"/guilds/{id}": {
				GET: &service.OperationSpec{
					Summary:     "Get guild",
					Description: "Returns a single guild by ID including members, founder, and specialization.",
					Tags:        []string{"Guilds"},
					Parameters: []service.ParameterSpec{
						{Name: "id", In: "path", Required: true, Description: "Guild ID", Schema: service.Schema{Type: "string"}},
					},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Guild details", ContentType: "application/json", SchemaRef: "#/components/schemas/Guild"},
						"404": {Description: "Guild not found"},
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
						"200": {Description: "List of peer reviews (blind-masked)", ContentType: "application/json", SchemaRef: "#/components/schemas/PeerReview", IsArray: true},
					},
				},
				POST: &service.OperationSpec{
					Summary:     "Create review",
					Description: "Creates a new peer review for a quest. For party quests, leader and member review each other. For solo quests, set is_solo_task=true and only the leader submits.",
					Tags:        []string{"Peer Reviews"},
					RequestBody: &service.RequestBodySpec{
						Description: "Review creation parameters",
						SchemaRef:   "#/components/schemas/CreateReviewRequest",
						Required:    true,
					},
					Responses: map[string]service.ResponseSpec{
						"201": {Description: "Peer review created with status 'pending'", ContentType: "application/json", SchemaRef: "#/components/schemas/PeerReview"},
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
						"200": {Description: "Peer review details (blind-masked)", ContentType: "application/json", SchemaRef: "#/components/schemas/PeerReview"},
						"404": {Description: "Review not found"},
					},
				},
			},
			"/reviews/{id}/submit": {
				POST: &service.OperationSpec{
					Summary:     "Submit review",
					Description: "Submits ratings for a peer review. Each party submits independently. Ratings are integers 1-5. If average rating < 3.0, an explanation is required. When both parties have submitted (or leader for solo), the review is marked completed and average ratings are computed.",
					Tags:        []string{"Peer Reviews"},
					Parameters: []service.ParameterSpec{
						{Name: "id", In: "path", Required: true, Description: "Review ID", Schema: service.Schema{Type: "string"}},
					},
					RequestBody: &service.RequestBodySpec{
						Description: "Review ratings and optional explanation",
						SchemaRef:   "#/components/schemas/SubmitReviewRequest",
						Required:    true,
					},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Review submission accepted (blind-masked response)", ContentType: "application/json", SchemaRef: "#/components/schemas/PeerReview"},
						"400": {Description: "Missing fields, invalid ratings, or missing explanation for low ratings"},
						"404": {Description: "Review not found"},
						"409": {Description: "Review already completed or party already submitted"},
					},
				},
			},

			// ── Board Control ────────────────────────────────────
			"/board/status": {
				GET: &service.OperationSpec{
					Summary:     "Get board status",
					Description: "Returns the current board play/pause status. When paused, autonomous actions (heartbeats, quest dispatch, boid suggestions) are suspended while manual operations continue.",
					Tags:        []string{"Board Control"},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Board status", ContentType: "application/json", SchemaRef: "#/components/schemas/BoardStatusResponse"},
					},
				},
			},
			"/board/pause": {
				POST: &service.OperationSpec{
					Summary:     "Pause the board",
					Description: "Pauses autonomous processing. In-progress agentic loops complete gracefully. Manual operations continue. Idempotent — pausing an already-paused board returns the current state.",
					Tags:        []string{"Board Control"},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Board paused", ContentType: "application/json", SchemaRef: "#/components/schemas/BoardStatusResponse"},
						"503": {Description: "Board controller unavailable"},
					},
				},
			},
			"/board/resume": {
				POST: &service.OperationSpec{
					Summary:     "Resume the board",
					Description: "Resumes autonomous processing. Triggers reconciliation for any quests that transitioned to in_progress while paused. Idempotent.",
					Tags:        []string{"Board Control"},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Board resumed", ContentType: "application/json", SchemaRef: "#/components/schemas/BoardStatusResponse"},
						"503": {Description: "Board controller unavailable"},
					},
				},
			},

			// ── Token Budget ────────────────────────────────────
			"/board/tokens": {
				GET: &service.OperationSpec{
					Summary:     "Get token usage stats",
					Description: "Returns current token usage counters, hourly limit, budget percentage, and circuit breaker state. The breaker field is 'ok', 'warning' (>=80%), or 'tripped' (>=100%).",
					Tags:        []string{"Board Control"},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Token usage statistics", ContentType: "application/json", SchemaRef: "#/components/schemas/TokenStats"},
					},
				},
			},
			"/board/tokens/budget": {
				POST: &service.OperationSpec{
					Summary:     "Set token budget",
					Description: "Updates the global hourly token limit. Set to 0 for unlimited. Persisted to KV for restart recovery.",
					Tags:        []string{"Board Control"},
					RequestBody: &service.RequestBodySpec{
						Description: "New hourly token limit",
						SchemaRef:   "#/components/schemas/SetTokenBudgetRequest",
						Required:    true,
					},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Budget updated", ContentType: "application/json", SchemaRef: "#/components/schemas/TokenStats"},
						"400": {Description: "Invalid limit value"},
					},
				},
			},

			// ── Model Registry ───────────────────────────────────
			"/models": {
				GET: &service.OperationSpec{
					Summary:     "Get model registry",
					Description: "Returns model registry state. With ?resolve=capability, returns the resolution result for a single capability including endpoint name, model, and provider. Without it, returns a full summary of all endpoints and capabilities.",
					Tags:        []string{"Model Registry"},
					Parameters: []service.ParameterSpec{
						{Name: "resolve", In: "query", Description: "Capability key to resolve (e.g. 'agent-work.apprentice')", Schema: service.Schema{Type: "string"}},
					},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Model registry summary or resolution result", ContentType: "application/json"},
						"503": {Description: "Model registry unavailable"},
					},
				},
			},

			// ── DM ───────────────────────────────────────────────
			"/dm/chat": {
				POST: &service.OperationSpec{
					Summary:     "DM chat",
					Description: "Send a natural language message to the Dungeon Master. Supports two modes: 'converse' (default, Q&A) and 'quest' (create quests/chains with structured output). Sessions persist across turns.",
					Tags:        []string{"DM"},
					RequestBody: &service.RequestBodySpec{
						Description: "Chat message and optional context",
						SchemaRef:   "#/components/schemas/DMChatRequest",
						Required:    true,
					},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "DM response with optional actions taken", ContentType: "application/json", SchemaRef: "#/components/schemas/DMChatResponse"},
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
						"200": {Description: "DM chat session with turn history", ContentType: "application/json", SchemaRef: "#/components/schemas/DMChatSession"},
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
			"/dm/triage/{questId}": {
				POST: &service.OperationSpec{
					Summary:     "DM triage failed quest",
					Description: "Apply a DM triage decision to a quest in pending_triage status. Body: {path: salvage|tpk|escalate|terminal, analysis: string, salvaged_output?: any, anti_patterns?: string[]}. Salvage preserves partial work and retries. TPK clears output, adds anti-pattern warnings, retries. Escalate marks for human attention. Terminal marks permanently failed.",
					Tags:        []string{"DM"},
					Parameters: []service.ParameterSpec{
						{Name: "questId", In: "path", Required: true, Description: "Quest ID (must be in pending_triage status)", Schema: service.Schema{Type: "string"}},
					},
					RequestBody: &service.RequestBodySpec{
						Required:    true,
						ContentType: "application/json",
						SchemaRef:   "#/components/schemas/TriageDecision",
					},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Triage applied, returns updated quest", ContentType: "application/json", SchemaRef: "#/components/schemas/Quest"},
						"400": {Description: "Invalid request (bad path, missing analysis)"},
						"404": {Description: "Quest not found"},
						"409": {Description: "Quest not in pending_triage status"},
					},
				},
			},

			// ── Settings ────────────────────────────────────────
			"/settings": {
				GET: &service.OperationSpec{
					Summary:     "Get settings",
					Description: "Returns the current runtime configuration including platform identity, NATS status, LLM providers, capabilities, components, workspace, and token budget. API key values are never exposed.",
					Tags:        []string{"Settings"},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Current settings", ContentType: "application/json", SchemaRef: "#/components/schemas/SettingsResponse"},
					},
				},
				POST: &service.OperationSpec{
					Summary:     "Update settings",
					Description: "Mutates runtime-configurable settings (model registry, token budget). Auth required. Platform and NATS settings require a restart. Changes are persisted to disk and propagated via KV.",
					Tags:        []string{"Settings"},
					RequestBody: &service.RequestBodySpec{
						Description: "Settings updates (partial)",
						SchemaRef:   "#/components/schemas/UpdateSettingsRequest",
						Required:    true,
					},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Updated settings", ContentType: "application/json", SchemaRef: "#/components/schemas/SettingsResponse"},
						"400": {Description: "Validation error"},
					},
				},
			},
			"/settings/health": {
				GET: &service.OperationSpec{
					Summary:     "Get settings health",
					Description: "Runs live validation checks (NATS, LLM keys, workspace, streams, buckets) and returns an onboarding checklist showing prerequisite status.",
					Tags:        []string{"Settings"},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Health checks and onboarding checklist", ContentType: "application/json", SchemaRef: "#/components/schemas/HealthResponse"},
					},
				},
			},

			// ── SSE ─────────────────────────────────────────────
			"/events": {
				GET: &service.OperationSpec{
					Summary:     "Real-time entity updates",
					Description: "Server-Sent Events stream of entity state changes from the board KV bucket. Sends `event: connected` on connect, then `event: kv_change` for each entity create/update/delete. Includes `initial_sync_complete` operation after all existing entities are sent. Auto-reconnects via SSE `retry` directive.",
					Tags:        []string{"Observability"},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "SSE stream of KV change events", ContentType: "text/event-stream"},
					},
				},
			},

			// ── Observability ────────────────────────────────────
			"/trajectories/{id}": {
				GET: &service.OperationSpec{
					Summary:     "Get trajectory",
					Description: "Returns the execution trajectory for an agentic loop, including all model calls, tool calls, token usage, and timing. The trajectory ID is the loop_id from a completed quest.",
					Tags:        []string{"Observability"},
					Parameters: []service.ParameterSpec{
						{Name: "id", In: "path", Required: true, Description: "Trajectory/loop ID (from quest.loop_id)", Schema: service.Schema{Type: "string"}},
					},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Trajectory with execution steps", ContentType: "application/json", SchemaRef: "#/components/schemas/Trajectory"},
						"400": {Description: "Invalid trajectory ID format"},
						"404": {Description: "Trajectory not found"},
						"503": {Description: "Trajectory service unavailable"},
					},
				},
			},
		},
		Tags: []service.TagSpec{
			{Name: "Settings", Description: "Runtime configuration, health checks, and onboarding"},
			{Name: "Model Registry", Description: "Model registry introspection and capability resolution"},
			{Name: "Board Control", Description: "Board play/pause control"},
			{Name: "World", Description: "Game world state"},
			{Name: "Quests", Description: "Quest board operations"},
			{Name: "Quest Lifecycle", Description: "Quest state transitions (claim, start, submit, complete, fail, abandon)"},
			{Name: "Quest Artifacts", Description: "Quest artifact file storage and retrieval"},
			{Name: "Agents", Description: "Agent management"},
			{Name: "Peer Reviews", Description: "Blind peer review between party members"},
			{Name: "Battles", Description: "Boss battle (automated review) operations"},
			{Name: "Parties", Description: "Party formation and management"},
			{Name: "Guilds", Description: "Guild formation and management"},
			{Name: "DM", Description: "Dungeon Master interaction and chat sessions"},
			{Name: "Store", Description: "Agent store, inventory, and consumable effects"},
			{Name: "Observability", Description: "Trajectory and event tracing"},
		},

		// Response types — the generator reflects these to build components.schemas
		ResponseTypes: []reflect.Type{
			// Domain entities
			reflect.TypeOf(domain.Quest{}),
			reflect.TypeOf(domain.QuestConstraints{}),
			reflect.TypeOf(domain.BattleVerdict{}),
			reflect.TypeOf(domain.Guild{}),
			reflect.TypeOf(domain.GuildMember{}),
			reflect.TypeOf(domain.PeerReview{}),
			reflect.TypeOf(domain.ReviewSubmission{}),
			reflect.TypeOf(domain.ReviewRatings{}),
			reflect.TypeOf(domain.WorldStats{}),

			// Processor entities
			reflect.TypeOf(agentprogression.Agent{}),
			reflect.TypeOf(agentprogression.AgentStats{}),
			reflect.TypeOf(agentprogression.AgentConfig{}),
			reflect.TypeOf(agentprogression.AgentPersona{}),
			reflect.TypeOf(bossbattle.BossBattle{}),
			reflect.TypeOf(domain.Judge{}),
			reflect.TypeOf(domain.ReviewCriterion{}),
			reflect.TypeOf(domain.ReviewResult{}),
			reflect.TypeOf(partycoord.Party{}),
			reflect.TypeOf(partycoord.PartyMember{}),

			// Store types
			reflect.TypeOf(agentstore.StoreItem{}),
			reflect.TypeOf(agentstore.OwnedItem{}),
			reflect.TypeOf(agentstore.AgentInventory{}),
			reflect.TypeOf(agentstore.ActiveEffect{}),
			reflect.TypeOf(agentstore.ConsumableEffect{}),

			// Trajectory types
			reflect.TypeOf(agentic.Trajectory{}),
			reflect.TypeOf(agentic.TrajectoryStep{}),

			// Character sheet
			reflect.TypeOf(agentsheet.CharacterSheet{}),
			reflect.TypeOf(agentsheet.SkillBar{}),
			reflect.TypeOf(agentsheet.DerivedStats{}),
			reflect.TypeOf(agentsheet.GuildMembership{}),
			reflect.TypeOf(agentsheet.EquippedItem{}),

			// DM types
			reflect.TypeOf(DMChatSession{}),
			reflect.TypeOf(DMChatTurn{}),
			reflect.TypeOf(domain.QuestBrief{}),
			reflect.TypeOf(domain.QuestChainBrief{}),
			reflect.TypeOf(domain.QuestChainEntry{}),
			reflect.TypeOf(domain.QuestHints{}),

			// API response types
			reflect.TypeOf(WorldStateResponse{}),
			reflect.TypeOf(DMChatResponse{}),
			reflect.TypeOf(TraceInfoResponse{}),
			reflect.TypeOf(PurchaseResponse{}),
			reflect.TypeOf(UseConsumableResponse{}),
			reflect.TypeOf(BoardStatusResponse{}),

			// Model registry types
			reflect.TypeOf(ModelResolveResponse{}),
			reflect.TypeOf(ModelEndpointSummary{}),
			reflect.TypeOf(ModelRegistrySummary{}),

			// Token budget types
			reflect.TypeOf(tokenbudget.TokenStats{}),
			reflect.TypeOf(tokenbudget.UsageSnapshot{}),

			// Settings types
			reflect.TypeOf(SettingsResponse{}),
			reflect.TypeOf(PlatformInfo{}),
			reflect.TypeOf(NATSInfo{}),
			reflect.TypeOf(ModelRegistryView{}),
			reflect.TypeOf(ModelEndpointView{}),
			reflect.TypeOf(CapabilityView{}),
			reflect.TypeOf(ModelDefaultsView{}),
			reflect.TypeOf(ComponentInfoView{}),
			reflect.TypeOf(WorkspaceInfoView{}),
			reflect.TypeOf(TokenBudgetView{}),
			reflect.TypeOf(HealthResponse{}),
			reflect.TypeOf(HealthCheck{}),
			reflect.TypeOf(ChecklistItem{}),
			reflect.TypeOf(WebsocketInputView{}),
		},

		// Request body types — the generator reflects these to build components.schemas
		RequestBodyTypes: []reflect.Type{
			reflect.TypeOf(CreateQuestRequest{}),
			reflect.TypeOf(CreateQuestHints{}),
			reflect.TypeOf(ClaimQuestRequest{}),
			reflect.TypeOf(SubmitQuestRequest{}),
			reflect.TypeOf(FailQuestRequest{}),
			reflect.TypeOf(AbandonQuestRequest{}),
			reflect.TypeOf(CancelQuestRequest{}),
			reflect.TypeOf(RecruitAgentRequest{}),
			reflect.TypeOf(PurchaseItemRequest{}),
			reflect.TypeOf(UseConsumableRequest{}),
			reflect.TypeOf(CreateReviewRequest{}),
			reflect.TypeOf(SubmitReviewRequest{}),
			reflect.TypeOf(DMChatRequest{}),
			reflect.TypeOf(DMChatContextRef{}),
			reflect.TypeOf(DMChatHistoryItem{}),
			reflect.TypeOf(SetTokenBudgetRequest{}),

			// Settings request types
			reflect.TypeOf(UpdateSettingsRequest{}),
			reflect.TypeOf(ModelRegistryUpdate{}),
			reflect.TypeOf(EndpointUpdate{}),
			reflect.TypeOf(CapabilityUpdate{}),
			reflect.TypeOf(WebsocketInputUpdate{}),
		},
	}
}
