package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type BuildCommandAnalyzer struct {
	mu           sync.RWMutex
	graph        *CommandGraph
	store        GraphStore
	logger       *zap.Logger
	projectRoot  string
	commandCache map[string]*CommandNode
}

func NewBuildCommandAnalyzer(projectRoot string, store GraphStore, logger *zap.Logger) *BuildCommandAnalyzer {
	return &BuildCommandAnalyzer{
		graph:        &CommandGraph{Nodes: []CommandNode{}, Edges: []CommandEdge{}},
		store:        store,
		logger:       logger,
		projectRoot:  projectRoot,
		commandCache: make(map[string]*CommandNode),
	}
}

func (bca *BuildCommandAnalyzer) AnalyzeBuildGraph(ctx context.Context) (*CommandGraph, error) {
	bca.mu.Lock()
	defer bca.mu.Unlock()

	bca.graph = &CommandGraph{Nodes: []CommandNode{}, Edges: []CommandEdge{}}

	if err := bca.discoverCommands(ctx); err != nil {
		return nil, fmt.Errorf("failed to discover commands: %w", err)
	}

	if err := bca.analyzeDependencies(ctx); err != nil {
		return nil, fmt.Errorf("failed to analyze dependencies: %w", err)
	}

	if bca.store != nil {
		bca.persistGraph(ctx)
	}

	return bca.graph, nil
}

