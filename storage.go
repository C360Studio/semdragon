package semdragons

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/nats-io/nats.go/jetstream"
)

// =============================================================================
// STORAGE - Single KV bucket with dotted keys
// =============================================================================
// All state stored in one bucket with hierarchical dotted keys.
// This enables NATS wildcard queries since KV is streams under the hood.
//
// Key patterns:
//   quest.{instance}                    - Quest state
//   agent.{instance}                    - Agent state
//   party.{instance}                    - Party state
//   guild.{instance}                    - Guild state
//   battle.{instance}                   - Battle state
//   idx.quest.status.{status}.{id}     - Status index (presence-based)
//   idx.quest.agent.{agent}.{quest}    - Agent's quests index
//   idx.quest.guild.{guild}.{quest}    - Guild priority index
//   stats.board                         - Board statistics
// =============================================================================

// Storage provides access to the semdragons KV store.
type Storage struct {
	kv     *natsclient.KVStore
	config *BoardConfig
	logger *slog.Logger
}

// NewStorage creates storage with an existing KV store.
func NewStorage(kv *natsclient.KVStore, config *BoardConfig) *Storage {
	return &Storage{
		kv:     kv,
		config: config,
		logger: slog.Default(),
	}
}

// WithLogger sets a custom logger for the storage.
func (s *Storage) WithLogger(l *slog.Logger) *Storage {
	s.logger = l
	return s
}

// CreateStorage creates a new storage instance, creating the KV bucket if needed.
func CreateStorage(ctx context.Context, client *natsclient.Client, config *BoardConfig) (*Storage, error) {
	bucketName := config.BucketName()

	bucket, err := client.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{
		Bucket:      bucketName,
		Description: fmt.Sprintf("Semdragons quest board: %s", config.Board),
		History:     10,
		Storage:     jetstream.FileStorage,
	})
	if err != nil {
		return nil, errs.Wrap(err, "Storage", "CreateStorage", "create bucket")
	}

	kv := client.NewKVStore(bucket)
	return NewStorage(kv, config), nil
}

// KV returns the underlying KVStore for direct access if needed.
func (s *Storage) KV() *natsclient.KVStore {
	return s.kv
}

// Config returns the board configuration.
func (s *Storage) Config() *BoardConfig {
	return s.config
}

// --- Key Generation ---

// QuestKey returns the KV key for a quest's state.
func (s *Storage) QuestKey(instance string) string {
	return "quest." + instance
}

// AgentKey returns the KV key for an agent's state.
func (s *Storage) AgentKey(instance string) string {
	return "agent." + instance
}

// PartyKey returns the KV key for a party's state.
func (s *Storage) PartyKey(instance string) string {
	return "party." + instance
}

// GuildKey returns the KV key for a guild's state.
func (s *Storage) GuildKey(instance string) string {
	return "guild." + instance
}

// BattleKey returns the KV key for a battle's state.
func (s *Storage) BattleKey(instance string) string {
	return "battle." + instance
}

// StatusIndexKey returns the key for a quest status index entry.
func (s *Storage) StatusIndexKey(status QuestStatus, questInstance string) string {
	return fmt.Sprintf("idx.quest.status.%s.%s", status, questInstance)
}

// AgentQuestsIndexKey returns the key for an agent's quest index entry.
func (s *Storage) AgentQuestsIndexKey(agentInstance, questInstance string) string {
	return fmt.Sprintf("idx.quest.agent.%s.%s", agentInstance, questInstance)
}

// GuildQuestsIndexKey returns the key for a guild's quest index entry.
func (s *Storage) GuildQuestsIndexKey(guildInstance, questInstance string) string {
	return fmt.Sprintf("idx.quest.guild.%s.%s", guildInstance, questInstance)
}

// ParentQuestIndexKey returns the key for a parent-child quest relationship.
func (s *Storage) ParentQuestIndexKey(parentInstance, childInstance string) string {
	return fmt.Sprintf("idx.quest.parent.%s.%s", parentInstance, childInstance)
}

