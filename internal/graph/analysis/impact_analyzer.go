package analysis

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/iSundram/Automergent/internal/graph"
)

type ImpactAnalyzer struct {
	store *graph.Store
	query *graph.QueryEngine
	mu    sync.RWMutex

	blastRadiusCache map[uuid.UUID]*ImpactScope
	chainCache       map[uuid.UUID]*DependencyChain
}

func NewImpactAnalyzer(store *graph.Store, query *graph.QueryEngine) *ImpactAnalyzer {
	return &ImpactAnalyzer{
		store:            store,
		query:            query,
		blastRadiusCache: make(map[uuid.UUID]*ImpactScope),
		chainCache:       make(map[uuid.UUID]*DependencyChain),
	}
}

func (ia *ImpactAnalyzer) GetImpactScope(ctx context.Context, filePath string) (*ImpactScope, error) {
	nodeID, err := ia.findNodeByFilePath(ctx, filePath)
	if err != nil {
		return nil, err
	}

	if nodeID == uuid.Nil {
		return nil, fmt.Errorf("file not found in graph: %s", filePath)
	}

	ia.mu.RLock()
	if cached, ok := ia.blastRadiusCache[nodeID]; ok {
		ia.mu.RUnlock()
		return cached, nil
	}
	ia.mu.RUnlock()

	scope := &ImpactScope{
		ScopeID:      uuid.New(),
		SourceFile:   filePath,
		SourceNodeID: nodeID,
		GeneratedAt:  time.Now(),
	}

	features := ia.findAffectedFeatures(ctx, nodeID, filePath)
	scope.AffectedFeatures = features

	tests := ia.findAffectedTests(ctx, nodeID)
	scope.AffectedTests = tests

	deps := ia.findDependencies(ctx, nodeID)
	scope.Dependencies = deps

	dependents := ia.findDependents(ctx, nodeID)
	scope.Dependents = dependents

	configs := ia.findConfigFiles(ctx, nodeID)
	scope.ConfigFiles = configs

	scope.BlastRadius = len(features) + len(tests) + len(deps) + len(dependents) + len(configs)
	scope.RiskLevel = ia.calculateRiskLevel(scope)

	ia.mu.Lock()
	ia.blastRadiusCache[nodeID] = scope
	ia.mu.Unlock()

	return scope, nil
}

func (ia *ImpactAnalyzer) findNodeByFilePath(ctx context.Context, filePath string) (uuid.UUID, error) {
	nodes, err := ia.store.ListNodes(ctx, graph.NodeTypeFile, 0, 0)
	if err != nil {
		return uuid.Nil, err
	}

	for _, node := range nodes {
		var data map[string]interface{}
		if err := node.UnmarshalData(&data); err != nil {
			continue
		}
		if path, ok := data["path"].(string); ok && path == filePath {
			return node.ID, nil
		}
	}

	return uuid.Nil, nil
}

func (ia *ImpactAnalyzer) findAffectedFeatures(ctx context.Context, nodeID uuid.UUID, sourceFile string) []FeatureImpact {
	var impacts []FeatureImpact

	visited := make(map[uuid.UUID]bool)
	var traverse func(currentID uuid.UUID, path []string, depth int)
	traverse = func(currentID uuid.UUID, path []string, depth int) {
		if depth > 5 || visited[currentID] {
			return
		}
		visited[currentID] = true

		node, err := ia.store.GetNode(ctx, currentID)
		if err != nil {
			return
		}

		if node.Type == graph.NodeTypeTask {
			var data map[string]interface{}
			if err := node.UnmarshalData(&data); err == nil {
				name := ""
				if n, ok := data["name"].(string); ok {
					name = n
				} else if t, ok := data["title"].(string); ok {
					name = t
				}

				if name != "" {
					severity := 1.0 - (float64(depth) * 0.15)
					if severity < 0.1 {
						severity = 0.1
					}

					impacts = append(impacts, FeatureImpact{
						FeatureID:   node.ID,
						FeatureName: name,
						ImpactType:  ia.determineImpactType(node, depth),
						Severity:    severity,
						Path:        append([]string{}, path...),
					})
				}
			}
		}

		edges, _ := ia.store.GetEdgesFrom(ctx, currentID, "")
		for _, edge := range edges {
			newPath := append(append([]string{}, path...), fmt.Sprintf("%s(%s)", ia.getNodeName(node), string(edge.Type)))
			traverse(edge.ToID, newPath, depth+1)
		}

		edges, _ = ia.store.GetEdgesTo(ctx, currentID, "")
		for _, edge := range edges {
			newPath := append(append([]string{}, path...), fmt.Sprintf("%s(%s)", ia.getNodeName(node), string(edge.Type)))
			traverse(edge.FromID, newPath, depth+1)
		}
	}

	traverse(nodeID, []string{sourceFile}, 0)

	sort.Slice(impacts, func(i, j int) bool {
		return impacts[i].Severity > impacts[j].Severity
	})

	return impacts
}

