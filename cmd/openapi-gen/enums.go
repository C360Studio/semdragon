package main

// applyEnumOverrides injects enum values into generated schemas.
// SchemaFromType reflects Go string-typed enums as {"type": "string"} with no
// enum constraint. This function patches known domain enums so that
// openapi-typescript generates string literal unions instead of plain strings.
func applyEnumOverrides(schemas map[string]any) {
	for typeName, values := range enumValues {
		schema, ok := schemas[typeName]
		if !ok {
			continue
		}
		m, ok := schema.(map[string]any)
		if !ok {
			continue
		}
		// Patch top-level enum fields (e.g., Quest.Status is a string field).
		m["enum"] = values
	}

	// Patch enum fields that are embedded inside object schemas.
	for typeName, fieldOverrides := range enumFieldOverrides {
		schema, ok := schemas[typeName]
		if !ok {
			continue
		}
		m, ok := schema.(map[string]any)
		if !ok {
			continue
		}
		props, ok := m["properties"].(map[string]any)
		if !ok {
			continue
		}
		for fieldName, values := range fieldOverrides {
			prop, ok := props[fieldName].(map[string]any)
			if !ok {
				continue
			}
			prop["enum"] = values
		}
	}
}

// enumValues maps top-level schema names (standalone enum types) to their
// allowed string values. These types appear as fields in entity schemas.
var enumValues = map[string][]any{
	// Domain enums used as field types — these only need field-level overrides
	// since they don't have their own top-level schemas.
}

// enumFieldOverrides maps schema name → field name → allowed values.
// This patches enum fields inside object schemas.
var enumFieldOverrides = map[string]map[string][]any{
	// Quest
	"Quest": {
		"status":       {"posted", "claimed", "in_progress", "in_review", "completed", "failed", "escalated", "cancelled"},
		"difficulty":   {0, 1, 2, 3, 4, 5},
		"min_tier":     {0, 1, 2, 3, 4},
		"failure_type": {"quality", "timeout", "error", "abandoned"},
	},
	"QuestConstraints": {
		"review_level": {0, 1, 2, 3},
	},

	// Agent
	"Agent": {
		"status": {"idle", "on_quest", "in_battle", "cooldown", "retired", "pending_review"},
		"tier":   {0, 1, 2, 3, 4},
	},

	// BossBattle
	"BossBattle": {
		"status": {"active", "victory", "defeat", "retreat"},
		"level":  {0, 1, 2, 3},
	},
	"Judge": {
		"type": {"automated", "llm", "human"},
	},

	// Party
	"Party": {
		"status":   {"forming", "active", "disbanded"},
		"strategy": {"balanced", "specialist", "mentor", "minimal"},
	},
	"PartyMember": {
		"role": {"lead", "executor", "reviewer", "scout"},
	},

	// Guild
	"Guild": {
		"status": {"active", "inactive"},
	},
	"GuildMember": {
		"rank": {"initiate", "member", "veteran", "officer", "guildmaster"},
	},

	// Peer Review
	"PeerReview": {
		"status": {"pending", "partial", "completed"},
	},
	"ReviewSubmission": {
		"direction": {"leader_to_member", "member_to_leader", "dm_to_agent"},
	},

	// Store
	"StoreItem": {
		"item_type":     {"tool", "consumable"},
		"purchase_type": {"permanent", "rental"},
	},
	"ConsumableEffect": {
		"type": {"retry_token", "cooldown_skip", "xp_boost", "quality_shield", "insight_scroll"},
	},

	// DM
	"DMChatContextRef": {
		"type": {"agent", "quest", "battle", "guild"},
	},
	"DMChatHistoryItem": {
		"role": {"user", "dm"},
	},
}