func (bca *BuildCommandAnalyzer) discoverCommands(ctx context.Context) error {
	commonCommands := []struct {
		name        string
		command     string
		description string
		category    string
		triggers    []ContextTrigger
	}{
		{"go_build", "go build ./...", "Build all Go packages", "build", []ContextTrigger{
			{BucketType: ContextBucketTypeProject, Keywords: []string{"go", "build", "compile"}, MinRelevance: 0.3},
		}},
		{"go_test", "go test ./...", "Run all Go tests", "test", []ContextTrigger{
			{BucketType: ContextBucketTypeProject, Keywords: []string{"go", "test", "testing"}, MinRelevance: 0.3},
		}},
		{"go_test_verbose", "go test -v ./...", "Run all Go tests with verbose output", "test", []ContextTrigger{
			{BucketType: ContextBucketTypeProject, Keywords: []string{"go", "test", "verbose", "debug"}, MinRelevance: 0.4},
		}},
		{"go_test_race", "go test -race ./...", "Run Go tests with race detector", "test", []ContextTrigger{
			{BucketType: ContextBucketTypeProject, Keywords: []string{"go", "test", "race", "concurrency"}, MinRelevance: 0.4},
		}},
		{"go_lint", "golangci-lint run", "Run Go linter", "lint", []ContextTrigger{
			{BucketType: ContextBucketTypeProject, Keywords: []string{"go", "lint", "static", "analysis"}, MinRelevance: 0.3},
		}},
		{"go_fmt", "go fmt ./...", "Format Go code", "format", []ContextTrigger{
			{BucketType: ContextBucketTypeProject, Keywords: []string{"go", "format", "fmt", "style"}, MinRelevance: 0.3},
		}},
		{"go_vet", "go vet ./...", "Run Go vet", "lint", []ContextTrigger{
			{BucketType: ContextBucketTypeProject, Keywords: []string{"go", "vet", "analyze"}, MinRelevance: 0.3},
		}},
		{"go_mod_tidy", "go mod tidy", "Tidy Go modules", "dependency", []ContextTrigger{
			{BucketType: ContextBucketTypeProject, Keywords: []string{"go", "mod", "tidy", "dependency"}, MinRelevance: 0.3},
		}},
		{"go_mod_download", "go mod download", "Download Go modules", "dependency", []ContextTrigger{
			{BucketType: ContextBucketTypeProject, Keywords: []string{"go", "mod", "download", "dependency"}, MinRelevance: 0.3},
		}},
		{"go_generate", "go generate ./...", "Run Go generate", "generate", []ContextTrigger{
			{BucketType: ContextBucketTypeProject, Keywords: []string{"go", "generate", "codegen"}, MinRelevance: 0.3},
		}},
		{"make_build", "make build", "Build using Makefile", "build", []ContextTrigger{
			{BucketType: ContextBucketTypeProject, Keywords: []string{"make", "build", "compile"}, MinRelevance: 0.3},
		}},
		{"make_test", "make test", "Test using Makefile", "test", []ContextTrigger{
			{BucketType: ContextBucketTypeProject, Keywords: []string{"make", "test", "testing"}, MinRelevance: 0.3},
		}},
		{"make_lint", "make lint", "Lint using Makefile", "lint", []ContextTrigger{
			{BucketType: ContextBucketTypeProject, Keywords: []string{"make", "lint", "static"}, MinRelevance: 0.3},
		}},
		{"npm_build", "npm run build", "Build using npm", "build", []ContextTrigger{
			{BucketType: ContextBucketTypeProject, Keywords: []string{"npm", "build", "javascript", "typescript"}, MinRelevance: 0.3},
		}},
		{"npm_test", "npm test", "Test using npm", "test", []ContextTrigger{
			{BucketType: ContextBucketTypeProject, Keywords: []string{"npm", "test", "javascript", "typescript"}, MinRelevance: 0.3},
		}},
		{"npm_lint", "npm run lint", "Lint using npm", "lint", []ContextTrigger{
			{BucketType: ContextBucketTypeProject, Keywords: []string{"npm", "lint", "javascript", "typescript"}, MinRelevance: 0.3},
		}},
		{"cargo_build", "cargo build", "Build Rust project", "build", []ContextTrigger{
			{BucketType: ContextBucketTypeProject, Keywords: []string{"cargo", "rust", "build", "compile"}, MinRelevance: 0.3},
		}},
		{"cargo_test", "cargo test", "Test Rust project", "test", []ContextTrigger{
			{BucketType: ContextBucketTypeProject, Keywords: []string{"cargo", "rust", "test", "testing"}, MinRelevance: 0.3},
		}},
		{"cargo_check", "cargo check", "Check Rust project", "lint", []ContextTrigger{
			{BucketType: ContextBucketTypeProject, Keywords: []string{"cargo", "rust", "check", "analyze"}, MinRelevance: 0.3},
		}},
		{"docker_build", "docker build .", "Build Docker image", "build", []ContextTrigger{
			{BucketType: ContextBucketTypeProject, Keywords: []string{"docker", "build", "container", "image"}, MinRelevance: 0.3},
		}},
	}

	for _, cmd := range commonCommands {
		if bca.isCommandAvailable(cmd.command) {
			node := CommandNode{
				ID:              uuid.New(),
				Name:            cmd.name,
				Command:         cmd.command,
				Description:     cmd.description,
				Category:        cmd.category,
				Dependencies:    []uuid.UUID{},
				ContextTriggers: cmd.triggers,
				IsStale:         false,
				RunCount:        0,
				SuccessRate:     1.0,
				CreatedAt:       time.Now(),
				UpdatedAt:       time.Now(),
			}
			bca.graph.Nodes = append(bca.graph.Nodes, node)
			bca.commandCache[cmd.name] = &node
		}
	}

	bca.discoverProjectSpecificCommands(ctx)

	return nil
}

func (bca *BuildCommandAnalyzer) isCommandAvailable(cmd string) bool {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return false
	}
	_, err := execLookPath(parts[0])
	return err == nil
}

func execLookPath(cmd string) (string, error) {
	return "", nil
}

