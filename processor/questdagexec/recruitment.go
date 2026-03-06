package questdagexec

import (
	"context"
	"fmt"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semdragons/processor/partycoord"
)

// =============================================================================
// DEPENDENCY INTERFACES
// =============================================================================

// AgentLister returns all idle agents available for recruitment.
// Implementations query the KV graph for agents whose status is idle and who
// have no CurrentQuest set.
type AgentLister interface {
	ListIdleAgents(ctx context.Context) ([]*agentprogression.Agent, error)
}

// PartyJoiner adds an agent to a party with the given role.
// The signature mirrors partycoord.Component.JoinParty.
type PartyJoiner interface {
	JoinParty(ctx context.Context, partyID domain.PartyID, agentID domain.AgentID, role domain.PartyRole) error
}

// RecruitmentDeps bundles the dependencies required by RecruitMembers.
// Using an interface per dependency rather than a single fat interface keeps
// mock implementations focused and test cases readable.
type RecruitmentDeps struct {
	Agents     AgentLister
	PartyJoins PartyJoiner
}

// PartyMemberLister returns the current members of a party.
type PartyMemberLister interface {
	GetParty(partyID domain.PartyID) (*partycoord.Party, bool)
}

// TaskAssigner records a sub-quest assignment within the party coordination layer.
// The signature mirrors partycoord.Component.AssignTask.
type TaskAssigner interface {
	AssignTask(ctx context.Context, partyID domain.PartyID, subQuestID domain.QuestID, assignedTo domain.AgentID, rationale string) error
}

// QuestClaimerForParty formally claims a sub-quest on behalf of a party.
// The signature mirrors questboard.Component.ClaimQuestForParty.
type QuestClaimerForParty interface {
	ClaimQuestForParty(ctx context.Context, questID domain.QuestID, partyID domain.PartyID) error
}

// AssignmentDeps bundles the dependencies required by AssignReadyNodes.
type AssignmentDeps struct {
	Members     PartyMemberLister
	Tasks       TaskAssigner
	QuestClaims QuestClaimerForParty
}

// =============================================================================
// SKILL SCORING
// =============================================================================

// scoreAgentForNode returns the number of skills the agent possesses that
// overlap with the node's required skills. A higher score indicates a better
// fit. Score of zero means no overlap; the agent may still be assigned if no
// better candidate exists.
func scoreAgentForNode(agent *agentprogression.Agent, node QuestNode) int {
	if len(node.Skills) == 0 {
		// Node has no skill requirements — any agent is equally suitable.
		return 1
	}
	score := 0
	for _, required := range node.Skills {
		if agent.HasSkill(domain.SkillTag(required)) {
			score++
		}
	}
	return score
}

// agentMeetsTier returns true when the agent's level qualifies them for the
// node's difficulty. Nodes with Difficulty zero (Trivial) accept all agents.
func agentMeetsTier(agent *agentprogression.Agent, node QuestNode) bool {
	agentTier := domain.TierFromLevel(agent.Level)
	minTier := domain.TierFromDifficulty(domain.QuestDifficulty(node.Difficulty))
	return agentTier >= minTier
}

// =============================================================================
// RECRUIT MEMBERS
// =============================================================================

// RecruitMembers finds and recruits idle agents for party sub-quests.
// It scores candidates by skill overlap with sub-quest requirements, filters
// by trust tier, and calls JoinParty for each selected agent.
//
// The function works greedily: each node is served by the highest-scoring
// remaining candidate. An agent assigned to one node is excluded from
// subsequent nodes to prevent double-assignment.
//
// Returns an error when there are fewer eligible idle agents than DAG nodes
// that require a recruit. Callers should retry after a delay.
func RecruitMembers(ctx context.Context, dagState *DAGExecutionState, deps RecruitmentDeps) error {
	idleAgents, err := deps.Agents.ListIdleAgents(ctx)
	if err != nil {
		return fmt.Errorf("list idle agents: %w", err)
	}

	if len(idleAgents) == 0 {
		return fmt.Errorf("no idle agents available for recruitment")
	}

	// Build a node index for O(1) lookup when iterating.
	nodesByID := make(map[string]QuestNode, len(dagState.DAG.Nodes))
	for _, node := range dagState.DAG.Nodes {
		nodesByID[node.ID] = node
	}

	// Track which agents have already been recruited so we don't double-assign.
	recruited := make(map[domain.AgentID]struct{}, len(dagState.DAG.Nodes))

	// Attempt to recruit one agent per node.
	var shortfall int
	for _, node := range dagState.DAG.Nodes {
		best, score := selectCandidate(idleAgents, node, recruited)
		if best == nil || score < 0 {
			shortfall++
			continue
		}

		if err := deps.PartyJoins.JoinParty(ctx, domain.PartyID(dagState.PartyID), best.ID, domain.RoleExecutor); err != nil {
			return fmt.Errorf("join party for agent %s (node %s): %w", best.ID, node.ID, err)
		}

		recruited[best.ID] = struct{}{}
	}

	if shortfall > 0 {
		return fmt.Errorf("insufficient idle agents: %d node(s) could not be staffed", shortfall)
	}

	return nil
}

