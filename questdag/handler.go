package questdag

import (
	"context"
	"fmt"

	"github.com/c360studio/semstreams/graph"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semdragons/processor/partycoord"
)

// handleDispatch turns one dispatched gated-DAG unit reference into sub-quest
// work: it resolves the unit's sub-quest and party, picks the best-skill
// available party member (lead excluded), and claim-starts the sub-quest on
// that member. It is the durable-consume handler — a nil return acks the
// dispatch, a non-nil return naks it for redelivery. "No available member" naks
// (natural backpressure until a member frees); a malformed/already-handled
// dispatch acks (returning an error would nak-loop a poison message forever).
func (c *Component) handleDispatch(ctx context.Context, data []byte) error {
	unitID, ok := c.decodeUnit(data)
	if !ok {
		c.logger.Warn("questdag: undecodable dispatch envelope, dropping")
		return nil
	}
	c.dispatchesHandled.Add(1)

	// Resolve the unit entity → sub-quest, node, parent.
	unit, err := c.graph.GetEntityDirect(ctx, unitID)
	if err != nil {
		return fmt.Errorf("questdag: load unit %s: %w", unitID, err) // nak: transient
	}
	subQuestID := tripleString(unit, PredUnitSubQuest)
	nodeID := tripleString(unit, PredUnitNodeID)
	if subQuestID == "" {
		c.logger.Error("questdag: unit has no sub_quest, cannot dispatch", "unit", unitID)
		return nil // ack: malformed unit, nak would loop
	}

	subEntity, err := c.graph.GetQuest(ctx, domain.QuestID(subQuestID))
	if err != nil {
		return fmt.Errorf("questdag: load sub-quest %s: %w", subQuestID, err)
	}
	subQuest := domain.QuestFromEntityState(subEntity)
	if subQuest == nil {
		c.logger.Error("questdag: sub-quest reconstruction returned nil", "sub_quest", subQuestID)
		return nil
	}

	// Idempotency: only a posted sub-quest is dispatchable. A redelivery (or a
	// concurrent claim) finds it already beyond posted — ack and move on.
	if subQuest.Status != domain.QuestPosted {
		c.logger.Info("questdag: sub-quest already beyond posted, skipping",
			"sub_quest", subQuestID, "status", subQuest.Status)
		return nil
	}
	if subQuest.PartyID == nil {
		c.logger.Error("questdag: sub-quest has no party", "sub_quest", subQuestID)
		return nil
	}

	pc := c.resolveParty()
	if pc == nil {
		return fmt.Errorf("questdag: partycoord not available")
	}
	party, found := pc.GetParty(*subQuest.PartyID)
	if !found {
		return fmt.Errorf("questdag: party %s not found for sub-quest %s", *subQuest.PartyID, subQuestID)
	}

	member, err := c.reserveMember(ctx, party, subQuest.RequiredSkills, domain.QuestID(subQuestID))
	if err != nil {
		return err // nak: no member available right now, redeliver later
	}
	reservationCommitted := false
	defer func() {
		if !reservationCommitted {
			c.releaseMemberReservation(member, domain.QuestID(subQuestID))
		}
	}()

	qb := c.resolveQuestBoard()
	if qb == nil {
		return fmt.Errorf("questdag: questboard not available")
	}

	rationale := fmt.Sprintf("questdag: node %s → agent %s (best skill match for %v)", nodeID, member, subQuest.RequiredSkills)
	if err := pc.AssignTask(ctx, party.ID, domain.QuestID(subQuestID), member, rationale); err != nil {
		// Advisory bookkeeping — do not fail the dispatch on it.
		c.logger.Warn("questdag: AssignTask failed, proceeding to claim-start", "error", err)
	}
	if err := qb.ClaimAndStartForParty(ctx, domain.QuestID(subQuestID), party.ID, member); err != nil {
		c.errorsCount.Add(1)
		return fmt.Errorf("questdag: claim-start sub-quest %s for %s: %w", subQuestID, member, err) // nak
	}
	c.commitMemberReservation(member, domain.QuestID(subQuestID))
	reservationCommitted = true

	c.claimStarts.Add(1)
	c.logger.Info("questdag: dispatched sub-quest to party member",
		"unit", unitID, "node", nodeID, "sub_quest", subQuestID, "agent", member, "party", party.ID)
	return nil
}

