package analysis

import (
	"context"
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/iSundram/Automergent/internal/graph"
)

type WiringAnalyzer struct {
	store *graph.Store
	query *graph.QueryEngine
	mu    sync.RWMutex

	patternCache map[string][]WiringPattern
}

func NewWiringAnalyzer(store *graph.Store, query *graph.QueryEngine) *WiringAnalyzer {
	return &WiringAnalyzer{
		store:        store,
		query:        query,
		patternCache: make(map[string][]WiringPattern),
	}
}

func (wa *WiringAnalyzer) GetWiringPattern(ctx context.Context, featureID uuid.UUID) (*WiringPattern, error) {
	feature, err := wa.store.GetNode(ctx, featureID)
	if err != nil {
		return nil, err
	}

	var data map[string]interface{}
	if err := feature.UnmarshalData(&data); err != nil {
		return nil, err
	}

	pattern := &WiringPattern{
		PatternID:   uuid.New(),
		Name:        wa.extractPatternName(data),
		Description: wa.extractPatternDescription(data),
		Steps:       []WiringStep{},
		ConfigKeys:  []string{},
		Providers:   []string{},
		Clients:     []string{},
		RequestFlow: []string{},
		Metadata:    make(map[string]interface{}),
		Frequency:   1,
		Confidence:  0.8,
	}

	configKeys := wa.findConfigKeys(ctx, feature)
	pattern.ConfigKeys = configKeys

	providers := wa.findProviders(ctx, feature)
	pattern.Providers = providers

	clients := wa.findClients(ctx, feature)
	pattern.Clients = clients

	requestFlow := wa.findRequestFlow(ctx, feature)
	pattern.RequestFlow = requestFlow

	steps := wa.buildWiringSteps(configKeys, providers, clients, requestFlow)
	pattern.Steps = steps

	return pattern, nil
}

func (wa *WiringAnalyzer) findConfigKeys(ctx context.Context, feature *graph.Node) []string {
	var keys []string

	edges, err := wa.store.GetEdgesFrom(ctx, feature.ID, graph.EdgeTypeContains)
	if err != nil {
		return keys
	}

	for _, edge := range edges {
		node, err := wa.store.GetNode(ctx, edge.ToID)
		if err != nil {
			continue
		}

		if node.Type == graph.NodeTypeFile {
			var data map[string]interface{}
			if err := node.UnmarshalData(&data); err != nil {
				continue
			}

			if path, ok := data["path"].(string); ok {
				if wa.isConfigFile(path) {
					keys = append(keys, path)
				}
			}
		}

		if node.Type == graph.NodeTypeMemory {
			var data map[string]interface{}
			if err := node.UnmarshalData(&data); err != nil {
				continue
			}

			if memType, ok := data["type"].(string); ok && memType == "config" {
				if key, ok := data["key"].(string); ok {
					keys = append(keys, key)
				}
			}
		}
	}

	return wa.deduplicate(keys)
}

func (wa *WiringAnalyzer) findProviders(ctx context.Context, feature *graph.Node) []string {
	var providers []string

	edges, err := wa.store.GetEdgesTo(ctx, feature.ID, graph.EdgeTypeDependsOn)
	if err != nil {
		return providers
	}

	for _, edge := range edges {
		node, err := wa.store.GetNode(ctx, edge.FromID)
		if err != nil {
			continue
		}

		var data map[string]interface{}
		if err := node.UnmarshalData(&data); err != nil {
			continue
		}

		if name, ok := data["name"].(string); ok {
			if wa.isProvider(name, data) {
				providers = append(providers, name)
			}
		}
	}

	edges, err = wa.store.GetEdgesFrom(ctx, feature.ID, graph.EdgeTypeReferences)
	if err != nil {
		return providers
	}

	for _, edge := range edges {
		node, err := wa.store.GetNode(ctx, edge.ToID)
		if err != nil {
			continue
		}

		var data map[string]interface{}
		if err := node.UnmarshalData(&data); err != nil {
			continue
		}

		if name, ok := data["name"].(string); ok {
			if wa.isProvider(name, data) {
				providers = append(providers, name)
			}
		}
	}

	return wa.deduplicate(providers)
}