// StatsKey returns the key for board statistics.
func (s *Storage) StatsKey() string {
	return "stats.board"
}

// AgentStreakKey returns the key for an agent's success streak.
func (s *Storage) AgentStreakKey(instance string) string {
	return fmt.Sprintf("streak.agent.%s", instance)
}

// --- Quest Operations ---

// GetQuest loads a quest by instance ID.
func (s *Storage) GetQuest(ctx context.Context, instance string) (*Quest, error) {
	key := s.QuestKey(instance)
	entry, err := s.kv.Get(ctx, key)
	if err != nil {
		if natsclient.IsKVNotFoundError(err) {
			return nil, fmt.Errorf("quest not found: %s", instance)
		}
		return nil, errs.Wrap(err, "Storage", "GetQuest", "get")
	}

	var quest Quest
	if err := json.Unmarshal(entry.Value, &quest); err != nil {
		return nil, errs.Wrap(err, "Storage", "GetQuest", "unmarshal")
	}
	return &quest, nil
}

// PutQuest stores a quest.
func (s *Storage) PutQuest(ctx context.Context, instance string, quest *Quest) error {
	key := s.QuestKey(instance)
	data, err := json.Marshal(quest)
	if err != nil {
		return errs.Wrap(err, "Storage", "PutQuest", "marshal")
	}
	_, err = s.kv.Put(ctx, key, data)
	if err != nil {
		return errs.Wrap(err, "Storage", "PutQuest", "put")
	}
	return nil
}

// UpdateQuest atomically updates a quest using a modifier function.
func (s *Storage) UpdateQuest(ctx context.Context, instance string, fn func(*Quest) error) error {
	key := s.QuestKey(instance)
	return s.kv.UpdateWithRetry(ctx, key, func(current []byte) ([]byte, error) {
		if len(current) == 0 {
			return nil, fmt.Errorf("quest not found: %s", instance)
		}
		var quest Quest
		if err := json.Unmarshal(current, &quest); err != nil {
			return nil, err
		}
		if err := fn(&quest); err != nil {
			return nil, err
		}
		return json.Marshal(&quest)
	})
}

// --- Agent Operations ---

// GetAgent loads an agent by instance ID.
func (s *Storage) GetAgent(ctx context.Context, instance string) (*Agent, error) {
	key := s.AgentKey(instance)
	entry, err := s.kv.Get(ctx, key)
	if err != nil {
		if natsclient.IsKVNotFoundError(err) {
			return nil, fmt.Errorf("agent not found: %s", instance)
		}
		return nil, errs.Wrap(err, "Storage", "GetAgent", "get")
	}

	var agent Agent
	if err := json.Unmarshal(entry.Value, &agent); err != nil {
		return nil, errs.Wrap(err, "Storage", "GetAgent", "unmarshal")
	}
	return &agent, nil
}

// PutAgent stores an agent.
func (s *Storage) PutAgent(ctx context.Context, instance string, agent *Agent) error {
	key := s.AgentKey(instance)
	data, err := json.Marshal(agent)
	if err != nil {
		return errs.Wrap(err, "Storage", "PutAgent", "marshal")
	}
	_, err = s.kv.Put(ctx, key, data)
	if err != nil {
		return errs.Wrap(err, "Storage", "PutAgent", "put")
	}
	return nil
}

// UpdateAgent atomically updates an agent using a modifier function.
func (s *Storage) UpdateAgent(ctx context.Context, instance string, fn func(*Agent) error) error {
	key := s.AgentKey(instance)
	return s.kv.UpdateWithRetry(ctx, key, func(current []byte) ([]byte, error) {
		if len(current) == 0 {
			return nil, fmt.Errorf("agent not found: %s", instance)
		}
		var agent Agent
		if err := json.Unmarshal(current, &agent); err != nil {
			return nil, err
		}
		if err := fn(&agent); err != nil {
			return nil, err
		}
		return json.Marshal(&agent)
	})
}

// --- Party Operations ---

