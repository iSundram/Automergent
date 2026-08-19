package wiring

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/iSundram/Automergent/internal/graph"
)

type EventSystem struct {
	store        *graph.Store
	engine       *WiringEngine
	eventDefs    map[uuid.UUID]*EventDefinition
	handlers     map[uuid.UUID][]*EventHandler
	flows        map[uuid.UUID]*EventFlow
	emissionPoints map[uuid.UUID][]*EmissionPoint
	mu           sync.RWMutex
}

func NewEventSystem(store *graph.Store, engine *WiringEngine) *EventSystem {
	return &EventSystem{
		store:           store,
		engine:          engine,
		eventDefs:       make(map[uuid.UUID]*EventDefinition),
		handlers:        make(map[uuid.UUID][]*EventHandler),
		flows:           make(map[uuid.UUID]*EventFlow),
		emissionPoints:  make(map[uuid.UUID][]*EmissionPoint),
	}
}

func (es *EventSystem) DefineEvent(ctx context.Context, feature *FeatureAppearance, eventDef *EventDefinition) error {
	es.mu.Lock()
	defer es.mu.Unlock()

	if eventDef.ID == uuid.Nil {
		eventDef.ID = uuid.New()
	}
	eventDef.FeatureID = feature.FeatureID
	eventDef.CreatedAt = time.Now()
	eventDef.UpdatedAt = time.Now()

	es.eventDefs[eventDef.ID] = eventDef

	for _, handler := range eventDef.Handlers {
		handler.EventID = eventDef.ID
		handler.FeatureID = feature.FeatureID
		if handler.ID == uuid.Nil {
			handler.ID = uuid.New()
		}
		h := handler
		es.RegisterHandler(ctx, eventDef.ID, &h)
	}

	for _, ep := range eventDef.EmissionPoints {
		ep.EventID = eventDef.ID
		if ep.ID == uuid.Nil {
			ep.ID = uuid.New()
		}
		es.emissionPoints[eventDef.ID] = append(es.emissionPoints[eventDef.ID], &ep)
	}

	return es.persistEventDefinition(ctx, eventDef)
}

func (es *EventSystem) DefineFeatureEvents(ctx context.Context, feature *FeatureAppearance) ([]EventDefinition, error) {
	var events []EventDefinition

	for _, epType := range []EntryPointType{EntryPointTypeTUI, EntryPointTypeAPI, EntryPointTypeCLI, EntryPointTypeWebhook} {
		if eps, ok := feature.EntryPoints[epType]; ok {
			for _, ep := range eps {
				event := EventDefinition{
					ID:          uuid.New(),
					FeatureID:   feature.FeatureID,
					Name:        fmt.Sprintf("%s_%s", feature.FeatureID.String()[:8], ep.GetName()),
					Description: fmt.Sprintf("Event emitted by %s entry point %s", epType, ep.GetName()),
					EventType:   fmt.Sprintf("%s.%s", epType, ep.GetName()),
					PayloadSchema: json.RawMessage(`{"type": "object"}`),
					PropagationRule: PropagationRuleBroadcast,
					Priority:    100,
					Async:       true,
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				}

				if epType == EntryPointTypeWebhook {
					event.Async = false
					event.PropagationRule = PropagationRuleDirect
				}

				if err := es.DefineEvent(ctx, feature, &event); err != nil {
					return nil, err
				}
				events = append(events, event)
			}
		}
	}

	return events, nil
}

func (es *EventSystem) GetEventFlow(ctx context.Context, featureID uuid.UUID) (*EventFlow, error) {
	es.mu.RLock()
	defer es.mu.RUnlock()

	if flow, ok := es.flows[featureID]; ok {
		return flow, nil
	}

	return es.buildEventFlow(ctx, featureID)
}