func (wa *WiringAnalyzer) findClients(ctx context.Context, feature *graph.Node) []string {
	var clients []string

	edges, err := wa.store.GetEdgesFrom(ctx, feature.ID, graph.EdgeTypeDependsOn)
	if err != nil {
		return clients
	}

	for _, edge := range edges {
		node, err := wa.store.GetNode(ctx, edge.ToID)
		if err != nil {
			continue
		}

		var data map[string]interface{}
		if err := node.UnmarshalData(&data); err != nil {
			continue
		}

		if name, ok := data["name"].(string); ok {
			if wa.isClient(name, data) {
				clients = append(clients, name)
			}
		}
	}

	edges, err = wa.store.GetEdgesTo(ctx, feature.ID, graph.EdgeTypeReferences)
	if err != nil {
		return clients
	}

	for _, edge := range edges {
		node, err := wa.store.GetNode(ctx, edge.FromID)
		if err != nil {
			continue
		}

		var data map[string]interface{}
		if err := node.UnmarshalData(&data); err != nil {
			continue
		}

		if name, ok := data["name"].(string); ok {
			if wa.isClient(name, data) {
				clients = append(clients, name)
			}
		}
	}

	return wa.deduplicate(clients)
}

func (wa *WiringAnalyzer) findRequestFlow(ctx context.Context, feature *graph.Node) []string {
	var flow []string

	edges, err := wa.store.GetEdgesFrom(ctx, feature.ID, graph.EdgeTypeContains)
	if err != nil {
		return flow
	}

	for _, edge := range edges {
		node, err := wa.store.GetNode(ctx, edge.ToID)
		if err != nil {
			continue
		}

		if node.Type == graph.NodeTypeFile {
			var data map[string]interface{}
			if err := node.UnmarshalData(&data); err != nil {
				continue
			}

			if path, ok := data["path"].(string); ok {
				if wa.isRequestHandler(path) {
					flow = append(flow, path)
				}
			}
		}
	}

	return wa.deduplicate(flow)
}

func (wa *WiringAnalyzer) buildWiringSteps(configKeys, providers, clients, requestFlow []string) []WiringStep {
	var steps []WiringStep
	stepOrder := 0

	for _, key := range configKeys {
		steps = append(steps, WiringStep{
			StepOrder:   stepOrder,
			FromType:    "config",
			ToType:      "provider",
			EdgeType:    "configures",
			Description: "Config key " + key + " configures provider",
			Properties:  map[string]interface{}{"config_key": key},
		})
		stepOrder++
	}

	for _, provider := range providers {
		steps = append(steps, WiringStep{
			StepOrder:   stepOrder,
			FromType:    "provider",
			ToType:      "client",
			EdgeType:    "provides",
			Description: "Provider " + provider + " provides service to client",
			Properties:  map[string]interface{}{"provider": provider},
		})
		stepOrder++
	}

	for _, client := range clients {
		steps = append(steps, WiringStep{
			StepOrder:   stepOrder,
			FromType:    "client",
			ToType:      "request",
			EdgeType:    "calls",
			Description: "Client " + client + " makes request",
			Properties:  map[string]interface{}{"client": client},
		})
		stepOrder++
	}

	for _, req := range requestFlow {
		steps = append(steps, WiringStep{
			StepOrder:   stepOrder,
			FromType:    "request",
			ToType:      "handler",
			EdgeType:    "handles",
			Description: "Request handled by " + req,
			Properties:  map[string]interface{}{"handler": req},
		})
		stepOrder++
	}

	return steps
}

func (wa *WiringAnalyzer) SuggestWiring(ctx context.Context, newFeature *graph.Node, similarFeatures []FeatureMatch) (*WiringPattern, error) {
	if len(similarFeatures) == 0 {
		return wa.GetWiringPattern(ctx, newFeature.ID)
	}

	patterns := make([]*WiringPattern, 0, len(similarFeatures))
	for _, match := range similarFeatures {
		pattern, err := wa.GetWiringPattern(ctx, match.FeatureID)
		if err == nil {
			patterns = append(patterns, pattern)
		}
	}

	if len(patterns) == 0 {
		return wa.GetWiringPattern(ctx, newFeature.ID)
	}

	merged := wa.mergePatterns(patterns)
	merged.PatternID = uuid.New()
	merged.Name = "suggested_" + wa.extractPatternNameFromFeature(newFeature)
	merged.Description = "Suggested wiring based on " + wa.intToString(len(patterns)) + " similar features"
	merged.Confidence = wa.calculateConfidence(patterns)

	return merged, nil
}

