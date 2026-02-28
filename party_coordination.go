package semdragons

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/errs"
)

// =============================================================================
// PARTY COORDINATOR - Communication helper for party quest coordination
// =============================================================================
// Provides event-driven coordination between party leads and members:
//
// Lead → Members:
//   - DecomposeQuest: Break parent quest into sub-quests
//   - AssignTask: Assign sub-quest to member
//   - ShareContext: Share insights/context with party
//   - IssueGuidance: Provide guidance to struggling member
//
// Members → Lead:
//   - ReportProgress: Report sub-quest progress
//   - RequestHelp: Ask lead for assistance
//   - SubmitResult: Submit sub-quest result
//
// Lead Rollup:
//   - StartRollup: Begin combining sub-results
//   - CompleteRollup: Finish rollup, ready for boss battle
// =============================================================================

// PartyCoordinator provides event-driven coordination for party quests.
type PartyCoordinator struct {
	client  *natsclient.Client
	graph   *GraphClient
	config  *BoardConfig
	events  *EventPublisher
	partyID PartyID
}

// NewPartyCoordinator creates a coordinator for a specific party.
func NewPartyCoordinator(client *natsclient.Client, graph *GraphClient, config *BoardConfig, partyID PartyID) *PartyCoordinator {
	return &PartyCoordinator{
		client:  client,
		graph:   graph,
		config:  config,
		events:  NewEventPublisher(client),
		partyID: partyID,
	}
}

// PartyID returns the party this coordinator is managing.
func (pc *PartyCoordinator) PartyID() PartyID {
	return pc.partyID
}

// =============================================================================
// KV HELPERS - Private methods for party coordination state
// =============================================================================

// kvKey generates a KV key for party coordination data.
func (pc *PartyCoordinator) kvKey(suffix string) string {
	return fmt.Sprintf("party.coord.%s.%s", pc.partyID, suffix)
}

// getSubQuestMap retrieves the sub-quest to agent assignment map.
func (pc *PartyCoordinator) getSubQuestMap(ctx context.Context) (map[QuestID]AgentID, error) {
	bucket, err := pc.client.GetKeyValueBucket(ctx, pc.config.BucketName())
	if err != nil {
		return nil, err
	}

	entry, err := bucket.Get(ctx, pc.kvKey("subquests"))
	if err != nil {
		// Not found means empty map
		return make(map[QuestID]AgentID), nil
	}

	var result map[QuestID]AgentID
	if err := json.Unmarshal(entry.Value(), &result); err != nil {
		return nil, err
	}
	return result, nil
}

// putSubQuestMap stores the sub-quest to agent assignment map.
func (pc *PartyCoordinator) putSubQuestMap(ctx context.Context, subQuestMap map[QuestID]AgentID) error {
	bucket, err := pc.client.GetKeyValueBucket(ctx, pc.config.BucketName())
	if err != nil {
		return err
	}

	data, err := json.Marshal(subQuestMap)
	if err != nil {
		return err
	}

	_, err = bucket.Put(ctx, pc.kvKey("subquests"), data)
	return err
}

// addContext appends a context item to the party's shared context.
func (pc *PartyCoordinator) addContext(ctx context.Context, item ContextItem) error {
	existing, err := pc.getContext(ctx)
	if err != nil {
		return err
	}

	existing = append(existing, item)

	bucket, err := pc.client.GetKeyValueBucket(ctx, pc.config.BucketName())
	if err != nil {
		return err
	}

	data, err := json.Marshal(existing)
	if err != nil {
		return err
	}

	_, err = bucket.Put(ctx, pc.kvKey("context"), data)
	return err
}

// getContext retrieves all shared context items.
func (pc *PartyCoordinator) getContext(ctx context.Context) ([]ContextItem, error) {
	bucket, err := pc.client.GetKeyValueBucket(ctx, pc.config.BucketName())
	if err != nil {
		return nil, err
	}

	entry, err := bucket.Get(ctx, pc.kvKey("context"))
	if err != nil {
		// Not found means empty slice
		return []ContextItem{}, nil
	}

	var result []ContextItem
	if err := json.Unmarshal(entry.Value(), &result); err != nil {
		return nil, err
	}
	return result, nil
}