func (es *EventSystem) buildEventFlow(ctx context.Context, featureID uuid.UUID) (*EventFlow, error) {
	flow := &EventFlow{
		ID:          uuid.New(),
		FeatureID:   featureID,
		Graph:       &EventFlowGraph{Nodes: []EventFlowNode{}, Edges: []EventFlowEdge{}},
		EntryPoints: []uuid.UUID{},
		ExitPoints:  []uuid.UUID{},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	nodeMap := make(map[uuid.UUID]*EventFlowNode)

	for eventID, eventDef := range es.eventDefs {
		if eventDef.FeatureID != featureID {
			continue
		}

		eventNode := EventFlowNode{
			ID:        uuid.New(),
			EventID:   eventID,
			FeatureID: featureID,
			Type:      "event",
			Label:     eventDef.Name,
			Position:  Position{X: 0, Y: 0},
		}
		nodeMap[eventID] = &eventNode
		flow.Graph.Nodes = append(flow.Graph.Nodes, eventNode)
		flow.EntryPoints = append(flow.EntryPoints, eventID)
	}

	for eventID, handlers := range es.handlers {
		if _, ok := nodeMap[eventID]; !ok {
			continue
		}

		for _, handler := range handlers {
			handlerNode := EventFlowNode{
				ID:        handler.ID,
				EventID:   eventID,
				HandlerID: handler.ID,
				FeatureID: handler.FeatureID,
				Type:      "handler",
				Label:     handler.Function,
				Position:  Position{X: 100, Y: 0},
			}
			nodeMap[handler.ID] = &handlerNode
			flow.Graph.Nodes = append(flow.Graph.Nodes, handlerNode)

			edge := EventFlowEdge{
				ID:         uuid.New(),
				FromNodeID: eventID,
				ToNodeID:   handler.ID,
				Type:       "handles",
				Label:      "handles",
			}
			flow.Graph.Edges = append(flow.Graph.Edges, edge)
			flow.ExitPoints = append(flow.ExitPoints, handler.ID)
		}
	}

	es.detectCycles(flow)
	es.findCriticalPath(flow)

	es.flows[featureID] = flow
	return flow, nil
}

func (es *EventSystem) detectCycles(flow *EventFlow) {
	visited := make(map[uuid.UUID]bool)
	recStack := make(map[uuid.UUID]bool)
	path := []uuid.UUID{}

	var dfs func(nodeID uuid.UUID) bool
	dfs = func(nodeID uuid.UUID) bool {
		visited[nodeID] = true
		recStack[nodeID] = true
		path = append(path, nodeID)

		for _, edge := range flow.Graph.Edges {
			if edge.FromNodeID == nodeID {
				if !visited[edge.ToNodeID] {
					if dfs(edge.ToNodeID) {
						return true
					}
				} else if recStack[edge.ToNodeID] {
					cycleStart := -1
					for i, id := range path {
						if id == edge.ToNodeID {
							cycleStart = i
							break
						}
					}
					if cycleStart >= 0 {
						flow.Cycles = append(flow.Cycles, path[cycleStart:])
					}
					return true
				}
			}
		}

		recStack[nodeID] = false
		path = path[:len(path)-1]
		return false
	}

	for _, node := range flow.Graph.Nodes {
		if !visited[node.ID] {
			dfs(node.ID)
		}
	}
}

func (es *EventSystem) findCriticalPath(flow *EventFlow) {
	if len(flow.Graph.Nodes) == 0 {
		return
	}

	dist := make(map[uuid.UUID]int)
	prev := make(map[uuid.UUID]uuid.UUID)

	for _, node := range flow.Graph.Nodes {
		dist[node.ID] = -1
	}

	for _, entryID := range flow.EntryPoints {
		dist[entryID] = 0
	}

	for i := 0; i < len(flow.Graph.Nodes)-1; i++ {
		for _, edge := range flow.Graph.Edges {
			if dist[edge.FromNodeID] != -1 && dist[edge.FromNodeID]+1 > dist[edge.ToNodeID] {
				dist[edge.ToNodeID] = dist[edge.FromNodeID] + 1
				prev[edge.ToNodeID] = edge.FromNodeID
			}
		}
	}

	maxDist := -1
	var endNode uuid.UUID
	for _, exitID := range flow.ExitPoints {
		if dist[exitID] > maxDist {
			maxDist = dist[exitID]
			endNode = exitID
		}
	}

	if endNode != uuid.Nil {
		path := []uuid.UUID{}
		current := endNode
		for current != uuid.Nil {
			path = append([]uuid.UUID{current}, path...)
			current = prev[current]
		}
		flow.CriticalPath = path
	}
}

func (es *EventSystem) RegisterHandler(ctx context.Context, eventID uuid.UUID, handler *EventHandler) error {
	es.mu.Lock()
	defer es.mu.Unlock()

	handler.EventID = eventID
	if handler.ID == uuid.Nil {
		handler.ID = uuid.New()
	}

	es.handlers[eventID] = append(es.handlers[eventID], handler)
	return es.persistHandler(ctx, handler)
}

func (es *EventSystem) UnregisterHandler(ctx context.Context, eventID uuid.UUID, handlerID uuid.UUID) error {
	es.mu.Lock()
	defer es.mu.Unlock()

	handlers := es.handlers[eventID]
	for i, h := range handlers {
		if h.ID == handlerID {
			es.handlers[eventID] = append(handlers[:i], handlers[i+1:]...)
			break
		}
	}

	return es.deleteHandler(ctx, handlerID)
}

func (es *EventSystem) ValidateEventFlow(ctx context.Context, feature *FeatureAppearance) (*ValidationResult, error) {
	flow, err := es.GetEventFlow(ctx, feature.FeatureID)
	if err != nil {
		return nil, err
	}

	result := &ValidationResult{
		Errors:   []ValidationError{},
		Warnings: []string{},
		Valid:    true,
	}

	if len(flow.Cycles) > 0 {
		for _, cycle := range flow.Cycles {
			result.Errors = append(result.Errors, ValidationError{
				Code:       "EVENT_CYCLE_DETECTED",
				Message:    fmt.Sprintf("Event cycle detected: %v", cycle),
				Severity:   "error",
				Location:   "event_flow",
				FeatureID:  feature.FeatureID,
				Suggestion: "Remove cyclic event dependencies",
			})
			result.Valid = false
		}
	}

	for eventID, eventDef := range es.eventDefs {
		if eventDef.FeatureID != feature.FeatureID {
			continue
		}

		handlers := es.handlers[eventID]
		if len(handlers) == 0 && eventDef.PropagationRule != PropagationRuleDirect {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Event %s has no handlers", eventDef.Name))
		}

		for _, ep := range eventDef.EmissionPoints {
			if ep.Function == "" {
				result.Errors = append(result.Errors, ValidationError{
					Code:       "MISSING_EMISSION_FUNCTION",
					Message:    fmt.Sprintf("Event %s emission point missing function", eventDef.Name),
					Severity:   "error",
					Location:   ep.Location,
					FeatureID:  feature.FeatureID,
					Suggestion: "Implement emission function",
				})
				result.Valid = false
			}
		}

		for _, handler := range handlers {
			if handler.Function == "" {
				result.Errors = append(result.Errors, ValidationError{
					Code:       "MISSING_HANDLER_FUNCTION",
					Message:    fmt.Sprintf("Handler for event %s missing function", eventDef.Name),
					Severity:   "error",
					Location:   "handler",
					FeatureID:  feature.FeatureID,
					Suggestion: "Implement handler function",
				})
				result.Valid = false
			}
		}
	}

	unconnectedEvents := es.findUnconnectedEvents(feature.FeatureID)
	for _, eventID := range unconnectedEvents {
		if eventDef, ok := es.eventDefs[eventID]; ok {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Event %s is not connected to any entry/exit point", eventDef.Name))
		}
	}

	return result, nil
}

func (es *EventSystem) findUnconnectedEvents(featureID uuid.UUID) []uuid.UUID {
	connected := make(map[uuid.UUID]bool)

	for _, handler := range es.handlers {
		for _, h := range handler {
			connected[h.EventID] = true
			connected[h.ID] = true
		}
	}

	var unconnected []uuid.UUID
	for eventID, eventDef := range es.eventDefs {
		if eventDef.FeatureID == featureID && !connected[eventID] {
			unconnected = append(unconnected, eventID)
		}
	}

	return unconnected
}

func (es *EventSystem) GenerateEventCode(ctx context.Context, feature *FeatureAppearance) ([]GeneratedFile, error) {
	var files []GeneratedFile

	for _, eventDef := range es.eventDefs {
		if eventDef.FeatureID != feature.FeatureID {
			continue
		}

		eventFile := es.generateEventDefinition(eventDef)
		files = append(files, GeneratedFile{
			Path:      fmt.Sprintf("internal/events/%s.go", eventDef.Name),
			Content:   eventFile,
			Operation: "create",
		})

		for _, handler := range es.handlers[eventDef.ID] {
			handlerFile := es.generateEventHandler(eventDef, handler)
			files = append(files, GeneratedFile{
				Path:      fmt.Sprintf("internal/events/handlers/%s_%s.go", eventDef.Name, handler.Function),
				Content:   handlerFile,
				Operation: "create",
			})
		}
	}

	flow, err := es.GetEventFlow(ctx, feature.FeatureID)
	if err == nil {
		flowFile := es.generateEventFlow(flow)
		files = append(files, GeneratedFile{
			Path:      fmt.Sprintf("internal/events/flow_%s.go", feature.FeatureID.String()[:8]),
			Content:   flowFile,
			Operation: "create",
		})
	}

	return files, nil
}

func (es *EventSystem) generateEventDefinition(eventDef *EventDefinition) string {
	payloadType := "map[string]interface{}"
	var payloadSchema map[string]interface{}
	if err := json.Unmarshal(eventDef.PayloadSchema, &payloadSchema); err == nil {
		if t, ok := payloadSchema["type"].(string); ok && t != "object" {
			payloadType = t
		}
	}

	emissionCode := ""
	for _, ep := range eventDef.EmissionPoints {
		emissionCode += fmt.Sprintf(`
func Emit%s(ctx context.Context, payload %s) error {
	%s
	return events.Emit(ctx, %sEvent, payload)
}
`, eventDef.Name, payloadType, ep.PayloadBuilder, eventDef.Name)
	}

	return fmt.Sprintf(`package events

import (
	"context"
	"github.com/owecode/events"
)

var %sEvent = events.DefineEvent("%s", %s{})

%s

func Handle%s(handler func(ctx context.Context, payload %s) error, options ...events.HandlerOption) {
	events.RegisterHandler(%sEvent, handler, options...)
}

func Get%sEvent() *events.EventDefinition {
	return %sEvent
}
`, eventDef.Name, eventDef.EventType, payloadType, emissionCode, eventDef.Name, payloadType, eventDef.Name, eventDef.Name, eventDef.Name)
}

func (es *EventSystem) generateEventHandler(eventDef *EventDefinition, handler *EventHandler) string {
	async := "events.SyncHandler"
	if handler.Async {
		async = "events.AsyncHandler"
	}

	filterCode := ""
	if handler.Filter != "" {
		filterCode = fmt.Sprintf(`
	if !matchesFilter(payload, "%s") {
		return nil
	}
`, handler.Filter)
	}

	timeoutCode := ""
	if handler.Timeout > 0 {
		timeoutCode = fmt.Sprintf(`
	ctx, cancel := context.WithTimeout(ctx, %v)
	defer cancel()
`, handler.Timeout)
	}

	return fmt.Sprintf(`package handlers

import (
	"context"
	"github.com/owecode/events"
)

func %sHandler(ctx context.Context, payload %s) error {
%s%s
	// TODO: Implement handler logic for %s
	return nil
}

func init() {
	events.RegisterHandler(%sEvent, %sHandler, %s)
}
`, handler.Function, "interface{}", timeoutCode, filterCode, eventDef.Name, eventDef.Name, handler.Function, async)
}

func (es *EventSystem) generateEventFlow(flow *EventFlow) string {
	nodes := ""
	for _, node := range flow.Graph.Nodes {
		nodes += fmt.Sprintf(`		{
			ID:       "%s",
			EventID:  "%s",
			Type:     "%s",
			Label:    "%s",
			Position: Position{X: %f, Y: %f},
		},
`, node.ID, node.EventID, node.Type, node.Label, node.Position.X, node.Position.Y)
	}

	edges := ""
	for _, edge := range flow.Graph.Edges {
		edges += fmt.Sprintf(`		{
			ID:        "%s",
			FromNodeID: "%s",
			ToNodeID:   "%s",
			Type:      "%s",
			Label:     "%s",
		},
`, edge.ID, edge.FromNodeID, edge.ToNodeID, edge.Type, edge.Label)
	}

	return fmt.Sprintf(`package events

import "github.com/owecode/events"

var %sFlow = &events.EventFlow{
	ID: "%s",
	Graph: events.EventFlowGraph{
		Nodes: []events.EventFlowNode{
%s		},
		Edges: []events.EventFlowEdge{
%s		},
	},
	EntryPoints: []string{%s},
	ExitPoints:  []string{%s},
	CriticalPath: []string{%s},
}

func init() {
	events.RegisterFlow(%sFlow)
}
`, flow.FeatureID.String()[:8], flow.ID, nodes, edges, 
	formatUUIDSlice(flow.EntryPoints), formatUUIDSlice(flow.ExitPoints), formatUUIDSlice(flow.CriticalPath),
	flow.FeatureID.String()[:8])
}

func formatUUIDSlice(uuids []uuid.UUID) string {
	result := ""
	for i, id := range uuids {
		if i > 0 {
			result += ", "
		}
		result += fmt.Sprintf(`"%s"`, id)
	}
	return result
}

func (es *EventSystem) GetEventDefinitions(featureID uuid.UUID) []EventDefinition {
	es.mu.RLock()
	defer es.mu.RUnlock()

	var events []EventDefinition
	for _, eventDef := range es.eventDefs {
		if eventDef.FeatureID == featureID {
			events = append(events, *eventDef)
		}
	}
	return events
}

func (es *EventSystem) GetHandlers(eventID uuid.UUID) []*EventHandler {
	es.mu.RLock()
	defer es.mu.RUnlock()
	return es.handlers[eventID]
}

func (es *EventSystem) GetEventFlows(ctx context.Context, featureID uuid.UUID) ([]EventFlow, error) {
	flow, err := es.GetEventFlow(ctx, featureID)
	if err != nil {
		return nil, err
	}
	return []EventFlow{*flow}, nil
}

func (es *EventSystem) persistEventDefinition(ctx context.Context, eventDef *EventDefinition) error {
	data, err := json.Marshal(eventDef)
	if err != nil {
		return err
	}

	node, err := graph.NewNode(graph.NodeTypeEvent, map[string]interface{}{
		"event_definition": string(data),
	})
	if err != nil {
		return err
	}

	return es.store.CreateNode(ctx, node)
}

func (es *EventSystem) persistHandler(ctx context.Context, handler *EventHandler) error {
	data, err := json.Marshal(handler)
	if err != nil {
		return err
	}

	node, err := graph.NewNode(graph.NodeTypeEvent, map[string]interface{}{
		"event_handler": string(data),
	})
	if err != nil {
		return err
	}

	return es.store.CreateNode(ctx, node)
}

func (es *EventSystem) deleteHandler(ctx context.Context, handlerID uuid.UUID) error {
	nodes, err := es.store.ListNodes(ctx, graph.NodeTypeEvent, 1000, 0)
	if err != nil {
		return err
	}

	for _, node := range nodes {
		var data map[string]interface{}
		if err := node.UnmarshalData(&data); err != nil {
			continue
		}
		if handlerData, ok := data["event_handler"].(string); ok {
			var handler EventHandler
			if json.Unmarshal([]byte(handlerData), &handler) == nil && handler.ID == handlerID {
				return es.store.DeleteNode(ctx, node.ID)
			}
		}
	}

	return nil
}

func (es *EventSystem) LoadEvents(ctx context.Context) error {
	nodes, err := es.store.ListNodes(ctx, graph.NodeTypeEvent, 1000, 0)
	if err != nil {
		return err
	}

	for _, node := range nodes {
		var data map[string]interface{}
		if err := node.UnmarshalData(&data); err != nil {
			continue
		}

		if eventDefData, ok := data["event_definition"].(string); ok {
			var eventDef EventDefinition
			if json.Unmarshal([]byte(eventDefData), &eventDef) == nil {
				es.eventDefs[eventDef.ID] = &eventDef
			}
		}

		if handlerData, ok := data["event_handler"].(string); ok {
			var handler EventHandler
			if json.Unmarshal([]byte(handlerData), &handler) == nil {
				es.handlers[handler.EventID] = append(es.handlers[handler.EventID], &handler)
			}
		}
	}

	return nil
}