func (wa *WiringAnalyzer) mergePatterns(patterns []*WiringPattern) *WiringPattern {
	if len(patterns) == 0 {
		return &WiringPattern{}
	}

	merged := &WiringPattern{
		Steps:       []WiringStep{},
		ConfigKeys:  []string{},
		Providers:   []string{},
		Clients:     []string{},
		RequestFlow: []string{},
		Metadata:    make(map[string]interface{}),
	}

	stepFreq := make(map[string]int)
	configFreq := make(map[string]int)
	providerFreq := make(map[string]int)
	clientFreq := make(map[string]int)
	flowFreq := make(map[string]int)

	for _, p := range patterns {
		for _, step := range p.Steps {
			key := step.FromType + "->" + step.ToType + ":" + step.Description
			stepFreq[key]++
		}
		for _, k := range p.ConfigKeys {
			configFreq[k]++
		}
		for _, p := range p.Providers {
			providerFreq[p]++
		}
		for _, c := range p.Clients {
			clientFreq[c]++
		}
		for _, f := range p.RequestFlow {
			flowFreq[f]++
		}
	}

	threshold := len(patterns) / 2
	if threshold == 0 {
		threshold = 1
	}

	for step, freq := range stepFreq {
		if freq >= threshold {
			parts := strings.Split(step, ":")
			if len(parts) == 2 {
				fromTo := strings.Split(parts[0], "->")
				if len(fromTo) == 2 {
					merged.Steps = append(merged.Steps, WiringStep{
						FromType:   fromTo[0],
						ToType:     fromTo[1],
						EdgeType:   "suggested",
						Description: parts[1],
					})
				}
			}
		}
	}

	for k, freq := range configFreq {
		if freq >= threshold {
			merged.ConfigKeys = append(merged.ConfigKeys, k)
		}
	}
	for p, freq := range providerFreq {
		if freq >= threshold {
			merged.Providers = append(merged.Providers, p)
		}
	}
	for c, freq := range clientFreq {
		if freq >= threshold {
			merged.Clients = append(merged.Clients, c)
		}
	}
	for f, freq := range flowFreq {
		if freq >= threshold {
			merged.RequestFlow = append(merged.RequestFlow, f)
		}
	}

	wa.sortStepsByOrder(merged.Steps)

	return merged
}

func (wa *WiringAnalyzer) GetIntegrationPath(ctx context.Context, from, to uuid.UUID) (*IntegrationPath, error) {
	result, err := wa.query.ShortestPath(ctx, from, to, nil)
	if err != nil {
		return nil, err
	}

	if result == nil || len(result.Path) == 0 {
		return nil, nil
	}

	path := &IntegrationPath{
		PathID:      uuid.New(),
		FromFeature: from,
		ToFeature:   to,
		Steps:       []PathStep{},
		TotalCost:   result.Cost,
		Confidence:  wa.calculatePathConfidence(result),
		Description: "Integration path from " + from.String() + " to " + to.String(),
	}

	for i := 0; i < len(result.Path)-1; i++ {
		fromNode := result.Path[i]
		toNode := result.Path[i+1]

		var edge *graph.Edge
		for _, e := range result.Edges {
			if e.FromID == fromNode.ID && e.ToID == toNode.ID {
				edge = e
				break
			}
		}

		step := PathStep{
			StepOrder:  i,
			FromNodeID:  fromNode.ID,
			ToNodeID:   toNode.ID,
			EdgeType:   string(edge.Type),
			Description: wa.describeStep(fromNode, toNode, edge),
		}

		if edge != nil {
			var edgeData map[string]interface{}
			edge.UnmarshalData(&edgeData)
			step.Properties = edgeData
		}

		path.Steps = append(path.Steps, step)
	}

	return path, nil
}