func (bca *BuildCommandAnalyzer) discoverProjectSpecificCommands(ctx context.Context) {
	makefilePath := filepath.Join(bca.projectRoot, "Makefile")
	if _, err := os.Stat(makefilePath); err == nil {
		bca.parseMakefile(makefilePath)
	}

	packageJsonPath := filepath.Join(bca.projectRoot, "package.json")
	if _, err := os.Stat(packageJsonPath); err == nil {
		bca.parsePackageJson(packageJsonPath)
	}

	goModPath := filepath.Join(bca.projectRoot, "go.mod")
	if _, err := os.Stat(goModPath); err == nil {
		bca.parseGoMod(goModPath)
	}

	justfilePath := filepath.Join(bca.projectRoot, "justfile")
	if _, err := os.Stat(justfilePath); err == nil {
		bca.parseJustfile(justfilePath)
	}
}

func (bca *BuildCommandAnalyzer) parseMakefile(path string) {
	content, err := os.ReadFile(path)
	if err != nil {
		return
	}

	lines := strings.Split(string(content), "\n")
	targetPattern := regexp.MustCompile(`^([a-zA-Z0-9_-]+):`)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if matches := targetPattern.FindStringSubmatch(line); matches != nil {
			target := matches[1]
			if target == "all" || target == "clean" || target == "install" || target == "help" {
				continue
			}
			name := fmt.Sprintf("make_%s", target)
			if _, exists := bca.commandCache[name]; !exists {
				triggers := []ContextTrigger{
					{BucketType: ContextBucketTypeProject, Keywords: []string{"make", target}, MinRelevance: 0.4},
				}
				node := CommandNode{
					ID:              uuid.New(),
					Name:            name,
					Command:         fmt.Sprintf("make %s", target),
					Description:     fmt.Sprintf("Run make target: %s", target),
					Category:        "build",
					Dependencies:    []uuid.UUID{},
					ContextTriggers: triggers,
					IsStale:         false,
					RunCount:        0,
					SuccessRate:     1.0,
					CreatedAt:       time.Now(),
					UpdatedAt:       time.Now(),
				}
				bca.graph.Nodes = append(bca.graph.Nodes, node)
				bca.commandCache[name] = &node
			}
		}
	}
}

func (bca *BuildCommandAnalyzer) parsePackageJson(path string) {
	content, err := os.ReadFile(path)
	if err != nil {
		return
	}

	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(content, &pkg); err != nil {
		return
	}

	for name, _ := range pkg.Scripts {
		cmdName := fmt.Sprintf("npm_%s", name)
		if _, exists := bca.commandCache[cmdName]; !exists {
			category := "build"
			if strings.Contains(name, "test") {
				category = "test"
			} else if strings.Contains(name, "lint") || strings.Contains(name, "check") {
				category = "lint"
			}
			triggers := []ContextTrigger{
				{BucketType: ContextBucketTypeProject, Keywords: []string{"npm", name}, MinRelevance: 0.4},
			}
			node := CommandNode{
				ID:              uuid.New(),
				Name:            cmdName,
				Command:         fmt.Sprintf("npm run %s", name),
				Description:     fmt.Sprintf("Run npm script: %s", name),
				Category:        category,
				Dependencies:    []uuid.UUID{},
				ContextTriggers: triggers,
				IsStale:         false,
				RunCount:        0,
				SuccessRate:     1.0,
				CreatedAt:       time.Now(),
				UpdatedAt:       time.Now(),
			}
			bca.graph.Nodes = append(bca.graph.Nodes, node)
			bca.commandCache[cmdName] = &node
		}
	}
}

func (bca *BuildCommandAnalyzer) parseGoMod(path string) {
	content, err := os.ReadFile(path)
	if err != nil {
		return
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "go ") {
			continue
		}
	}
}