func (ia *ImpactAnalyzer) findAffectedTests(ctx context.Context, nodeID uuid.UUID) []TestImpact {
	var impacts []TestImpact

	nodes, err := ia.store.ListNodes(ctx, graph.NodeTypeFile, 0, 0)
	if err != nil {
		return impacts
	}

	for _, node := range nodes {
		var data map[string]interface{}
		if err := node.UnmarshalData(&data); err != nil {
			continue
		}

		path, _ := data["path"].(string)
		if !strings.Contains(path, "_test.") && !strings.Contains(path, "test_") && !strings.Contains(path, "/test/") {
			continue
		}

		result, err := ia.query.ShortestPath(ctx, node.ID, nodeID, []graph.EdgeType{
			graph.EdgeTypeDependsOn,
			graph.EdgeTypeReferences,
			graph.EdgeTypeContains,
		})
		if err != nil || result == nil {
			continue
		}

		distance := float64(len(result.Path) - 1)
		if distance > 4 {
			continue
		}

		name := ""
		if n, ok := data["name"].(string); ok {
			name = n
		} else {
			name = path
		}

		coverage := 1.0 - (distance * 0.2)
		if coverage < 0.2 {
			coverage = 0.2
		}

		severity := coverage

		impacts = append(impacts, TestImpact{
			TestFile: path,
			TestName: name,
			Coverage: coverage,
			Severity: severity,
		})
	}

	sort.Slice(impacts, func(i, j int) bool {
		return impacts[i].Severity > impacts[j].Severity
	})

	return impacts
}

func (ia *ImpactAnalyzer) findDependencies(ctx context.Context, nodeID uuid.UUID) []DependencyImpact {
	var impacts []DependencyImpact

	visited := make(map[uuid.UUID]bool)
	var traverse func(currentID uuid.UUID, depth int)
	traverse = func(currentID uuid.UUID, depth int) {
		if depth > 5 || visited[currentID] {
			return
		}
		visited[currentID] = true

		edges, err := ia.store.GetEdgesFrom(ctx, currentID, graph.EdgeTypeDependsOn)
		if err != nil {
			return
		}

		for _, edge := range edges {
			depNode, err := ia.store.GetNode(ctx, edge.ToID)
			if err != nil {
				continue
			}

			var data map[string]interface{}
			depNode.UnmarshalData(&data)

			name := ia.getNodeName(depNode)
			depType := ia.getNodeTypeName(depNode.Type)

			severity := 1.0 - (float64(depth) * 0.15)
			if severity < 0.1 {
				severity = 0.1
			}

			impacts = append(impacts, DependencyImpact{
				DepID:       depNode.ID,
				DepName:     name,
				DepType:     depType,
				ImpactDepth: depth,
				Severity:    severity,
			})

			traverse(edge.ToID, depth+1)
		}
	}

	traverse(nodeID, 1)

	sort.Slice(impacts, func(i, j int) bool {
		return impacts[i].Severity > impacts[j].Severity
	})

	return impacts
}