func (wa *WiringAnalyzer) calculatePathConfidence(result *graph.ShortestPathResult) float64 {
	if result == nil || len(result.Path) <= 1 {
		return 0
	}

	confidence := 1.0
	for _, edge := range result.Edges {
		switch edge.Type {
		case graph.EdgeTypeDependsOn, graph.EdgeTypeReferences, graph.EdgeTypeContains:
			confidence *= 0.9
		case graph.EdgeTypeRelatesTo:
			confidence *= 0.7
		default:
			confidence *= 0.5
		}
	}

	return math.Max(confidence, 0.1)
}

func (wa *WiringAnalyzer) describeStep(from, to *graph.Node, edge *graph.Edge) string {
	fromName := wa.getNodeName(from)
	toName := wa.getNodeName(to)

	if edge == nil {
		return fromName + " -> " + toName
	}

	return fromName + " --" + string(edge.Type) + "--> " + toName
}

func (wa *WiringAnalyzer) getNodeName(node *graph.Node) string {
	var data map[string]interface{}
	if err := node.UnmarshalData(&data); err != nil {
		return node.ID.String()[:8]
	}
	if name, ok := data["name"].(string); ok {
		return name
	}
	if title, ok := data["title"].(string); ok {
		return title
	}
	return string(node.Type) + "_" + node.ID.String()[:8]
}

func (wa *WiringAnalyzer) isConfigFile(path string) bool {
	return strings.Contains(path, "config") ||
		strings.Contains(path, "settings") ||
		strings.HasSuffix(path, ".yaml") ||
		strings.HasSuffix(path, ".yml") ||
		strings.HasSuffix(path, ".json") ||
		strings.HasSuffix(path, ".toml") ||
		strings.HasSuffix(path, ".ini")
}

func (wa *WiringAnalyzer) isProvider(name string, data map[string]interface{}) bool {
	nameLower := strings.ToLower(name)
	return strings.Contains(nameLower, "provider") ||
		strings.Contains(nameLower, "service") ||
		strings.Contains(nameLower, "repository") ||
		strings.Contains(nameLower, "store") ||
		strings.Contains(nameLower, "manager") ||
		strings.Contains(nameLower, "factory")
}

func (wa *WiringAnalyzer) isClient(name string, data map[string]interface{}) bool {
	nameLower := strings.ToLower(name)
	return strings.Contains(nameLower, "client") ||
		strings.Contains(nameLower, "consumer") ||
		strings.Contains(nameLower, "handler") ||
		strings.Contains(nameLower, "controller") ||
		strings.Contains(nameLower, "endpoint") ||
		strings.Contains(nameLower, "api")
}

func (wa *WiringAnalyzer) isRequestHandler(path string) bool {
	pathLower := strings.ToLower(path)
	return strings.Contains(pathLower, "handler") ||
		strings.Contains(pathLower, "endpoint") ||
		strings.Contains(pathLower, "route") ||
		strings.Contains(pathLower, "api") ||
		strings.Contains(pathLower, "request")
}

func (wa *WiringAnalyzer) extractPatternName(data map[string]interface{}) string {
	if name, ok := data["name"].(string); ok {
		return name + "_wiring"
	}
	return "unknown_wiring"
}

func (wa *WiringAnalyzer) extractPatternDescription(data map[string]interface{}) string {
	if desc, ok := data["description"].(string); ok {
		return "Wiring pattern for " + desc
	}
	return "Wiring pattern"
}

func (wa *WiringAnalyzer) extractPatternNameFromFeature(feature *graph.Node) string {
	var data map[string]interface{}
	if err := feature.UnmarshalData(&data); err != nil {
		return "feature"
	}
	if name, ok := data["name"].(string); ok {
		return name
	}
	return "feature"
}

func (wa *WiringAnalyzer) calculateConfidence(patterns []*WiringPattern) float64 {
	if len(patterns) == 0 {
		return 0
	}

	total := 0.0
	for _, p := range patterns {
		total += p.Confidence
	}
	return total / float64(len(patterns))
}

func (wa *WiringAnalyzer) sortStepsByOrder(steps []WiringStep) {
	sort.Slice(steps, func(i, j int) bool {
		return steps[i].StepOrder < steps[j].StepOrder
	})
	for i := range steps {
		steps[i].StepOrder = i
	}
}

func (wa *WiringAnalyzer) deduplicate(slice []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, s := range slice {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

func (wa *WiringAnalyzer) intToString(n int) string {
	if n == 1 {
		return "1"
	}
	return "multiple"
}