// GetParty loads a party by instance ID.
func (s *Storage) GetParty(ctx context.Context, instance string) (*Party, error) {
	key := s.PartyKey(instance)
	entry, err := s.kv.Get(ctx, key)
	if err != nil {
		if natsclient.IsKVNotFoundError(err) {
			return nil, fmt.Errorf("party not found: %s", instance)
		}
		return nil, errs.Wrap(err, "Storage", "GetParty", "get")
	}

	var party Party
	if err := json.Unmarshal(entry.Value, &party); err != nil {
		return nil, errs.Wrap(err, "Storage", "GetParty", "unmarshal")
	}
	return &party, nil
}

// PutParty stores a party.
func (s *Storage) PutParty(ctx context.Context, instance string, party *Party) error {
	key := s.PartyKey(instance)
	data, err := json.Marshal(party)
	if err != nil {
		return errs.Wrap(err, "Storage", "PutParty", "marshal")
	}
	_, err = s.kv.Put(ctx, key, data)
	if err != nil {
		return errs.Wrap(err, "Storage", "PutParty", "put")
	}
	return nil
}

// --- Guild Operations ---

// GetGuild loads a guild by instance ID.
func (s *Storage) GetGuild(ctx context.Context, instance string) (*Guild, error) {
	key := s.GuildKey(instance)
	entry, err := s.kv.Get(ctx, key)
	if err != nil {
		if natsclient.IsKVNotFoundError(err) {
			return nil, fmt.Errorf("guild not found: %s", instance)
		}
		return nil, errs.Wrap(err, "Storage", "GetGuild", "get")
	}

	var guild Guild
	if err := json.Unmarshal(entry.Value, &guild); err != nil {
		return nil, errs.Wrap(err, "Storage", "GetGuild", "unmarshal")
	}
	return &guild, nil
}

// PutGuild stores a guild.
func (s *Storage) PutGuild(ctx context.Context, instance string, guild *Guild) error {
	key := s.GuildKey(instance)
	data, err := json.Marshal(guild)
	if err != nil {
		return errs.Wrap(err, "Storage", "PutGuild", "marshal")
	}
	_, err = s.kv.Put(ctx, key, data)
	if err != nil {
		return errs.Wrap(err, "Storage", "PutGuild", "put")
	}
	return nil
}

// --- Battle Operations ---

// GetBattle loads a battle by instance ID.
func (s *Storage) GetBattle(ctx context.Context, instance string) (*BossBattle, error) {
	key := s.BattleKey(instance)
	entry, err := s.kv.Get(ctx, key)
	if err != nil {
		if natsclient.IsKVNotFoundError(err) {
			return nil, fmt.Errorf("battle not found: %s", instance)
		}
		return nil, errs.Wrap(err, "Storage", "GetBattle", "get")
	}

	var battle BossBattle
	if err := json.Unmarshal(entry.Value, &battle); err != nil {
		return nil, errs.Wrap(err, "Storage", "GetBattle", "unmarshal")
	}
	return &battle, nil
}

// PutBattle stores a battle.
func (s *Storage) PutBattle(ctx context.Context, instance string, battle *BossBattle) error {
	key := s.BattleKey(instance)
	data, err := json.Marshal(battle)
	if err != nil {
		return errs.Wrap(err, "Storage", "PutBattle", "marshal")
	}
	_, err = s.kv.Put(ctx, key, data)
	if err != nil {
		return errs.Wrap(err, "Storage", "PutBattle", "put")
	}
	return nil
}

// --- Presence-Based Index Operations ---
// Index entries are just keys with a minimal value.
// Presence of the key = membership in the index.

const indexMarker = "1"

// AddToIndex adds an entry to a presence-based index.
func (s *Storage) AddToIndex(ctx context.Context, key string) error {
	_, err := s.kv.Put(ctx, key, []byte(indexMarker))
	if err != nil {
		return errs.Wrap(err, "Storage", "AddToIndex", "put")
	}
	return nil
}