func (bca *BuildCommandAnalyzer) parseJustfile(path string) {
	content, err := os.ReadFile(path)
	if err != nil {
		return
	}

	lines := strings.Split(string(content), "\n")
	targetPattern := regexp.MustCompile(`^([a-zA-Z0-9_-]+)\s*:`)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if matches := targetPattern.FindStringSubmatch(line); matches != nil {
			target := matches[1]
			name := fmt.Sprintf("just_%s", target)
			if _, exists := bca.commandCache[name]; !exists {
				triggers := []ContextTrigger{
					{BucketType: ContextBucketTypeProject, Keywords: []string{"just", target}, MinRelevance: 0.4},
				}
				node := CommandNode{
					ID:              uuid.New(),
					Name:            name,
					Command:         fmt.Sprintf("just %s", target),
					Description:     fmt.Sprintf("Run just recipe: %s", target),
					Category:        "build",
					Dependencies:    []uuid.UUID{},
					ContextTriggers: triggers,
					IsStale:         false,
					RunCount:        0,
					SuccessRate:     1.0,
					CreatedAt:       time.Now(),
					UpdatedAt:       time.Now(),
				}
				bca.graph.Nodes = append(bca.graph.Nodes, node)
				bca.commandCache[name] = &node
			}
		}
	}
}

func (bca *BuildCommandAnalyzer) analyzeDependencies(ctx context.Context) error {
	buildNodes := bca.getNodesByCategory("build")
	testNodes := bca.getNodesByCategory("test")
	lintNodes := bca.getNodesByCategory("lint")
	formatNodes := bca.getNodesByCategory("format")
	depNodes := bca.getNodesByCategory("dependency")
	generateNodes := bca.getNodesByCategory("generate")

	for _, testNode := range testNodes {
		for _, buildNode := range buildNodes {
			bca.addEdge(testNode.ID, buildNode.ID, EdgeTypeCommandDependsOn, 0.9)
		}
	}

	for _, lintNode := range lintNodes {
		for _, buildNode := range buildNodes {
			bca.addEdge(lintNode.ID, buildNode.ID, EdgeTypeCommandRunsAfter, 0.7)
		}
	}

	for _, formatNode := range formatNodes {
		for _, lintNode := range lintNodes {
			bca.addEdge(formatNode.ID, lintNode.ID, EdgeTypeCommandRunsAfter, 0.8)
		}
	}

	for _, genNode := range generateNodes {
		for _, buildNode := range buildNodes {
			bca.addEdge(genNode.ID, buildNode.ID, EdgeTypeCommandDependsOn, 0.9)
		}
	}

	for _, depNode := range depNodes {
		for _, buildNode := range buildNodes {
			bca.addEdge(depNode.ID, buildNode.ID, EdgeTypeCommandDependsOn, 0.8)
		}
	}

	bca.addCommonDependencies()

	return nil
}

func (bca *BuildCommandAnalyzer) getNodesByCategory(category string) []*CommandNode {
	var nodes []*CommandNode
	for i := range bca.graph.Nodes {
		if bca.graph.Nodes[i].Category == category {
			nodes = append(nodes, &bca.graph.Nodes[i])
		}
	}
	return nodes
}

func (bca *BuildCommandAnalyzer) addCommonDependencies() {
	goBuild := bca.findNodeByName("go_build")
	goTest := bca.findNodeByName("go_test")
	goLint := bca.findNodeByName("go_lint")
	goFmt := bca.findNodeByName("go_fmt")
	goVet := bca.findNodeByName("go_vet")
	goModTidy := bca.findNodeByName("go_mod_tidy")
	goGenerate := bca.findNodeByName("go_generate")

	if goTest != nil && goBuild != nil {
		bca.addEdge(goTest.ID, goBuild.ID, EdgeTypeCommandDependsOn, 0.9)
	}
	if goLint != nil && goBuild != nil {
		bca.addEdge(goLint.ID, goBuild.ID, EdgeTypeCommandRunsAfter, 0.7)
	}
	if goFmt != nil && goLint != nil {
		bca.addEdge(goFmt.ID, goLint.ID, EdgeTypeCommandRunsAfter, 0.8)
	}
	if goVet != nil && goBuild != nil {
		bca.addEdge(goVet.ID, goBuild.ID, EdgeTypeCommandRunsAfter, 0.7)
	}
	if goModTidy != nil && goBuild != nil {
		bca.addEdge(goModTidy.ID, goBuild.ID, EdgeTypeCommandDependsOn, 0.8)
	}
	if goGenerate != nil && goBuild != nil {
		bca.addEdge(goGenerate.ID, goBuild.ID, EdgeTypeCommandDependsOn, 0.9)
	}

	makeBuild := bca.findNodeByName("make_build")
	makeTest := bca.findNodeByName("make_test")
	if makeTest != nil && makeBuild != nil {
		bca.addEdge(makeTest.ID, makeBuild.ID, EdgeTypeCommandDependsOn, 0.9)
	}
}