func (ia *ImpactAnalyzer) findDependents(ctx context.Context, nodeID uuid.UUID) []DependentImpact {
	var impacts []DependentImpact

	visited := make(map[uuid.UUID]bool)
	var traverse func(currentID uuid.UUID, depth int)
	traverse = func(currentID uuid.UUID, depth int) {
		if depth > 5 || visited[currentID] {
			return
		}
		visited[currentID] = true

		edges, err := ia.store.GetEdgesTo(ctx, currentID, graph.EdgeTypeDependsOn)
		if err != nil {
			return
		}

		for _, edge := range edges {
			depNode, err := ia.store.GetNode(ctx, edge.FromID)
			if err != nil {
				continue
			}

			var data map[string]interface{}
			depNode.UnmarshalData(&data)

			name := ia.getNodeName(depNode)
			depType := ia.getNodeTypeName(depNode.Type)

			severity := 1.0 - (float64(depth) * 0.15)
			if severity < 0.1 {
				severity = 0.1
			}

			impacts = append(impacts, DependentImpact{
				DepID:       depNode.ID,
				DepName:     name,
				DepType:     depType,
				ImpactDepth: depth,
				Severity:    severity,
			})

			traverse(edge.FromID, depth+1)
		}

		edges, err = ia.store.GetEdgesTo(ctx, currentID, graph.EdgeTypeReferences)
		if err != nil {
			return
		}

		for _, edge := range edges {
			if visited[edge.FromID] {
				continue
			}
			depNode, err := ia.store.GetNode(ctx, edge.FromID)
			if err != nil {
				continue
			}

			var data map[string]interface{}
			depNode.UnmarshalData(&data)

			name := ia.getNodeName(depNode)
			depType := ia.getNodeTypeName(depNode.Type)

			severity := 1.0 - (float64(depth) * 0.2)
			if severity < 0.1 {
				severity = 0.1
			}

			impacts = append(impacts, DependentImpact{
				DepID:       depNode.ID,
				DepName:     name,
				DepType:     depType,
				ImpactDepth: depth,
				Severity:    severity,
			})

			traverse(edge.FromID, depth+1)
		}
	}

	traverse(nodeID, 1)

	sort.Slice(impacts, func(i, j int) bool {
		return impacts[i].Severity > impacts[j].Severity
	})

	return impacts
}

func (ia *ImpactAnalyzer) findConfigFiles(ctx context.Context, nodeID uuid.UUID) []string {
	var configs []string

	visited := make(map[uuid.UUID]bool)
	var traverse func(currentID uuid.UUID)
	traverse = func(currentID uuid.UUID) {
		if visited[currentID] {
			return
		}
		visited[currentID] = true

		node, err := ia.store.GetNode(ctx, currentID)
		if err != nil {
			return
		}

		if node.Type == graph.NodeTypeFile {
			var data map[string]interface{}
			if err := node.UnmarshalData(&data); err == nil {
				if path, ok := data["path"].(string); ok {
					if ia.isConfigFile(path) {
						configs = append(configs, path)
					}
				}
			}
		}

		edges, _ := ia.store.GetEdgesFrom(ctx, currentID, graph.EdgeTypeContains)
		for _, edge := range edges {
			traverse(edge.ToID)
		}

		edges, _ = ia.store.GetEdgesTo(ctx, currentID, graph.EdgeTypeContains)
		for _, edge := range edges {
			traverse(edge.FromID)
		}
	}

	traverse(nodeID)

	return ia.deduplicate(configs)
}

func (ia *ImpactAnalyzer) GetBlastRadius(ctx context.Context, change ChangeRequest) (*ImpactScope, error) {
	var filePath string
	var err error

	if change.FilePath != "" {
		filePath = change.FilePath
	} else if change.NodeID != uuid.Nil {
		filePath, err = ia.findFilePathByNodeID(ctx, change.NodeID)
		if err != nil {
			return nil, err
		}
		if filePath == "" {
			return nil, fmt.Errorf("file path not found for node: %s", change.NodeID)
		}
	} else {
		return nil, fmt.Errorf("either file_path or node_id must be provided")
	}

	scope, err := ia.GetImpactScope(ctx, filePath)
	if err != nil {
		return nil, err
	}

	if change.ChangeType != "" {
		scope = ia.adjustScopeForChangeType(scope, change.ChangeType)
	}

	return scope, nil
}

