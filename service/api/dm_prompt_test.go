package api

import (
	"context"
	"strings"
	"testing"

	"github.com/c360studio/semdragons/domain"
)

type promptWorldState struct{}

func (promptWorldState) WorldState(context.Context) (*domain.WorldState, error) {
	return &domain.WorldState{}, nil
}

func TestDMSystemPromptUsesPhase1ExplicitQuestLifecycle(t *testing.T) {
	service := &Service{world: promptWorldState{}}

	for _, mode := range []domain.ChatMode{domain.ChatModeConverse, domain.ChatModeQuest} {
		t.Run(string(mode), func(t *testing.T) {
			prompt := service.buildDMSystemPrompt(context.Background(), mode, nil, "")

			for _, required := range []string{
				"Phase 1 uses explicit REST/API lifecycle control.",
				"Posting a quest does not claim or start it.",
				"POST /game/quests/{id}/claim",
				`{"agent_id":"<agent-id>"}`,
				"POST /game/quests/{id}/start",
				"The boid engine is dormant in shipped Phase 1 profiles.",
				"Phase 2 opt-in autonomous routing",
			} {
				if !strings.Contains(prompt, required) {
					t.Errorf("DM prompt missing Phase-1 operational contract %q", required)
				}
			}

			for _, stale := range []string{
				"agents claim quests autonomously via the boid engine",
				"so the boid engine routes it naturally",
				"Agents are autonomous — they claim quests",
				"boid-flocking algorithm",
				"You do NOT assign quests to agents",
				"POST /quests/{id}/claim",
				"POST /quests/{id}/start",
			} {
				if strings.Contains(prompt, stale) {
					t.Errorf("DM prompt retains stale autonomous-routing guidance %q", stale)
				}
			}
		})
	}
}