// RemoveFromIndex removes an entry from a presence-based index.
func (s *Storage) RemoveFromIndex(ctx context.Context, key string) error {
	err := s.kv.Delete(ctx, key)
	if err != nil && !natsclient.IsKVNotFoundError(err) {
		return errs.Wrap(err, "Storage", "RemoveFromIndex", "delete")
	}
	return nil
}

// ListIndexKeys returns all keys matching a prefix.
// The prefix should end with a dot (e.g., "idx.quest.status.posted.").
func (s *Storage) ListIndexKeys(ctx context.Context, prefix string) ([]string, error) {
	allKeys, err := s.kv.Keys(ctx)
	if err != nil {
		if natsclient.IsKVNotFoundError(err) {
			return []string{}, nil
		}
		return nil, errs.Wrap(err, "Storage", "ListIndexKeys", "keys")
	}

	var matching []string
	for _, key := range allKeys {
		if strings.HasPrefix(key, prefix) {
			matching = append(matching, key)
		}
	}
	return matching, nil
}

// ListIndexInstances returns instance IDs from index keys matching a prefix.
// Extracts the last segment after the prefix.
func (s *Storage) ListIndexInstances(ctx context.Context, prefix string) ([]string, error) {
	keys, err := s.ListIndexKeys(ctx, prefix)
	if err != nil {
		return nil, err
	}

	instances := make([]string, 0, len(keys))
	for _, key := range keys {
		// Extract instance from key: prefix + instance
		instance := strings.TrimPrefix(key, prefix)
		if instance != "" {
			instances = append(instances, instance)
		}
	}
	return instances, nil
}

// --- Status Index Helpers ---

// AddQuestStatusIndex adds a quest to a status index.
func (s *Storage) AddQuestStatusIndex(ctx context.Context, status QuestStatus, questInstance string) error {
	key := s.StatusIndexKey(status, questInstance)
	return s.AddToIndex(ctx, key)
}

// RemoveQuestStatusIndex removes a quest from a status index.
func (s *Storage) RemoveQuestStatusIndex(ctx context.Context, status QuestStatus, questInstance string) error {
	key := s.StatusIndexKey(status, questInstance)
	return s.RemoveFromIndex(ctx, key)
}

// ListQuestsByStatus returns quest instance IDs with a given status.
func (s *Storage) ListQuestsByStatus(ctx context.Context, status QuestStatus) ([]string, error) {
	prefix := fmt.Sprintf("idx.quest.status.%s.", status)
	return s.ListIndexInstances(ctx, prefix)
}

// MoveQuestStatus moves a quest from one status index to another.
func (s *Storage) MoveQuestStatus(ctx context.Context, questInstance string, from, to QuestStatus) error {
	// Remove from old index - may fail for various reasons, but doesn't block status transition
	if err := s.RemoveQuestStatusIndex(ctx, from, questInstance); err != nil {
		s.logger.Debug("failed to remove quest from old status index", "quest", questInstance, "from", from, "error", err)
	}
	return s.AddQuestStatusIndex(ctx, to, questInstance)
}

// --- Agent Quest Index Helpers ---

// AddAgentQuestIndex links a quest to an agent.
func (s *Storage) AddAgentQuestIndex(ctx context.Context, agentInstance, questInstance string) error {
	key := s.AgentQuestsIndexKey(agentInstance, questInstance)
	return s.AddToIndex(ctx, key)
}

// RemoveAgentQuestIndex unlinks a quest from an agent.
func (s *Storage) RemoveAgentQuestIndex(ctx context.Context, agentInstance, questInstance string) error {
	key := s.AgentQuestsIndexKey(agentInstance, questInstance)
	return s.RemoveFromIndex(ctx, key)
}

// ListQuestsByAgent returns quest instance IDs assigned to an agent.
func (s *Storage) ListQuestsByAgent(ctx context.Context, agentInstance string) ([]string, error) {
	prefix := fmt.Sprintf("idx.quest.agent.%s.", agentInstance)
	return s.ListIndexInstances(ctx, prefix)
}