// selectCandidate returns the best-fit idle agent for a node from the
// candidate pool, excluding already-recruited agents. The selection prefers
// agents with higher skill overlap. Agents that do not meet the node's minimum
// trust tier are excluded entirely.
//
// Returns (nil, -1) when no eligible candidate remains.
func selectCandidate(agents []*agentprogression.Agent, node QuestNode, recruited map[domain.AgentID]struct{}) (*agentprogression.Agent, int) {
	var best *agentprogression.Agent
	bestScore := -1

	for _, agent := range agents {
		if _, used := recruited[agent.ID]; used {
			continue
		}
		if !agentMeetsTier(agent, node) {
			continue
		}
		score := scoreAgentForNode(agent, node)
		if best == nil || score > bestScore {
			best = agent
			bestScore = score
		}
	}

	return best, bestScore
}

// =============================================================================
// ASSIGN READY NODES
// =============================================================================

// AssignReadyNodes assigns ready DAG nodes to recruited party members.
// It calls DAGReadyNodes to find eligible nodes, matches each to the
// best-fit party member by skill overlap, formally claims the sub-quest,
// and updates DAGExecutionState.
//
// NodeStates for each assigned node transitions to NodeAssigned.
// NodeAssignees is updated with the selected agent ID.
//
// Party members already assigned to a node in this DAGExecutionState are
// excluded from further assignment to prevent double-booking.
func AssignReadyNodes(ctx context.Context, dagState *DAGExecutionState, deps AssignmentDeps) error {
	readyNodeIDs := DAGReadyNodes(dagState.DAG, dagState.NodeStates)
	if len(readyNodeIDs) == 0 {
		return nil
	}

	party, ok := deps.Members.GetParty(domain.PartyID(dagState.PartyID))
	if !ok {
		return fmt.Errorf("party %s not found", dagState.PartyID)
	}

	// Build the set of already-assigned agents so we don't double-book them.
	alreadyAssigned := make(map[domain.AgentID]struct{}, len(dagState.NodeAssignees))
	for _, agentID := range dagState.NodeAssignees {
		alreadyAssigned[domain.AgentID(agentID)] = struct{}{}
	}

	// Build a node index for O(1) skill lookup.
	nodesByID := make(map[string]QuestNode, len(dagState.DAG.Nodes))
	for _, node := range dagState.DAG.Nodes {
		nodesByID[node.ID] = node
	}

	// Assign each ready node to the best available party member.
	for _, nodeID := range readyNodeIDs {
		node := nodesByID[nodeID]

		member := selectMember(party.Members, node, alreadyAssigned)
		if member == nil {
			return fmt.Errorf("no available party member for node %s", nodeID)
		}

		subQuestID := domain.QuestID(dagState.NodeQuestIDs[nodeID])
		rationale := buildRationale(member, node)

		if err := deps.Tasks.AssignTask(ctx, domain.PartyID(dagState.PartyID), subQuestID, member.AgentID, rationale); err != nil {
			return fmt.Errorf("assign task for node %s to agent %s: %w", nodeID, member.AgentID, err)
		}

		if err := deps.QuestClaims.ClaimQuestForParty(ctx, subQuestID, domain.PartyID(dagState.PartyID)); err != nil {
			return fmt.Errorf("claim quest %s for party: %w", subQuestID, err)
		}

		// Advance DAG state.
		dagState.NodeStates[nodeID] = NodeAssigned
		dagState.NodeAssignees[nodeID] = string(member.AgentID)

		// Exclude this member from subsequent node assignments in this pass.
		alreadyAssigned[member.AgentID] = struct{}{}
	}

	return nil
}

// selectMember returns the best-fit party member for a node, excluding members
// already assigned to another node. The lead (role "lead") is never assigned
// executor work — they orchestrate rather than execute.
//
// Returns nil when no eligible member remains.
func selectMember(members []partycoord.PartyMember, node QuestNode, alreadyAssigned map[domain.AgentID]struct{}) *partycoord.PartyMember {
	var best *partycoord.PartyMember
	bestScore := -1

	for i := range members {
		m := &members[i]

		// The lead orchestrates — they must not be assigned executor work.
		if m.Role == domain.RoleLead {
			continue
		}
		if _, used := alreadyAssigned[m.AgentID]; used {
			continue
		}

		// Score by skill overlap between member's skills and node requirements.
		score := scoreSkillOverlap(m.Skills, node.Skills)
		if best == nil || score > bestScore {
			best = m
			bestScore = score
		}
	}

	return best
}

// scoreSkillOverlap counts the number of node skills present in the member's
// skill list. When the node has no requirements, any member scores 1 (equally
// suitable). This matches the scoring logic in scoreAgentForNode but operates
// on []domain.SkillTag rather than Agent.SkillProficiencies.
func scoreSkillOverlap(memberSkills []domain.SkillTag, nodeSkills []string) int {
	if len(nodeSkills) == 0 {
		return 1
	}
	memberSet := make(map[domain.SkillTag]struct{}, len(memberSkills))
	for _, s := range memberSkills {
		memberSet[s] = struct{}{}
	}
	score := 0
	for _, required := range nodeSkills {
		if _, ok := memberSet[domain.SkillTag(required)]; ok {
			score++
		}
	}
	return score
}

// buildRationale constructs a human-readable assignment rationale string that
// summarises why this member was chosen for the node. This text is forwarded
// to the party coordination layer as the AssignTask rationale argument.
func buildRationale(member *partycoord.PartyMember, node QuestNode) string {
	if len(node.Skills) == 0 {
		return fmt.Sprintf("assigned agent %s to node %s (no specific skill requirements)", member.AgentID, node.ID)
	}
	return fmt.Sprintf("assigned agent %s to node %s based on skill overlap with requirements: %v", member.AgentID, node.ID, node.Skills)
}
