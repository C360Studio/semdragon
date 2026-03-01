package api

import (
	"github.com/c360studio/semstreams/service"
)

func init() {
	service.RegisterOpenAPISpec("semdragons-api", semdragonsOpenAPISpec())
}

// semdragonsOpenAPISpec returns the OpenAPI specification for domain endpoints.
func semdragonsOpenAPISpec() *service.OpenAPISpec {
	return &service.OpenAPISpec{
		Paths: map[string]service.PathSpec{
			"/world": {
				GET: &service.OperationSpec{
					Summary:     "Get world state",
					Description: "Returns the complete game world state including agents, quests, parties, guilds, battles, and stats",
					Tags:        []string{"World"},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "World state snapshot", ContentType: "application/json"},
					},
				},
			},
			"/quests": {
				GET: &service.OperationSpec{
					Summary:     "List quests",
					Description: "Returns all quests with optional filtering by status, difficulty, or guild",
					Tags:        []string{"Quests"},
					Parameters: []service.ParameterSpec{
						{Name: "status", In: "query", Description: "Filter by quest status", Schema: service.Schema{Type: "string"}},
						{Name: "difficulty", In: "query", Description: "Filter by difficulty level", Schema: service.Schema{Type: "integer"}},
						{Name: "guild_id", In: "query", Description: "Filter by guild priority", Schema: service.Schema{Type: "string"}},
					},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "List of quests", ContentType: "application/json"},
					},
				},
				POST: &service.OperationSpec{
					Summary:     "Create quest",
					Description: "Posts a new quest to the quest board",
					Tags:        []string{"Quests"},
					Responses: map[string]service.ResponseSpec{
						"201": {Description: "Quest created", ContentType: "application/json"},
						"400": {Description: "Invalid request"},
					},
				},
			},
			"/quests/{id}": {
				GET: &service.OperationSpec{
					Summary:     "Get quest",
					Description: "Returns a single quest by ID",
					Tags:        []string{"Quests"},
					Parameters: []service.ParameterSpec{
						{Name: "id", In: "path", Required: true, Description: "Quest ID", Schema: service.Schema{Type: "string"}},
					},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Quest details", ContentType: "application/json"},
						"404": {Description: "Quest not found"},
					},
				},
			},
			"/agents": {
				GET: &service.OperationSpec{
					Summary:     "List agents",
					Description: "Returns all agents",
					Tags:        []string{"Agents"},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "List of agents", ContentType: "application/json"},
					},
				},
				POST: &service.OperationSpec{
					Summary:     "Recruit agent",
					Description: "Recruits a new agent into the game",
					Tags:        []string{"Agents"},
					Responses: map[string]service.ResponseSpec{
						"201": {Description: "Agent recruited", ContentType: "application/json"},
						"400": {Description: "Invalid request"},
					},
				},
			},
			"/agents/{id}": {
				GET: &service.OperationSpec{
					Summary:     "Get agent",
					Description: "Returns a single agent by ID",
					Tags:        []string{"Agents"},
					Parameters: []service.ParameterSpec{
						{Name: "id", In: "path", Required: true, Description: "Agent ID", Schema: service.Schema{Type: "string"}},
					},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Agent details", ContentType: "application/json"},
						"404": {Description: "Agent not found"},
					},
				},
			},
			"/agents/{id}/retire": {
				POST: &service.OperationSpec{
					Summary:     "Retire agent",
					Description: "Retires an agent from active duty",
					Tags:        []string{"Agents"},
					Parameters: []service.ParameterSpec{
						{Name: "id", In: "path", Required: true, Description: "Agent ID", Schema: service.Schema{Type: "string"}},
					},
					Responses: map[string]service.ResponseSpec{
						"204": {Description: "Agent retired"},
						"404": {Description: "Agent not found"},
					},
				},
			},
			"/agents/{agentId}/inventory": {
				GET: &service.OperationSpec{
					Summary:     "Get inventory",
					Description: "Returns an agent's inventory",
					Tags:        []string{"Store"},
					Parameters: []service.ParameterSpec{
						{Name: "agentId", In: "path", Required: true, Description: "Agent ID", Schema: service.Schema{Type: "string"}},
					},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Agent inventory", ContentType: "application/json"},
					},
				},
			},
			"/agents/{agentId}/inventory/use": {
				POST: &service.OperationSpec{
					Summary:     "Use consumable",
					Description: "Uses a consumable from the agent's inventory",
					Tags:        []string{"Store"},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Consumable used", ContentType: "application/json"},
					},
				},
			},
			"/agents/{agentId}/effects": {
				GET: &service.OperationSpec{
					Summary:     "Get active effects",
					Description: "Returns active consumable effects for an agent",
					Tags:        []string{"Store"},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Active effects", ContentType: "application/json"},
					},
				},
			},
			"/battles": {
				GET: &service.OperationSpec{
					Summary:     "List battles",
					Description: "Returns all boss battles",
					Tags:        []string{"Battles"},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "List of battles", ContentType: "application/json"},
					},
				},
			},
			"/battles/{id}": {
				GET: &service.OperationSpec{
					Summary:     "Get battle",
					Description: "Returns a single boss battle by ID",
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
			"/trajectories/{id}": {
				GET: &service.OperationSpec{
					Summary:     "Get trajectory",
					Description: "Returns trajectory events for a quest",
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
			"/dm/chat": {
				POST: &service.OperationSpec{
					Summary:     "DM chat",
					Description: "Send a message to the Dungeon Master",
					Tags:        []string{"DM"},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "DM response", ContentType: "application/json"},
						"501": {Description: "Not yet implemented"},
					},
				},
			},
			"/dm/intervene/{questId}": {
				POST: &service.OperationSpec{
					Summary:     "DM intervene",
					Description: "DM intervenes on an active quest",
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
			"/store": {
				GET: &service.OperationSpec{
					Summary:     "List store items",
					Description: "Returns available items in the agent store",
					Tags:        []string{"Store"},
					Parameters: []service.ParameterSpec{
						{Name: "agent_id", In: "query", Description: "Agent ID for personalized catalog", Schema: service.Schema{Type: "string"}},
					},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Store catalog", ContentType: "application/json"},
					},
				},
			},
			"/store/{id}": {
				GET: &service.OperationSpec{
					Summary:     "Get store item",
					Description: "Returns a single store item by ID",
					Tags:        []string{"Store"},
					Parameters: []service.ParameterSpec{
						{Name: "id", In: "path", Required: true, Description: "Item ID", Schema: service.Schema{Type: "string"}},
					},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Store item details", ContentType: "application/json"},
						"404": {Description: "Item not found"},
					},
				},
			},
			"/store/purchase": {
				POST: &service.OperationSpec{
					Summary:     "Purchase item",
					Description: "Purchases an item from the store for an agent",
					Tags:        []string{"Store"},
					Responses: map[string]service.ResponseSpec{
						"200": {Description: "Purchase result", ContentType: "application/json"},
						"400": {Description: "Invalid request"},
					},
				},
			},
		},
		Tags: []service.TagSpec{
			{Name: "World", Description: "Game world state"},
			{Name: "Quests", Description: "Quest board operations"},
			{Name: "Agents", Description: "Agent management"},
			{Name: "Battles", Description: "Boss battle operations"},
			{Name: "DM", Description: "Dungeon Master interaction"},
			{Name: "Store", Description: "Agent store and inventory"},
			{Name: "Observability", Description: "Trajectory and event tracing"},
		},
	}
}