// --- Guild Quest Index Helpers ---

// AddGuildQuestIndex links a quest to a guild (priority routing).
func (s *Storage) AddGuildQuestIndex(ctx context.Context, guildInstance, questInstance string) error {
	key := s.GuildQuestsIndexKey(guildInstance, questInstance)
	return s.AddToIndex(ctx, key)
}

// RemoveGuildQuestIndex unlinks a quest from a guild.
func (s *Storage) RemoveGuildQuestIndex(ctx context.Context, guildInstance, questInstance string) error {
	key := s.GuildQuestsIndexKey(guildInstance, questInstance)
	return s.RemoveFromIndex(ctx, key)
}

// ListQuestsByGuild returns quest instance IDs with priority for a guild.
func (s *Storage) ListQuestsByGuild(ctx context.Context, guildInstance string) ([]string, error) {
	prefix := fmt.Sprintf("idx.quest.guild.%s.", guildInstance)
	return s.ListIndexInstances(ctx, prefix)
}

// --- Stats Operations ---

// GetBoardStats loads the current board statistics.
func (s *Storage) GetBoardStats(ctx context.Context) (*BoardStats, error) {
	key := s.StatsKey()
	entry, err := s.kv.Get(ctx, key)
	if err != nil {
		if natsclient.IsKVNotFoundError(err) {
			// Return empty stats if not found
			return &BoardStats{
				ByDifficulty: make(map[QuestDifficulty]int),
				BySkill:      make(map[SkillTag]int),
			}, nil
		}
		return nil, errs.Wrap(err, "Storage", "GetBoardStats", "get")
	}

	var stats BoardStats
	if err := json.Unmarshal(entry.Value, &stats); err != nil {
		return nil, errs.Wrap(err, "Storage", "GetBoardStats", "unmarshal")
	}
	return &stats, nil
}

// PutBoardStats stores board statistics.
func (s *Storage) PutBoardStats(ctx context.Context, stats *BoardStats) error {
	key := s.StatsKey()
	data, err := json.Marshal(stats)
	if err != nil {
		return errs.Wrap(err, "Storage", "PutBoardStats", "marshal")
	}
	_, err = s.kv.Put(ctx, key, data)
	if err != nil {
		return errs.Wrap(err, "Storage", "PutBoardStats", "put")
	}
	return nil
}

// --- Agent Streak Operations ---
// Streak tracking for consecutive successes/failures.

// GetAgentStreak returns the consecutive success count for an agent.
func (s *Storage) GetAgentStreak(ctx context.Context, instance string) (int, error) {
	key := s.AgentStreakKey(instance)
	entry, err := s.kv.Get(ctx, key)
	if err != nil {
		if natsclient.IsKVNotFoundError(err) {
			return 0, nil // No streak recorded yet
		}
		return 0, errs.Wrap(err, "Storage", "GetAgentStreak", "get")
	}

	var streak int
	if err := json.Unmarshal(entry.Value, &streak); err != nil {
		return 0, errs.Wrap(err, "Storage", "GetAgentStreak", "unmarshal")
	}
	return streak, nil
}

// SetAgentStreak sets the consecutive success count for an agent.
func (s *Storage) SetAgentStreak(ctx context.Context, instance string, streak int) error {
	key := s.AgentStreakKey(instance)
	data, err := json.Marshal(streak)
	if err != nil {
		return errs.Wrap(err, "Storage", "SetAgentStreak", "marshal")
	}
	_, err = s.kv.Put(ctx, key, data)
	if err != nil {
		return errs.Wrap(err, "Storage", "SetAgentStreak", "put")
	}
	return nil
}

// IncrementAgentStreak atomically increments and returns the new streak value.
func (s *Storage) IncrementAgentStreak(ctx context.Context, instance string) (int, error) {
	key := s.AgentStreakKey(instance)
	var newStreak int

	err := s.kv.UpdateWithRetry(ctx, key, func(current []byte) ([]byte, error) {
		var streak int
		if len(current) > 0 {
			if err := json.Unmarshal(current, &streak); err != nil {
				return nil, err
			}
		}
		newStreak = streak + 1
		return json.Marshal(newStreak)
	})
	if err != nil {
		return 0, errs.Wrap(err, "Storage", "IncrementAgentStreak", "update")
	}
	return newStreak, nil
}