func (bca *BuildCommandAnalyzer) findNodeByName(name string) *CommandNode {
	for i := range bca.graph.Nodes {
		if bca.graph.Nodes[i].Name == name {
			return &bca.graph.Nodes[i]
		}
	}
	return nil
}

func (bca *BuildCommandAnalyzer) addEdge(fromID, toID uuid.UUID, edgeType EdgeType, strength float64) {
	edge := CommandEdge{
		FromID:   fromID,
		ToID:     toID,
		Type:     string(edgeType),
		Strength: strength,
	}
	bca.graph.Edges = append(bca.graph.Edges, edge)

	for i := range bca.graph.Nodes {
		if bca.graph.Nodes[i].ID == fromID {
			found := false
			for _, dep := range bca.graph.Nodes[i].Dependencies {
				if dep == toID {
					found = true
					break
				}
			}
			if !found {
				bca.graph.Nodes[i].Dependencies = append(bca.graph.Nodes[i].Dependencies, toID)
			}
			break
		}
	}
}

func (bca *BuildCommandAnalyzer) SuggestCommands(contextBucket ContextBucketType) []*CommandNode {
	bca.mu.RLock()
	defer bca.mu.RUnlock()

	type scoredCmd struct {
		node  *CommandNode
		score float64
	}

	var scored []scoredCmd
	for i := range bca.graph.Nodes {
		node := &bca.graph.Nodes[i]
		score := bca.calculateCommandRelevance(node, contextBucket)
		if score > 0 {
			scored = append(scored, scoredCmd{node: node, score: score})
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	result := make([]*CommandNode, len(scored))
	for i, s := range scored {
		result[i] = s.node
	}
	return result
}

func (bca *BuildCommandAnalyzer) calculateCommandRelevance(node *CommandNode, contextBucket ContextBucketType) float64 {
	maxScore := 0.0
	for _, trigger := range node.ContextTriggers {
		if trigger.BucketType == contextBucket || trigger.BucketType == ContextBucketTypeGlobal {
			score := bca.keywordMatchScore(trigger.Keywords, contextBucket)
			if score >= trigger.MinRelevance && score > maxScore {
				maxScore = score
			}
		}
	}

	if node.RunCount > 0 {
		maxScore *= (0.5 + 0.5*node.SuccessRate)
	}

	return maxScore
}

func (bca *BuildCommandAnalyzer) keywordMatchScore(keywords []string, contextBucket ContextBucketType) float64 {
	if len(keywords) == 0 {
		return 0.5
	}

	bucketStr := string(contextBucket)
	matches := 0
	for _, kw := range keywords {
		if strings.Contains(strings.ToLower(bucketStr), strings.ToLower(kw)) {
			matches++
		}
	}
	return float64(matches) / float64(len(keywords))
}

func (bca *BuildCommandAnalyzer) GetCommandGraph() *CommandGraph {
	bca.mu.RLock()
	defer bca.mu.RUnlock()

	graphCopy := &CommandGraph{
		Nodes: make([]CommandNode, len(bca.graph.Nodes)),
		Edges: make([]CommandEdge, len(bca.graph.Edges)),
	}
	copy(graphCopy.Nodes, bca.graph.Nodes)
	copy(graphCopy.Edges, bca.graph.Edges)
	return graphCopy
}

func (bca *BuildCommandAnalyzer) DetectStaleCommands(thresholdDays int) []*CommandNode {
	bca.mu.Lock()
	defer bca.mu.Unlock()

	cutoff := time.Now().AddDate(0, 0, -thresholdDays)
	var stale []*CommandNode

	for i := range bca.graph.Nodes {
		node := &bca.graph.Nodes[i]
		if node.LastRun != nil && node.LastRun.Before(cutoff) {
			node.IsStale = true
			node.UpdatedAt = time.Now()
			stale = append(stale, node)
		}
		if node.RunCount == 0 && node.CreatedAt.Before(cutoff) {
			node.IsStale = true
			node.UpdatedAt = time.Now()
			stale = append(stale, node)
		}
	}

	if bca.store != nil {
		for _, node := range stale {
			bca.persistCommandNode(node)
		}
	}

	return stale
}

func (bca *BuildCommandAnalyzer) RecordCommandRun(commandName string, success bool, durationMs int64) {
	bca.mu.Lock()
	defer bca.mu.Unlock()

	for i := range bca.graph.Nodes {
		if bca.graph.Nodes[i].Name == commandName {
			node := &bca.graph.Nodes[i]
			now := time.Now()
			node.LastRun = &now
			node.RunCount++
			if success {
				node.SuccessRate = (node.SuccessRate*float64(node.RunCount-1) + 1.0) / float64(node.RunCount)
			} else {
				node.SuccessRate = (node.SuccessRate * float64(node.RunCount-1)) / float64(node.RunCount)
			}
			node.UpdatedAt = now
			node.IsStale = false

			if bca.store != nil {
				bca.persistCommandNode(node)
			}
			break
		}
	}
}

func (bca *BuildCommandAnalyzer) persistGraph(ctx context.Context) {
	for _, node := range bca.graph.Nodes {
		bca.persistCommandNode(&node)
	}

	for _, edge := range bca.graph.Edges {
		bca.persistCommandEdge(&edge)
	}
}

func (bca *BuildCommandAnalyzer) persistCommandNode(node *CommandNode) {
	n := &Node{
		ID:        node.ID,
		Type:      NodeTypeCommandNode,
		CreatedAt: node.CreatedAt,
		UpdatedAt: node.UpdatedAt,
	}
	data, _ := json.Marshal(node)
	n.Data = data
	bca.store.CreateNode(context.Background(), n)
}

func (bca *BuildCommandAnalyzer) persistCommandEdge(edge *CommandEdge) {
	data, _ := json.Marshal(edge)
	e := &Edge{
		ID:        uuid.New(),
		FromID:    edge.FromID,
		ToID:      edge.ToID,
		Type:      EdgeType(edge.Type),
		Data:      data,
		CreatedAt: time.Now(),
	}
	bca.store.CreateEdge(context.Background(), e)
}

func (bca *BuildCommandAnalyzer) GetTopologicalOrder() ([]*CommandNode, error) {
	bca.mu.RLock()
	defer bca.mu.RUnlock()

	nodeMap := make(map[uuid.UUID]*CommandNode)
	for i := range bca.graph.Nodes {
		nodeMap[bca.graph.Nodes[i].ID] = &bca.graph.Nodes[i]
	}

	inDegree := make(map[uuid.UUID]int)
	adj := make(map[uuid.UUID][]uuid.UUID)

	for _, node := range bca.graph.Nodes {
		inDegree[node.ID] = 0
		adj[node.ID] = []uuid.UUID{}
	}

	for _, edge := range bca.graph.Edges {
		adj[edge.FromID] = append(adj[edge.FromID], edge.ToID)
		inDegree[edge.ToID]++
	}

	queue := make([]uuid.UUID, 0)
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}

	var result []*CommandNode
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if node, ok := nodeMap[current]; ok {
			result = append(result, node)
		}

		for _, neighbor := range adj[current] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	if len(result) != len(bca.graph.Nodes) {
		return nil, fmt.Errorf("cycle detected in command graph")
	}

	return result, nil
}