// getSubResults retrieves submitted sub-quest results.
func (pc *PartyCoordinator) getSubResults(ctx context.Context) (map[QuestID]any, error) {
	bucket, err := pc.client.GetKeyValueBucket(ctx, pc.config.BucketName())
	if err != nil {
		return nil, err
	}

	entry, err := bucket.Get(ctx, pc.kvKey("results"))
	if err != nil {
		// Not found means empty map
		return make(map[QuestID]any), nil
	}

	var result map[QuestID]any
	if err := json.Unmarshal(entry.Value(), &result); err != nil {
		return nil, err
	}
	return result, nil
}

// putSubResult stores a single sub-quest result.
func (pc *PartyCoordinator) putSubResult(ctx context.Context, questID QuestID, result any) error {
	existing, err := pc.getSubResults(ctx)
	if err != nil {
		return err
	}

	existing[questID] = result

	bucket, err := pc.client.GetKeyValueBucket(ctx, pc.config.BucketName())
	if err != nil {
		return err
	}

	data, err := json.Marshal(existing)
	if err != nil {
		return err
	}

	_, err = bucket.Put(ctx, pc.kvKey("results"), data)
	return err
}

// getRollupResult retrieves the rollup result.
func (pc *PartyCoordinator) getRollupResult(ctx context.Context) (any, error) {
	bucket, err := pc.client.GetKeyValueBucket(ctx, pc.config.BucketName())
	if err != nil {
		return nil, err
	}

	entry, err := bucket.Get(ctx, pc.kvKey("rollup"))
	if err != nil {
		// Not found means no rollup yet
		return nil, nil
	}

	var result any
	if err := json.Unmarshal(entry.Value(), &result); err != nil {
		return nil, err
	}
	return result, nil
}

// putRollupResult stores the rollup result.
func (pc *PartyCoordinator) putRollupResult(ctx context.Context, result any) error {
	bucket, err := pc.client.GetKeyValueBucket(ctx, pc.config.BucketName())
	if err != nil {
		return err
	}

	data, err := json.Marshal(result)
	if err != nil {
		return err
	}

	_, err = bucket.Put(ctx, pc.kvKey("rollup"), data)
	return err
}

// =============================================================================
// LEAD ACTIONS - Operations available to the party lead
// =============================================================================

// DecomposeQuest records the decomposition of a parent quest into sub-quests.
// The lead should call this after posting sub-quests to the board.
func (pc *PartyCoordinator) DecomposeQuest(
	ctx context.Context,
	leadID AgentID,
	parentQuest QuestID,
	subQuests []QuestID,
	strategy string,
) error {
	// Update party state with sub-quest map (initially empty assignments)
	subQuestMap := make(map[QuestID]AgentID)
	for _, sq := range subQuests {
		subQuestMap[sq] = "" // Unassigned
	}
	if err := pc.putSubQuestMap(ctx, subQuestMap); err != nil {
		return errs.Wrap(err, "PartyCoordinator", "DecomposeQuest", "update sub-quest map")
	}

	// Emit decomposition event
	return pc.events.PublishPartyQuestDecomposed(ctx, PartyQuestDecomposedPayload{
		PartyID:     pc.partyID,
		LeadID:      leadID,
		ParentQuest: parentQuest,
		SubQuests:   subQuests,
		Strategy:    strategy,
		Timestamp:   time.Now(),
	})
}

// AssignTask assigns a sub-quest to a party member.
func (pc *PartyCoordinator) AssignTask(
	ctx context.Context,
	leadID AgentID,
	assignedTo AgentID,
	subQuestID QuestID,
	rationale string,
	dependencies []QuestID,
	guidance string,
) error {
	// Update the sub-quest map
	subQuestMap, err := pc.getSubQuestMap(ctx)
	if err != nil {
		return errs.Wrap(err, "PartyCoordinator", "AssignTask", "get sub-quest map")
	}
	subQuestMap[subQuestID] = assignedTo
	if err := pc.putSubQuestMap(ctx, subQuestMap); err != nil {
		return errs.Wrap(err, "PartyCoordinator", "AssignTask", "update sub-quest map")
	}

	// Emit assignment event
	return pc.events.PublishPartyTaskAssigned(ctx, PartyTaskAssignedPayload{
		PartyID:      pc.partyID,
		LeadID:       leadID,
		AssignedTo:   assignedTo,
		SubQuestID:   subQuestID,
		Rationale:    rationale,
		Dependencies: dependencies,
		Guidance:     guidance,
		Timestamp:    time.Now(),
	})
}