// selectMember picks the best-skill available party member for the required
// skills. The lead is never assigned executor work; a member whose agent is not
// idle is skipped (prevents double-booking across concurrent dispatches). Ties
// and no-skill nodes fall back to any available non-lead member. Returns an
// error when no member is currently available.
type memberReservation struct {
	questID  domain.QuestID
	claiming bool
}

// reserveMember selects and reserves a member as one atomic process-local
// operation. Successful reservations remain until a later selection observes
// the assigned quest outside claimed/in_progress; failed claim-starts release
// immediately so redelivery can try the member again.
func (c *Component) reserveMember(ctx context.Context, party *partycoord.Party, required []domain.SkillTag, questID domain.QuestID) (domain.AgentID, error) {
	c.reservationMu.Lock()
	defer c.reservationMu.Unlock()

	if c.reservations == nil {
		c.reservations = make(map[domain.AgentID]memberReservation)
	}

	var best *partycoord.PartyMember
	bestScore := -1
	for i := range party.Members {
		m := &party.Members[i]
		if m.Role == domain.RoleLead {
			continue
		}
		if c.memberReserved(ctx, m.AgentID) {
			continue
		}
		if !c.memberAvailable(ctx, m.AgentID) {
			continue
		}
		score := skillOverlap(m.Skills, required)
		if best == nil || score > bestScore {
			best = m
			bestScore = score
		}
	}
	if best == nil {
		return "", fmt.Errorf("questdag: no available non-lead party member for skills %v", required)
	}
	c.reservations[best.AgentID] = memberReservation{questID: questID, claiming: true}
	return best.AgentID, nil
}

// memberReserved is called with reservationMu held. In-flight claim-starts are
// always reserved. Once committed, the quest phase is authoritative: terminal,
// retry, and triage phases free the member for a later unit.
func (c *Component) memberReserved(ctx context.Context, agentID domain.AgentID) bool {
	reservation, ok := c.reservations[agentID]
	if !ok {
		return false
	}
	if reservation.claiming {
		return true
	}
	entity, err := c.graph.GetQuest(ctx, reservation.questID)
	if err != nil {
		return true // conservative: do not double-book on an uncertain read
	}
	quest := domain.QuestFromEntityState(entity)
	if quest != nil && (quest.Status == domain.QuestClaimed || quest.Status == domain.QuestInProgress) {
		return true
	}
	delete(c.reservations, agentID)
	return false
}

func (c *Component) commitMemberReservation(agentID domain.AgentID, questID domain.QuestID) {
	c.reservationMu.Lock()
	defer c.reservationMu.Unlock()
	if current, ok := c.reservations[agentID]; ok && current.questID == questID {
		current.claiming = false
		c.reservations[agentID] = current
	}
}

func (c *Component) releaseMemberReservation(agentID domain.AgentID, questID domain.QuestID) {
	c.reservationMu.Lock()
	defer c.reservationMu.Unlock()
	if current, ok := c.reservations[agentID]; ok && current.questID == questID {
		delete(c.reservations, agentID)
	}
}

// memberAvailable reports whether the member's agent is idle (available for a
// new assignment). A read failure is treated as unavailable (skip, don't crash).
func (c *Component) memberAvailable(ctx context.Context, agentID domain.AgentID) bool {
	entity, err := c.graph.GetAgent(ctx, agentID)
	if err != nil {
		return false
	}
	agent := agentprogression.AgentFromEntityState(entity)
	if agent == nil {
		return false
	}
	return agent.Status == domain.AgentIdle
}

// skillOverlap counts how many required skills the member possesses. A node with
// no skill requirements scores 1 (any member is equally suitable).
func skillOverlap(memberSkills, required []domain.SkillTag) int {
	if len(required) == 0 {
		return 1
	}
	set := make(map[domain.SkillTag]struct{}, len(memberSkills))
	for _, s := range memberSkills {
		set[s] = struct{}{}
	}
	score := 0
	for _, r := range required {
		if _, ok := set[r]; ok {
			score++
		}
	}
	return score
}

// tripleString returns an entity's triple value as a string, or "" if absent.
func tripleString(entity *graph.EntityState, predicate string) string {
	t := entity.GetTriple(predicate)
	if t == nil {
		return ""
	}
	return domain.AsString(t.Object)
}