func (ia *ImpactAnalyzer) findFilePathByNodeID(ctx context.Context, nodeID uuid.UUID) (string, error) {
	node, err := ia.store.GetNode(ctx, nodeID)
	if err != nil {
		return "", err
	}

	var data map[string]interface{}
	if err := node.UnmarshalData(&data); err != nil {
		return "", err
	}

	if path, ok := data["path"].(string); ok {
		return path, nil
	}

	return "", nil
}

type ChangeRequest struct {
	FilePath   string `json:"file_path"`
	NodeID     uuid.UUID `json:"node_id"`
	ChangeType string `json:"change_type"`
}

func (ia *ImpactAnalyzer) adjustScopeForChangeType(scope *ImpactScope, changeType string) *ImpactScope {
	multiplier := 1.0

	switch strings.ToLower(changeType) {
	case "delete", "remove":
		multiplier = 1.5
	case "modify", "update":
		multiplier = 1.0
	case "refactor":
		multiplier = 1.2
	case "config":
		multiplier = 0.8
	case "api":
		multiplier = 1.3
	}

	for i := range scope.AffectedFeatures {
		scope.AffectedFeatures[i].Severity *= multiplier
		if scope.AffectedFeatures[i].Severity > 1.0 {
			scope.AffectedFeatures[i].Severity = 1.0
		}
	}
	for i := range scope.AffectedTests {
		scope.AffectedTests[i].Severity *= multiplier
		if scope.AffectedTests[i].Severity > 1.0 {
			scope.AffectedTests[i].Severity = 1.0
		}
	}
	for i := range scope.Dependencies {
		scope.Dependencies[i].Severity *= multiplier
		if scope.Dependencies[i].Severity > 1.0 {
			scope.Dependencies[i].Severity = 1.0
		}
	}
	for i := range scope.Dependents {
		scope.Dependents[i].Severity *= multiplier
		if scope.Dependents[i].Severity > 1.0 {
			scope.Dependents[i].Severity = 1.0
		}
	}

	scope.BlastRadius = int(float64(scope.BlastRadius) * multiplier)
	scope.RiskLevel = ia.calculateRiskLevel(scope)

	return scope
}

func (ia *ImpactAnalyzer) GetDependencyChain(ctx context.Context, filePath string) (*DependencyChain, error) {
	nodeID, err := ia.findNodeByFilePath(ctx, filePath)
	if err != nil {
		return nil, err
	}

	if nodeID == uuid.Nil {
		return nil, fmt.Errorf("file not found in graph: %s", filePath)
	}

	ia.mu.RLock()
	if cached, ok := ia.chainCache[nodeID]; ok {
		ia.mu.RUnlock()
		return cached, nil
	}
	ia.mu.RUnlock()

	chain := &DependencyChain{
		ChainID:    uuid.New(),
		RootNodeID: nodeID,
		RootFile:   filePath,
		Nodes:      []ChainNode{},
		MaxDepth:   0,
	}

	visited := make(map[uuid.UUID]bool)
	circularRefs := make(map[string][]uuid.UUID)

	var traverse func(currentID uuid.UUID, depth int, path []uuid.UUID)
	traverse = func(currentID uuid.UUID, depth int, path []uuid.UUID) {
		if visited[currentID] {
			cycleKey := fmt.Sprintf("%v", path)
			circularRefs[cycleKey] = append(path, currentID)
			return
		}

		if depth > 10 {
			return
		}

		visited[currentID] = true

		node, err := ia.store.GetNode(ctx, currentID)
		if err != nil {
			return
		}

		var data map[string]interface{}
		node.UnmarshalData(&data)

		filePath := ""
		if p, ok := data["path"].(string); ok {
			filePath = p
		}

		depType := ""
		if depth > 0 {
			depType = "depends_on"
		}

		chain.Nodes = append(chain.Nodes, ChainNode{
			NodeID:     currentID,
			NodeType:   string(node.Type),
			FilePath:   filePath,
			Depth:      depth,
			DepType:    depType,
			IsOptional: ia.isOptionalDependency(data),
			IsExternal: ia.isExternalDependency(data),
		})

		if depth > chain.MaxDepth {
			chain.MaxDepth = depth
		}

		edges, _ := ia.store.GetEdgesFrom(ctx, currentID, graph.EdgeTypeDependsOn)
		for _, edge := range edges {
			newPath := append(append([]uuid.UUID{}, path...), currentID)
			traverse(edge.ToID, depth+1, newPath)
		}

		edges, _ = ia.store.GetEdgesFrom(ctx, currentID, graph.EdgeTypeReferences)
		for _, edge := range edges {
			newPath := append(append([]uuid.UUID{}, path...), currentID)
			traverse(edge.ToID, depth+1, newPath)
		}
	}

	traverse(nodeID, 0, []uuid.UUID{})

	chain.TotalNodes = len(chain.Nodes)

	for _, nodes := range circularRefs {
		files := make([]string, len(nodes))
		for i, n := range nodes {
			node, _ := ia.store.GetNode(ctx, n)
			var data map[string]interface{}
			if node != nil && node.UnmarshalData(&data) == nil {
				if p, ok := data["path"].(string); ok {
					files[i] = p
				}
			}
		}
		chain.CircularRefs = append(chain.CircularRefs, CircularRef{
			Nodes:    nodes,
			Files:    files,
			Severity: 0.8,
		})
	}

	ia.mu.Lock()
	ia.chainCache[nodeID] = chain
	ia.mu.Unlock()

	return chain, nil
}