// IssueGuidance sends guidance to a party member working on a sub-quest.
func (pc *PartyCoordinator) IssueGuidance(
	ctx context.Context,
	leadID AgentID,
	targetMember AgentID,
	subQuestID QuestID,
	guidanceType string,
	guidance string,
) error {
	return pc.events.PublishPartyGuidanceIssued(ctx, PartyGuidanceIssuedPayload{
		PartyID:      pc.partyID,
		LeadID:       leadID,
		TargetMember: targetMember,
		SubQuestID:   subQuestID,
		GuidanceType: guidanceType,
		Guidance:     guidance,
		Timestamp:    time.Now(),
	})
}

// ShareContext shares context/insights with the entire party.
func (pc *PartyCoordinator) ShareContext(
	ctx context.Context,
	sharedBy AgentID,
	item ContextItem,
	relevance []QuestID,
) error {
	// Store context in party state
	if err := pc.addContext(ctx, item); err != nil {
		return errs.Wrap(err, "PartyCoordinator", "ShareContext", "add context")
	}

	// Emit context shared event
	return pc.events.PublishPartyContextShared(ctx, PartyContextSharedPayload{
		PartyID:     pc.partyID,
		SharedBy:    sharedBy,
		ContextItem: item,
		Relevance:   relevance,
		Timestamp:   time.Now(),
	})
}

// StartRollup marks the beginning of result combination.
func (pc *PartyCoordinator) StartRollup(
	ctx context.Context,
	leadID AgentID,
	parentQuestID QuestID,
) error {
	// Get count of collected results
	results, err := pc.getSubResults(ctx)
	if err != nil {
		return errs.Wrap(err, "PartyCoordinator", "StartRollup", "get sub-results")
	}

	return pc.events.PublishPartyRollupStarted(ctx, PartyRollupStartedPayload{
		PartyID:         pc.partyID,
		LeadID:          leadID,
		ParentQuestID:   parentQuestID,
		SubResultsCount: len(results),
		Timestamp:       time.Now(),
	})
}

// CompleteRollup records the completion of result combination.
func (pc *PartyCoordinator) CompleteRollup(
	ctx context.Context,
	leadID AgentID,
	parentQuestID QuestID,
	rollupResult any,
	memberContributions map[AgentID]float64,
) error {
	// Store rollup result
	if err := pc.putRollupResult(ctx, rollupResult); err != nil {
		return errs.Wrap(err, "PartyCoordinator", "CompleteRollup", "store rollup")
	}

	return pc.events.PublishPartyRollupCompleted(ctx, PartyRollupCompletedPayload{
		PartyID:       pc.partyID,
		LeadID:        leadID,
		ParentQuestID: parentQuestID,
		RollupResult:  rollupResult,
		MemberContrib: memberContributions,
		Timestamp:     time.Now(),
	})
}

// =============================================================================
// MEMBER ACTIONS - Operations available to party members
// =============================================================================

// ReportProgress reports progress on a sub-quest to the lead.
func (pc *PartyCoordinator) ReportProgress(
	ctx context.Context,
	memberID AgentID,
	subQuestID QuestID,
	progressPercent int,
	status string,
	message string,
) error {
	return pc.events.PublishPartyProgressReported(ctx, PartyProgressReportedPayload{
		PartyID:         pc.partyID,
		MemberID:        memberID,
		SubQuestID:      subQuestID,
		ProgressPercent: progressPercent,
		Status:          status,
		Message:         message,
		Timestamp:       time.Now(),
	})
}

// RequestHelp requests assistance from the party lead.
func (pc *PartyCoordinator) RequestHelp(
	ctx context.Context,
	memberID AgentID,
	subQuestID QuestID,
	issueType string,
	description string,
	urgency string,
) error {
	return pc.events.PublishPartyHelpRequested(ctx, PartyHelpRequestedPayload{
		PartyID:     pc.partyID,
		MemberID:    memberID,
		SubQuestID:  subQuestID,
		IssueType:   issueType,
		Description: description,
		Urgency:     urgency,
		Timestamp:   time.Now(),
	})
}

// SubmitResult submits a sub-quest result to the party lead.
func (pc *PartyCoordinator) SubmitResult(
	ctx context.Context,
	memberID AgentID,
	subQuestID QuestID,
	result any,
	qualityScore float64,
) error {
	// Store the result
	if err := pc.putSubResult(ctx, subQuestID, result); err != nil {
		return errs.Wrap(err, "PartyCoordinator", "SubmitResult", "store result")
	}

	return pc.events.PublishPartyResultSubmitted(ctx, PartyResultSubmittedPayload{
		PartyID:      pc.partyID,
		MemberID:     memberID,
		SubQuestID:   subQuestID,
		Result:       result,
		QualityScore: qualityScore,
		Timestamp:    time.Now(),
	})
}

