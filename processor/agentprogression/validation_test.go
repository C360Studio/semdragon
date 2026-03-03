package agentprogression

import (
	"testing"
	"time"

	"github.com/c360studio/semdragons/domain"
)

func TestValidateAgentCanClaim_Retired(t *testing.T) {
	agent := &Agent{Status: domain.AgentRetired}
	quest := &domain.Quest{Status: domain.QuestPosted}

	err := ValidateAgentCanClaim(agent, quest)
	if err == nil {
		t.Error("expected error for retired agent")
	}
	if err.Error() != "agent is retired" {
		t.Errorf("got %q, want %q", err, "agent is retired")
	}
}

func TestValidateAgentCanClaim_InBattle(t *testing.T) {
	agent := &Agent{Status: domain.AgentInBattle}
	quest := &domain.Quest{Status: domain.QuestPosted}

	err := ValidateAgentCanClaim(agent, quest)
	if err == nil {
		t.Error("expected error for in-battle agent")
	}
	if err.Error() != "agent is in battle" {
		t.Errorf("got %q, want %q", err, "agent is in battle")
	}
}

func TestValidateAgentCanClaim_ActiveCooldown(t *testing.T) {
	future := time.Now().Add(1 * time.Hour)
	agent := &Agent{
		Status:        domain.AgentCooldown,
		CooldownUntil: &future,
	}
	quest := &domain.Quest{Status: domain.QuestPosted}

	err := ValidateAgentCanClaim(agent, quest)
	if err == nil {
		t.Error("expected error for agent on active cooldown")
	}
	if err.Error() != "agent on cooldown" {
		t.Errorf("got %q, want %q", err, "agent on cooldown")
	}
}

func TestValidateAgentCanClaim_ExpiredCooldown(t *testing.T) {
	past := time.Now().Add(-1 * time.Hour)
	agent := &Agent{
		Status:        domain.AgentCooldown,
		Level:         5,
		CooldownUntil: &past,
	}
	quest := &domain.Quest{Status: domain.QuestPosted}

	err := ValidateAgentCanClaim(agent, quest)
	if err != nil {
		t.Errorf("expected nil for expired cooldown, got %v", err)
	}
}

func TestValidateAgentCanClaim_AlreadyOnQuest(t *testing.T) {
	questID := domain.QuestID("some.quest.id")
	agent := &Agent{
		Status:       domain.AgentIdle,
		Level:        5,
		CurrentQuest: &questID,
	}
	quest := &domain.Quest{Status: domain.QuestPosted}

	err := ValidateAgentCanClaim(agent, quest)
	if err == nil {
		t.Error("expected error for agent already on quest")
	}
	if err.Error() != "agent already on a quest" {
		t.Errorf("got %q, want %q", err, "agent already on a quest")
	}
}

func TestValidateAgentCanClaim_TierTooLow(t *testing.T) {
	agent := &Agent{
		Status: domain.AgentIdle,
		Level:  1, // Apprentice
	}
	quest := &domain.Quest{
		Status:  domain.QuestPosted,
		MinTier: domain.TierExpert, // Requires Expert
	}

	err := ValidateAgentCanClaim(agent, quest)
	if err == nil {
		t.Error("expected error for tier mismatch")
	}
	if err.Error() != "agent tier too low" {
		t.Errorf("got %q, want %q", err, "agent tier too low")
	}
}

func TestValidateAgentCanClaim_PartyRequired(t *testing.T) {
	agent := &Agent{
		Status: domain.AgentIdle,
		Level:  5,
	}
	quest := &domain.Quest{
		Status:        domain.QuestPosted,
		PartyRequired: true,
	}

	err := ValidateAgentCanClaim(agent, quest)
	if err == nil {
		t.Error("expected error for party-required quest")
	}
	if err.Error() != "quest requires party" {
		t.Errorf("got %q, want %q", err, "quest requires party")
	}
}

func TestValidateAgentCanClaim_SkillsMismatch(t *testing.T) {
	agent := &Agent{
		Status: domain.AgentIdle,
		Level:  5,
		SkillProficiencies: map[domain.SkillTag]domain.SkillProficiency{
			domain.SkillCodeGen: {Level: 1},
		},
	}
	quest := &domain.Quest{
		Status:         domain.QuestPosted,
		RequiredSkills: []domain.SkillTag{domain.SkillResearch}, // Agent doesn't have this
	}

	err := ValidateAgentCanClaim(agent, quest)
	if err == nil {
		t.Error("expected error for skill mismatch")
	}
	if err.Error() != "agent lacks required skills" {
		t.Errorf("got %q, want %q", err, "agent lacks required skills")
	}
}

func TestValidateAgentCanClaim_Success(t *testing.T) {
	agent := &Agent{
		Status: domain.AgentIdle,
		Level:  5,
		SkillProficiencies: map[domain.SkillTag]domain.SkillProficiency{
			domain.SkillCodeGen: {Level: 2},
		},
	}
	quest := &domain.Quest{
		Status:         domain.QuestPosted,
		MinTier:        domain.TierApprentice,
		RequiredSkills: []domain.SkillTag{domain.SkillCodeGen},
	}

	err := ValidateAgentCanClaim(agent, quest)
	if err != nil {
		t.Errorf("expected nil for valid claim, got %v", err)
	}
}

func TestValidateAgentCanClaim_NoRequiredSkills(t *testing.T) {
	agent := &Agent{
		Status: domain.AgentIdle,
		Level:  5,
	}
	quest := &domain.Quest{
		Status: domain.QuestPosted,
	}

	err := ValidateAgentCanClaim(agent, quest)
	if err != nil {
		t.Errorf("expected nil when quest has no required skills, got %v", err)
	}
}