func (ia *ImpactAnalyzer) determineImpactType(node *graph.Node, depth int) string {
	if depth == 0 {
		return "direct"
	} else if depth <= 2 {
		return "transitive"
	}
	return "indirect"
}

func (ia *ImpactAnalyzer) calculateRiskLevel(scope *ImpactScope) RiskLevel {
	maxSeverity := 0.0
	for _, f := range scope.AffectedFeatures {
		if f.Severity > maxSeverity {
			maxSeverity = f.Severity
		}
	}
	for _, t := range scope.AffectedTests {
		if t.Severity > maxSeverity {
			maxSeverity = t.Severity
		}
	}
	for _, d := range scope.Dependencies {
		if d.Severity > maxSeverity {
			maxSeverity = d.Severity
		}
	}
	for _, d := range scope.Dependents {
		if d.Severity > maxSeverity {
			maxSeverity = d.Severity
		}
	}

	if maxSeverity >= 0.8 || scope.BlastRadius > 20 {
		return RiskLevelCritical
	} else if maxSeverity >= 0.6 || scope.BlastRadius > 10 {
		return RiskLevelHigh
	} else if maxSeverity >= 0.4 || scope.BlastRadius > 5 {
		return RiskLevelMedium
	}
	return RiskLevelLow
}

func (ia *ImpactAnalyzer) getNodeName(node *graph.Node) string {
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

func (ia *ImpactAnalyzer) getNodeTypeName(nodeType graph.NodeType) string {
	return string(nodeType)
}

func (ia *ImpactAnalyzer) isConfigFile(path string) bool {
	return strings.Contains(path, "config") ||
		strings.Contains(path, "settings") ||
		strings.HasSuffix(path, ".yaml") ||
		strings.HasSuffix(path, ".yml") ||
		strings.HasSuffix(path, ".json") ||
		strings.HasSuffix(path, ".toml") ||
		strings.HasSuffix(path, ".ini")
}

func (ia *ImpactAnalyzer) isOptionalDependency(data map[string]interface{}) bool {
	if opt, ok := data["optional"].(bool); ok {
		return opt
	}
	return false
}

func (ia *ImpactAnalyzer) isExternalDependency(data map[string]interface{}) bool {
	if ext, ok := data["external"].(bool); ok {
		return ext
	}
	if source, ok := data["source"].(string); ok {
		return strings.Contains(source, "github.com") ||
			strings.Contains(source, "golang.org") ||
			strings.Contains(source, "gopkg.in")
	}
	return false
}

func (ia *ImpactAnalyzer) deduplicate(slice []string) []string {
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