// =============================================================================
// SUBSCRIPTION HELPERS - For reactive event handling
// =============================================================================

// SubscriptionOptions configures how subscriptions behave.
type SubscriptionOptions struct {
	// BufferSize is the channel buffer size (default: 16)
	BufferSize int
}

// DefaultSubscriptionOptions returns sensible defaults.
func DefaultSubscriptionOptions() SubscriptionOptions {
	return SubscriptionOptions{
		BufferSize: 16,
	}
}

// SubscribeToAssignments subscribes a member to task assignment events.
// The member can filter for their own assignments by checking AssignedTo.
func (pc *PartyCoordinator) SubscribeToAssignments(
	ctx context.Context,
	opts SubscriptionOptions,
) (<-chan PartyTaskAssignedPayload, error) {
	if opts.BufferSize <= 0 {
		opts.BufferSize = 16
	}

	ch := make(chan PartyTaskAssignedPayload, opts.BufferSize)
	partyID := pc.partyID

	sub, err := SubjectPartyTaskAssigned.Subscribe(ctx, pc.client, func(_ context.Context, payload PartyTaskAssignedPayload) error {
		// Filter for this party only
		if payload.PartyID != partyID {
			return nil
		}
		select {
		case ch <- payload:
		case <-ctx.Done():
		}
		return nil
	})
	if err != nil {
		close(ch)
		return nil, errs.Wrap(err, "PartyCoordinator", "SubscribeToAssignments", "subscribe")
	}

	// Clean up subscription when context is done
	go func() {
		<-ctx.Done()
		sub.Unsubscribe()
		close(ch)
	}()

	return ch, nil
}

// SubscribeToGuidance subscribes to guidance events for party members.
func (pc *PartyCoordinator) SubscribeToGuidance(
	ctx context.Context,
	opts SubscriptionOptions,
) (<-chan PartyGuidanceIssuedPayload, error) {
	if opts.BufferSize <= 0 {
		opts.BufferSize = 16
	}

	ch := make(chan PartyGuidanceIssuedPayload, opts.BufferSize)
	partyID := pc.partyID

	sub, err := SubjectPartyGuidanceIssued.Subscribe(ctx, pc.client, func(_ context.Context, payload PartyGuidanceIssuedPayload) error {
		if payload.PartyID != partyID {
			return nil
		}
		select {
		case ch <- payload:
		case <-ctx.Done():
		}
		return nil
	})
	if err != nil {
		close(ch)
		return nil, errs.Wrap(err, "PartyCoordinator", "SubscribeToGuidance", "subscribe")
	}

	go func() {
		<-ctx.Done()
		sub.Unsubscribe()
		close(ch)
	}()

	return ch, nil
}

// SubscribeToContextShared subscribes to context sharing events.
func (pc *PartyCoordinator) SubscribeToContextShared(
	ctx context.Context,
	opts SubscriptionOptions,
) (<-chan PartyContextSharedPayload, error) {
	if opts.BufferSize <= 0 {
		opts.BufferSize = 16
	}

	ch := make(chan PartyContextSharedPayload, opts.BufferSize)
	partyID := pc.partyID

	sub, err := SubjectPartyContextShared.Subscribe(ctx, pc.client, func(_ context.Context, payload PartyContextSharedPayload) error {
		if payload.PartyID != partyID {
			return nil
		}
		select {
		case ch <- payload:
		case <-ctx.Done():
		}
		return nil
	})
	if err != nil {
		close(ch)
		return nil, errs.Wrap(err, "PartyCoordinator", "SubscribeToContextShared", "subscribe")
	}

	go func() {
		<-ctx.Done()
		sub.Unsubscribe()
		close(ch)
	}()

	return ch, nil
}