// ResetAgentStreak resets the streak to zero.
func (s *Storage) ResetAgentStreak(ctx context.Context, instance string) error {
	return s.SetAgentStreak(ctx, instance, 0)
}

// --- Guild Skill Index Operations ---
// Index guilds by primary skill for fast lookup during auto-recruit.

// GuildSkillIndexKey returns the key for a guild's skill index entry.
func (s *Storage) GuildSkillIndexKey(skill SkillTag, guildInstance string) string {
	return fmt.Sprintf("idx.guild.skill.%s.%s", skill, guildInstance)
}

// AddGuildSkillIndex adds a guild to a skill index.
func (s *Storage) AddGuildSkillIndex(ctx context.Context, skill SkillTag, guildInstance string) error {
	key := s.GuildSkillIndexKey(skill, guildInstance)
	return s.AddToIndex(ctx, key)
}

// RemoveGuildSkillIndex removes a guild from a skill index.
func (s *Storage) RemoveGuildSkillIndex(ctx context.Context, skill SkillTag, guildInstance string) error {
	key := s.GuildSkillIndexKey(skill, guildInstance)
	return s.RemoveFromIndex(ctx, key)
}

// ListGuildsBySkill returns guild instance IDs that specialize in a given skill.
func (s *Storage) ListGuildsBySkill(ctx context.Context, skill SkillTag) ([]string, error) {
	prefix := fmt.Sprintf("idx.guild.skill.%s.", skill)
	return s.ListIndexInstances(ctx, prefix)
}

// ListAllAgents returns all agent instances from storage.
func (s *Storage) ListAllAgents(ctx context.Context) ([]*Agent, error) {
	keys, err := s.ListIndexKeys(ctx, "agent.")
	if err != nil {
		return nil, err
	}

	agents := make([]*Agent, 0, len(keys))
	for _, key := range keys {
		// Extract instance from key: "agent.{instance}"
		instance := strings.TrimPrefix(key, "agent.")
		if instance == "" || strings.HasPrefix(instance, "idx.") {
			continue // Skip non-agent keys
		}
		agent, err := s.GetAgent(ctx, instance)
		if err != nil {
			s.logger.Debug("failed to load agent", "instance", instance, "error", err)
			continue
		}
		agents = append(agents, agent)
	}
	return agents, nil
}

// ListAllGuilds returns all guild instances from storage.
func (s *Storage) ListAllGuilds(ctx context.Context) ([]*Guild, error) {
	keys, err := s.ListIndexKeys(ctx, "guild.")
	if err != nil {
		return nil, err
	}

	guilds := make([]*Guild, 0, len(keys))
	for _, key := range keys {
		instance := strings.TrimPrefix(key, "guild.")
		if instance == "" || strings.HasPrefix(instance, "idx.") {
			continue
		}
		guild, err := s.GetGuild(ctx, instance)
		if err != nil {
			s.logger.Debug("failed to load guild", "instance", instance, "error", err)
			continue
		}
		guilds = append(guilds, guild)
	}
	return guilds, nil
}

// UpdateGuild atomically updates a guild using a modifier function.
func (s *Storage) UpdateGuild(ctx context.Context, instance string, fn func(*Guild) error) error {
	key := s.GuildKey(instance)
	return s.kv.UpdateWithRetry(ctx, key, func(current []byte) ([]byte, error) {
		if len(current) == 0 {
			return nil, fmt.Errorf("guild not found: %s", instance)
		}
		var guild Guild
		if err := json.Unmarshal(current, &guild); err != nil {
			return nil, err
		}
		if err := fn(&guild); err != nil {
			return nil, err
		}
		return json.Marshal(&guild)
	})
}