// SubscribeToProgressReports subscribes the lead to member progress reports.
func (pc *PartyCoordinator) SubscribeToProgressReports(
	ctx context.Context,
	opts SubscriptionOptions,
) (<-chan PartyProgressReportedPayload, error) {
	if opts.BufferSize <= 0 {
		opts.BufferSize = 16
	}

	ch := make(chan PartyProgressReportedPayload, opts.BufferSize)
	partyID := pc.partyID

	sub, err := SubjectPartyProgressReported.Subscribe(ctx, pc.client, func(_ context.Context, payload PartyProgressReportedPayload) error {
		if payload.PartyID != partyID {
			return nil
		}
		select {
		case ch <- payload:
		case <-ctx.Done():
		}
		return nil
	})
	if err != nil {
		close(ch)
		return nil, errs.Wrap(err, "PartyCoordinator", "SubscribeToProgressReports", "subscribe")
	}

	go func() {
		<-ctx.Done()
		sub.Unsubscribe()
		close(ch)
	}()

	return ch, nil
}

// SubscribeToHelpRequests subscribes the lead to member help requests.
func (pc *PartyCoordinator) SubscribeToHelpRequests(
	ctx context.Context,
	opts SubscriptionOptions,
) (<-chan PartyHelpRequestedPayload, error) {
	if opts.BufferSize <= 0 {
		opts.BufferSize = 16
	}

	ch := make(chan PartyHelpRequestedPayload, opts.BufferSize)
	partyID := pc.partyID

	sub, err := SubjectPartyHelpRequested.Subscribe(ctx, pc.client, func(_ context.Context, payload PartyHelpRequestedPayload) error {
		if payload.PartyID != partyID {
			return nil
		}
		select {
		case ch <- payload:
		case <-ctx.Done():
		}
		return nil
	})
	if err != nil {
		close(ch)
		return nil, errs.Wrap(err, "PartyCoordinator", "SubscribeToHelpRequests", "subscribe")
	}

	go func() {
		<-ctx.Done()
		sub.Unsubscribe()
		close(ch)
	}()

	return ch, nil
}

// SubscribeToResultSubmissions subscribes the lead to member result submissions.
func (pc *PartyCoordinator) SubscribeToResultSubmissions(
	ctx context.Context,
	opts SubscriptionOptions,
) (<-chan PartyResultSubmittedPayload, error) {
	if opts.BufferSize <= 0 {
		opts.BufferSize = 16
	}

	ch := make(chan PartyResultSubmittedPayload, opts.BufferSize)
	partyID := pc.partyID

	sub, err := SubjectPartyResultSubmitted.Subscribe(ctx, pc.client, func(_ context.Context, payload PartyResultSubmittedPayload) error {
		if payload.PartyID != partyID {
			return nil
		}
		select {
		case ch <- payload:
		case <-ctx.Done():
		}
		return nil
	})
	if err != nil {
		close(ch)
		return nil, errs.Wrap(err, "PartyCoordinator", "SubscribeToResultSubmissions", "subscribe")
	}

	go func() {
		<-ctx.Done()
		sub.Unsubscribe()
		close(ch)
	}()

	return ch, nil
}

// =============================================================================
// QUERY HELPERS - Retrieve party coordination state
// =============================================================================

// GetAssignments returns the current sub-quest assignments.
func (pc *PartyCoordinator) GetAssignments(ctx context.Context) (map[QuestID]AgentID, error) {
	return pc.getSubQuestMap(ctx)
}

// GetSharedContext returns all shared context items.
func (pc *PartyCoordinator) GetSharedContext(ctx context.Context) ([]ContextItem, error) {
	return pc.getContext(ctx)
}

// GetCollectedResults returns all submitted sub-quest results.
func (pc *PartyCoordinator) GetCollectedResults(ctx context.Context) (map[QuestID]any, error) {
	return pc.getSubResults(ctx)
}

// GetRollupResult returns the final rollup result, if available.
func (pc *PartyCoordinator) GetRollupResult(ctx context.Context) (any, error) {
	return pc.getRollupResult(ctx)
}

// AreAllResultsCollected checks if all assigned sub-quests have submitted results.
func (pc *PartyCoordinator) AreAllResultsCollected(ctx context.Context) (bool, error) {
	assignments, err := pc.getSubQuestMap(ctx)
	if err != nil {
		return false, errs.Wrap(err, "PartyCoordinator", "AreAllResultsCollected", "get assignments")
	}

	results, err := pc.getSubResults(ctx)
	if err != nil {
		return false, errs.Wrap(err, "PartyCoordinator", "AreAllResultsCollected", "get results")
	}

	// Check that every assigned quest has a result
	for questID := range assignments {
		if _, hasResult := results[questID]; !hasResult {
			return false, nil
		}
	}

	return len(assignments) > 0, nil
}
