package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/iSundram/Automergent/internal/graph"
	"github.com/iSundram/Automergent/internal/graph/analysis"
	"github.com/iSundram/Automergent/internal/graph/continuity"
	"github.com/iSundram/Automergent/internal/graph/healing"
	"github.com/iSundram/Automergent/internal/graph/memory"
	"github.com/iSundram/Automergent/internal/graph/tools"
	"github.com/iSundram/Automergent/internal/graph/wiring"
	"github.com/iSundram/Automergent/internal/graph/workflow"
)

// GraphEngine is the unified interface for all graph-based intelligence.
type GraphEngine struct {
	mu sync.RWMutex

	// Core
	Manager *graph.GraphManager

	// Analysis
	FeatureAnalyzer     *analysis.FeatureAnalyzer
	EntryPointDetector  *analysis.EntryPointDetector
	WiringAnalyzer      *analysis.WiringAnalyzer
	ImpactAnalyzer      *analysis.ImpactAnalyzer

	// Workflow
	BucketManager   *workflow.ContextBucketManager
	TodoEngine      *workflow.TodoWorkflowEngine
	RememberTool    *workflow.RememberTool

	// Decision & Memory
	DecisionRecorder *memory.DecisionRecorder
	MemoryManager    *memory.MemoryManager
	ReplayEngine     *memory.ReplayEngine

	// Tools
	ToolRegistry      *tools.ToolRegistry
	BuildAnalyzer     *tools.BuildCommandAnalyzer
	DynamicToolGen    *tools.DynamicToolGenerator

	// Wiring
	WiringEngine       *wiring.WiringEngine
	EntryPointWiring   *wiring.EntryPointWiring
	EventSystem        *wiring.EventSystem
	AppearanceValidator *wiring.AppearanceValidator

	// Healing
	FixValidator     *healing.FixValidator
	ContextCleanup   *healing.ContextCleanup
	GraphMaintenance *healing.GraphMaintenance
	CleanupScheduler *healing.CleanupScheduler

	// Continuity
	ContinuityManager *continuity.ContinuityManager
	UndoManager       *continuity.UndoManager
	ContextResumer    *continuity.ContextResumer
	TaskRouter        *continuity.TaskRouter

	logger *zap.Logger
	
	// Adapters
	workflowStore *workflowStoreAdapter
	toolsStore    *toolsStoreAdapter
	healingStore  *healingStoreAdapter
}

// workflowStoreAdapter adapts graph.Store to workflow.StoreInterface
type workflowStoreAdapter struct {
	store *graph.Store
}

func (a *workflowStoreAdapter) CreateNode(ctx context.Context, node *workflow.Node) error {
	gNode := &graph.Node{
		ID:        node.ID,
		Type:      graph.NodeType(node.Type),
		Data:      node.Data,
		CreatedAt: node.CreatedAt,
		UpdatedAt: node.UpdatedAt,
	}
	return a.store.CreateNode(ctx, gNode)
}

func (a *workflowStoreAdapter) GetNode(ctx context.Context, id uuid.UUID) (*workflow.Node, error) {
	gNode, err := a.store.GetNode(ctx, id)
	if err != nil {
		return nil, err
	}
	return &workflow.Node{
		ID:        gNode.ID,
		Type:      workflow.NodeType(gNode.Type),
		Data:      gNode.Data,
		CreatedAt: gNode.CreatedAt,
		UpdatedAt: gNode.UpdatedAt,
	}, nil
}

func (a *workflowStoreAdapter) UpdateNode(ctx context.Context, node *workflow.Node) error {
	gNode := &graph.Node{
		ID:        node.ID,
		Type:      graph.NodeType(node.Type),
		Data:      node.Data,
		CreatedAt: node.CreatedAt,
		UpdatedAt: node.UpdatedAt,
	}
	return a.store.UpdateNode(ctx, gNode)
}

func (a *workflowStoreAdapter) DeleteNode(ctx context.Context, id uuid.UUID) error {
	return a.store.DeleteNode(ctx, id)
}

func (a *workflowStoreAdapter) ListNodes(ctx context.Context, nodeType workflow.NodeType, limit, offset int) ([]*workflow.Node, error) {
	gNodes, err := a.store.ListNodes(ctx, graph.NodeType(nodeType), limit, offset)
	if err != nil {
		return nil, err
	}
	var nodes []*workflow.Node
	for _, gNode := range gNodes {
		nodes = append(nodes, &workflow.Node{
			ID:        gNode.ID,
			Type:      workflow.NodeType(gNode.Type),
			Data:      gNode.Data,
			CreatedAt: gNode.CreatedAt,
			UpdatedAt: gNode.UpdatedAt,
		})
	}
	return nodes, nil
}

func (a *workflowStoreAdapter) CreateEdge(ctx context.Context, edge *workflow.Edge) error {
	gEdge := &graph.Edge{
		ID:        edge.ID,
		FromID:    edge.FromID,
		ToID:      edge.ToID,
		Type:      graph.EdgeType(edge.Type),
		Data:      edge.Data,
		CreatedAt: edge.CreatedAt,
	}
	return a.store.CreateEdge(ctx, gEdge)
}

func (a *workflowStoreAdapter) GetEdgesFrom(ctx context.Context, fromID uuid.UUID, edgeType workflow.EdgeType) ([]*workflow.Edge, error) {
	gEdges, err := a.store.GetEdgesFrom(ctx, fromID, graph.EdgeType(edgeType))
	if err != nil {
		return nil, err
	}
	var edges []*workflow.Edge
	for _, gEdge := range gEdges {
		edges = append(edges, &workflow.Edge{
			ID:        gEdge.ID,
			FromID:    gEdge.FromID,
			ToID:      gEdge.ToID,
			Type:      workflow.EdgeType(gEdge.Type),
			Data:      gEdge.Data,
			CreatedAt: gEdge.CreatedAt,
		})
	}
	return edges, nil
}

func (a *workflowStoreAdapter) GetEdgesTo(ctx context.Context, toID uuid.UUID, edgeType workflow.EdgeType) ([]*workflow.Edge, error) {
	gEdges, err := a.store.GetEdgesTo(ctx, toID, graph.EdgeType(edgeType))
	if err != nil {
		return nil, err
	}
	var edges []*workflow.Edge
	for _, gEdge := range gEdges {
		edges = append(edges, &workflow.Edge{
			ID:        gEdge.ID,
			FromID:    gEdge.FromID,
			ToID:      gEdge.ToID,
			Type:      workflow.EdgeType(gEdge.Type),
			Data:      gEdge.Data,
			CreatedAt: gEdge.CreatedAt,
		})
	}
	return edges, nil
}

func (a *workflowStoreAdapter) GetEdgesBetween(ctx context.Context, fromID, toID uuid.UUID) ([]*workflow.Edge, error) {
	gEdges, err := a.store.GetEdgesBetween(ctx, fromID, toID)
	if err != nil {
		return nil, err
	}
	var edges []*workflow.Edge
	for _, gEdge := range gEdges {
		edges = append(edges, &workflow.Edge{
			ID:        gEdge.ID,
			FromID:    gEdge.FromID,
			ToID:      gEdge.ToID,
			Type:      workflow.EdgeType(gEdge.Type),
			Data:      gEdge.Data,
			CreatedAt: gEdge.CreatedAt,
		})
	}
	return edges, nil
}

func (a *workflowStoreAdapter) BeginTx(ctx context.Context) (*workflow.Tx, error) {
	_, err := a.store.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	// Use reflection or type assertion to create workflow.Tx
	// Since workflow.Tx fields are unexported, we need a different approach
	// Return a minimal Tx that embeds the graph.Tx
	return &workflow.Tx{}, nil
}

// toolsStoreAdapter adapts graph.Store to tools.GraphStore
type toolsStoreAdapter struct {
	store *graph.Store
}

func (a *toolsStoreAdapter) CreateNode(ctx context.Context, node *tools.Node) error {
	gNode := &graph.Node{
		ID:        node.ID,
		Type:      graph.NodeType(node.Type),
		Data:      node.Data,
		CreatedAt: node.CreatedAt,
		UpdatedAt: node.UpdatedAt,
	}
	return a.store.CreateNode(ctx, gNode)
}

func (a *toolsStoreAdapter) GetNode(ctx context.Context, id uuid.UUID) (*tools.Node, error) {
	gNode, err := a.store.GetNode(ctx, id)
	if err != nil {
		return nil, err
	}
	return &tools.Node{
		ID:        gNode.ID,
		Type:      tools.NodeType(gNode.Type),
		Data:      gNode.Data,
		CreatedAt: gNode.CreatedAt,
		UpdatedAt: gNode.UpdatedAt,
	}, nil
}

func (a *toolsStoreAdapter) UpdateNode(ctx context.Context, node *tools.Node) error {
	gNode := &graph.Node{
		ID:        node.ID,
		Type:      graph.NodeType(node.Type),
		Data:      node.Data,
		CreatedAt: node.CreatedAt,
		UpdatedAt: node.UpdatedAt,
	}
	return a.store.UpdateNode(ctx, gNode)
}

func (a *toolsStoreAdapter) ListNodes(ctx context.Context, nodeType tools.NodeType, limit, offset int) ([]*tools.Node, error) {
	gNodes, err := a.store.ListNodes(ctx, graph.NodeType(nodeType), limit, offset)
	if err != nil {
		return nil, err
	}
	var nodes []*tools.Node
	for _, gNode := range gNodes {
		nodes = append(nodes, &tools.Node{
			ID:        gNode.ID,
			Type:      tools.NodeType(gNode.Type),
			Data:      gNode.Data,
			CreatedAt: gNode.CreatedAt,
			UpdatedAt: gNode.UpdatedAt,
		})
	}
	return nodes, nil
}

func (a *toolsStoreAdapter) CreateEdge(ctx context.Context, edge *tools.Edge) error {
	gEdge := &graph.Edge{
		ID:        edge.ID,
		FromID:    edge.FromID,
		ToID:      edge.ToID,
		Type:      graph.EdgeType(edge.Type),
		Data:      edge.Data,
		CreatedAt: edge.CreatedAt,
	}
	return a.store.CreateEdge(ctx, gEdge)
}

func (a *toolsStoreAdapter) GetEdgesFrom(ctx context.Context, fromID uuid.UUID, edgeType tools.EdgeType) ([]*tools.Edge, error) {
	gEdges, err := a.store.GetEdgesFrom(ctx, fromID, graph.EdgeType(edgeType))
	if err != nil {
		return nil, err
	}
	var edges []*tools.Edge
	for _, gEdge := range gEdges {
		edges = append(edges, &tools.Edge{
			ID:        gEdge.ID,
			FromID:    gEdge.FromID,
			ToID:      gEdge.ToID,
			Type:      tools.EdgeType(gEdge.Type),
			Data:      gEdge.Data,
			CreatedAt: gEdge.CreatedAt,
		})
	}
	return edges, nil
}

func (a *toolsStoreAdapter) GetEdgesTo(ctx context.Context, toID uuid.UUID, edgeType tools.EdgeType) ([]*tools.Edge, error) {
	gEdges, err := a.store.GetEdgesTo(ctx, toID, graph.EdgeType(edgeType))
	if err != nil {
		return nil, err
	}
	var edges []*tools.Edge
	for _, gEdge := range gEdges {
		edges = append(edges, &tools.Edge{
			ID:        gEdge.ID,
			FromID:    gEdge.FromID,
			ToID:      gEdge.ToID,
			Type:      tools.EdgeType(gEdge.Type),
			Data:      gEdge.Data,
			CreatedAt: gEdge.CreatedAt,
		})
	}
	return edges, nil
}

// healingStoreAdapter adapts graph.Store to healing.GraphStore
type healingStoreAdapter struct {
	store *graph.Store
}

func (a *healingStoreAdapter) GetNode(ctx context.Context, id uuid.UUID) (interface{}, error) {
	return a.store.GetNode(ctx, id)
}

func (a *healingStoreAdapter) UpdateNode(ctx context.Context, node interface{}) error {
	if gNode, ok := node.(*graph.Node); ok {
		return a.store.UpdateNode(ctx, gNode)
	}
	// Handle map[string]interface{} from healing components
	if nodeMap, ok := node.(map[string]interface{}); ok {
		idStr, _ := nodeMap["id"].(string)
		id, _ := uuid.Parse(idStr)
		typeStr, _ := nodeMap["type"].(string)
		data, _ := json.Marshal(nodeMap["data"])
		createdAt, _ := time.Parse(time.RFC3339, nodeMap["created_at"].(string))
		updatedAt, _ := time.Parse(time.RFC3339, nodeMap["updated_at"].(string))
		
		gNode := &graph.Node{
			ID:        id,
			Type:      graph.NodeType(typeStr),
			Data:      data,
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		}
		return a.store.UpdateNode(ctx, gNode)
	}
	return nil
}

func (a *healingStoreAdapter) DeleteNode(ctx context.Context, id uuid.UUID) error {
	return a.store.DeleteNode(ctx, id)
}

func (a *healingStoreAdapter) ListNodes(ctx context.Context, nodeType string, limit, offset int) ([]interface{}, error) {
	gNodes, err := a.store.ListNodes(ctx, graph.NodeType(nodeType), limit, offset)
	if err != nil {
		return nil, err
	}
	var nodes []interface{}
	for _, gNode := range gNodes {
		nodes = append(nodes, gNode)
	}
	return nodes, nil
}

func (a *healingStoreAdapter) CreateNode(ctx context.Context, node interface{}) error {
	if gNode, ok := node.(*graph.Node); ok {
		return a.store.CreateNode(ctx, gNode)
	}
	// Handle map[string]interface{} from healing components
	if nodeMap, ok := node.(map[string]interface{}); ok {
		idStr, _ := nodeMap["id"].(string)
		id, _ := uuid.Parse(idStr)
		typeStr, _ := nodeMap["type"].(string)
		data, _ := json.Marshal(nodeMap["data"])
		createdAt, _ := time.Parse(time.RFC3339, nodeMap["created_at"].(string))
		updatedAt, _ := time.Parse(time.RFC3339, nodeMap["updated_at"].(string))
		
		gNode := &graph.Node{
			ID:        id,
			Type:      graph.NodeType(typeStr),
			Data:      data,
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		}
		return a.store.CreateNode(ctx, gNode)
	}
	return nil
}

func (a *healingStoreAdapter) ExecuteQuery(ctx context.Context, query string, args ...interface{}) (interface{}, error) {
	// graph.Store doesn't have ExecuteQuery, return nil
	return nil, nil
}

// GraphConfig holds configuration for the entire graph engine.
type GraphConfig struct {
	DatabasePath string

	// Analysis
	SimilarityConfig analysis.SimilarityConfig

	// Workflow
	DefaultSharePolicy        workflow.SharePolicy
	MemoryPromotionThreshold  float64

	// Decision & Memory
	DecisionConfidenceThreshold float64
	MemoryDedupeThreshold       float64

	// Tools
	ToolEffectivenessWindow int
	ProjectRoot             string

	// Healing
	FixValidatorConfig *healing.FixValidatorConfig
	CleanupConfig      *healing.CleanupConfig

	// Continuity
	ContinuityConfig *continuity.ContinuityConfig

	// Logger
	Logger *zap.Logger
}

// DefaultGraphConfig returns sensible defaults.
func DefaultGraphConfig() *GraphConfig {
	simConfig := analysis.DefaultSimilarityConfig()
	fixConfig := healing.DefaultFixValidatorConfig()
	cleanupConfig := healing.DefaultCleanupConfig()
	continuityConfig := continuity.DefaultContinuityConfig()

	logger, _ := zap.NewDevelopment()

	return &GraphConfig{
		DatabasePath:                ".automergent/graph.db",
		SimilarityConfig:            simConfig,
		DefaultSharePolicy:          workflow.SharePolicySummary,
		MemoryPromotionThreshold:    0.7,
		DecisionConfidenceThreshold: 0.7,
		MemoryDedupeThreshold:       0.85,
		ToolEffectivenessWindow:     100,
		ProjectRoot:                 ".",
		FixValidatorConfig:          &fixConfig,
		CleanupConfig:               &cleanupConfig,
		ContinuityConfig:            &continuityConfig,
		Logger:                      logger,
	}
}

// NewGraphEngine creates a new unified graph engine with all components initialized.
func NewGraphEngine(ctx context.Context, config *GraphConfig) (*GraphEngine, error) {
	if config == nil {
		config = DefaultGraphConfig()
	}

	if config.Logger == nil {
		logger, _ := zap.NewDevelopment()
		config.Logger = logger
	}

	// Initialize core graph manager
	mgr, err := graph.NewManager(graph.ManagerConfig{
		Path: config.DatabasePath,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create graph manager: %w", err)
	}

	store := mgr.Store()
	query := mgr.Query()

	engine := &GraphEngine{
		Manager: mgr,
		logger:  config.Logger,
	}

	// Create adapters
	engine.workflowStore = &workflowStoreAdapter{store: store}
	engine.toolsStore = &toolsStoreAdapter{store: store}
	engine.healingStore = &healingStoreAdapter{store: store}

	// Initialize Analysis components
	engine.FeatureAnalyzer = analysis.NewFeatureAnalyzer(store, query, config.SimilarityConfig)
	engine.EntryPointDetector = analysis.NewEntryPointDetector(store, query)
	engine.WiringAnalyzer = analysis.NewWiringAnalyzer(store, query)
	engine.ImpactAnalyzer = analysis.NewImpactAnalyzer(store, query)

	// Initialize Workflow components
	engine.BucketManager = workflow.NewContextBucketManager(engine.workflowStore)
	engine.TodoEngine = workflow.NewTodoWorkflowEngine(engine.workflowStore, engine.BucketManager)
	engine.RememberTool = workflow.NewRememberTool(engine.workflowStore, engine.BucketManager, config.MemoryPromotionThreshold)

	// Initialize Decision & Memory components
	embeddingProvider := &dummyEmbeddingProvider{}
	engine.DecisionRecorder = memory.NewDecisionRecorder(store, embeddingProvider)
	engine.MemoryManager = memory.NewMemoryManager(store, embeddingProvider)
	engine.MemoryManager.SetDeduplicationThreshold(config.MemoryDedupeThreshold)
	engine.MemoryManager.SetPromotionThreshold(config.MemoryPromotionThreshold)
	engine.ReplayEngine = memory.NewReplayEngine(store, engine.DecisionRecorder, engine.MemoryManager)

	// Initialize Tools components
	engine.ToolRegistry = tools.NewToolRegistry(engine.toolsStore, config.Logger)
	engine.BuildAnalyzer = tools.NewBuildCommandAnalyzer(config.ProjectRoot, engine.toolsStore, config.Logger)
	engine.DynamicToolGen = tools.NewDynamicToolGenerator(engine.toolsStore, config.Logger, engine.ToolRegistry)

	// Initialize Wiring components
	engine.WiringEngine = wiring.NewWiringEngine(store)
	engine.EntryPointWiring = engine.WiringEngine.GetEntryPointWiring()
	engine.EventSystem = engine.WiringEngine.GetEventSystem()
	engine.AppearanceValidator = engine.WiringEngine.GetAppearanceValidator()

	// Initialize Healing components
	testRunner := &dummyTestRunner{}
	acceptanceChecker := &dummyAcceptanceChecker{}
	fixApplier := &dummyFixApplier{}
	engine.FixValidator = healing.NewFixValidator(
		engine.healingStore,
		*config.FixValidatorConfig,
		testRunner,
		acceptanceChecker,
		fixApplier,
	)
	engine.ContextCleanup = healing.NewContextCleanup(engine.healingStore, config.CleanupConfig.StalenessConfig)
	engine.GraphMaintenance = healing.NewGraphMaintenance(engine.healingStore, config.CleanupConfig.StalenessConfig)
	engine.CleanupScheduler = healing.NewCleanupScheduler(
		engine.healingStore,
		*config.CleanupConfig,
		engine.FixValidator,
		engine.ContextCleanup,
		engine.GraphMaintenance,
	)

	// Initialize Continuity components
	engine.ContinuityManager = continuity.NewContinuityManager(store, query, *config.ContinuityConfig)
	engine.UndoManager = continuity.NewUndoManager(store)
	engine.ContextResumer = continuity.NewContextResumer(store, query)
	engine.TaskRouter = continuity.NewTaskRouter(store, query, engine.ContinuityManager, engine.ContextResumer)

	return engine, nil
}

// dummyEmbeddingProvider is a placeholder for embedding generation.
type dummyEmbeddingProvider struct{}

func (d *dummyEmbeddingProvider) GetEmbedding(ctx context.Context, text string) ([]float32, error) {
	return make([]float32, 384), nil
}

// dummyTestRunner is a placeholder for test execution.
type dummyTestRunner struct{}

func (d *dummyTestRunner) RunTests(ctx context.Context, fixData json.RawMessage) (passed, failed int, err error) {
	return 1, 0, nil
}

// dummyAcceptanceChecker is a placeholder for acceptance checking.
type dummyAcceptanceChecker struct{}

func (d *dummyAcceptanceChecker) CheckAcceptance(ctx context.Context, fixData json.RawMessage) (bool, error) {
	return true, nil
}

// dummyFixApplier is a placeholder for fix application.
type dummyFixApplier struct{}

func (d *dummyFixApplier) ApplyFix(ctx context.Context, fixData json.RawMessage) error {
	return nil
}

func (d *dummyFixApplier) RevertFix(ctx context.Context, fixData json.RawMessage) error {
	return nil
}

// Close closes the graph engine and all components.
func (e *GraphEngine) Close() error {
	if e.CleanupScheduler != nil && e.CleanupScheduler.IsRunning() {
		e.CleanupScheduler.Stop()
	}
	return e.Manager.Close()
}

// ProcessResult holds the result of processing a user request.
type ProcessResult struct {
	TaskID            string
	WorkflowID        string
	RouteType         continuity.TaskRelation
	FeatureMatches    []analysis.FeatureMatch
	EntryPoints       []analysis.EntryPoint
	WiringPattern     *analysis.WiringPattern
	ImpactScope       *analysis.ImpactScope
	IntegrationResult *wiring.IntegrationResult
	Appearance        *wiring.FeatureAppearance
	UsageExamples     []wiring.UsageExample
}

// ProcessUserRequest orchestrates the full pipeline for processing a user request.
func (e *GraphEngine) ProcessUserRequest(ctx context.Context, userRequest string) (*ProcessResult, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := &ProcessResult{}

	// Step 1: Route the task to determine if it's new, follow-up, or related
	messages := []continuity.Message{
		{ID: uuid.New(), Role: "user", Content: userRequest, Timestamp: time.Now()},
	}
	boundaries := e.TaskRouter.DetectTaskBoundary(messages)

	var taskID *uuid.UUID
	var relation continuity.TaskRelation

	if len(boundaries) > 0 {
		lastBoundary := boundaries[len(boundaries)-1]
		if lastBoundary.NewTaskID != nil {
			taskID = lastBoundary.NewTaskID
			relation = continuity.TaskRelationNewTask
		} else if lastBoundary.PreviousTaskID != nil {
			taskID = lastBoundary.PreviousTaskID
			relation = continuity.TaskRelationFollowUp
		}
	}

	if taskID == nil {
		newTaskID := uuid.New()
		taskID = &newTaskID
		relation = continuity.TaskRelationNewTask
	}

	result.TaskID = taskID.String()
	result.RouteType = relation

	// Step 2: Create or resume workflow
	workflow, err := e.TodoEngine.CreateWorkflow(ctx, *taskID, "user_request", userRequest, userRequest)
	if err != nil {
		return nil, fmt.Errorf("create workflow: %w", err)
	}
	result.WorkflowID = workflow.ID.String()

	// Step 3: Find similar features based on text request
	featureMatches, err := e.FindSimilarFeaturesByText(ctx, userRequest, 10)
	if err != nil {
		e.logger.Warn("failed to find similar features", zap.Error(err))
	} else {
		result.FeatureMatches = featureMatches
	}

	// Step 4: Detect user entry points from the request
	entryPoints, err := e.DetectEntryPointsFromText(ctx, userRequest)
	if err != nil {
		e.logger.Warn("failed to detect entry points", zap.Error(err))
	} else {
		result.EntryPoints = entryPoints
	}

	// Step 5: Get wiring pattern from similar features
	var wiringPattern *analysis.WiringPattern
	if len(result.FeatureMatches) > 0 {
		wiringPattern, err = e.WiringAnalyzer.SuggestWiring(ctx, &graph.Node{}, result.FeatureMatches)
		if err != nil {
			e.logger.Warn("failed to suggest wiring", zap.Error(err))
		}
	} else {
		// Create a dummy feature node for pattern analysis
		dummyFeature := &graph.Node{ID: *taskID, Type: graph.NodeTypeTask}
		wiringPattern, err = e.WiringAnalyzer.GetWiringPattern(ctx, dummyFeature.ID)
		if err != nil {
			e.logger.Warn("failed to get wiring pattern", zap.Error(err))
		}
	}
	result.WiringPattern = wiringPattern

	// Step 6: Analyze impact scope
	impactScope, err := e.ImpactAnalyzer.GetImpactScope(ctx, "user_request")
	if err != nil {
		e.logger.Warn("failed to analyze impact", zap.Error(err))
	} else {
		result.ImpactScope = impactScope
	}

	// Step 7: Create feature appearance from request
	featureAppearance := e.buildFeatureAppearance(*taskID, userRequest, result.FeatureMatches, result.EntryPoints, wiringPattern)

	// Step 8: Wire the feature
	integrationResult, err := e.WiringEngine.WireFeature(ctx, featureAppearance, e.toSimilarFeaturesFromMatches(result.FeatureMatches))
	if err != nil {
		e.logger.Warn("failed to wire feature", zap.Error(err))
	} else {
		result.IntegrationResult = integrationResult
	}

	// Step 9: Validate appearance
	validationResult := e.AppearanceValidator.ValidateAppearance(ctx, featureAppearance)
	if !validationResult.Valid {
		e.logger.Warn("appearance validation failed", zap.Strings("errors", extractErrorMessages(validationResult.Errors)))
	}

	// Step 10: Generate usage examples
	usageExamples, err := e.AppearanceValidator.GenerateUsageExamples(ctx, featureAppearance)
	if err != nil {
		e.logger.Warn("failed to generate usage examples", zap.Error(err))
	} else {
		result.UsageExamples = usageExamples
	}

	result.Appearance = featureAppearance

	return result, nil
}

// FindSimilarFeaturesByText finds similar features based on a text request (not FeatureMatchRequest).
func (e *GraphEngine) FindSimilarFeaturesByText(ctx context.Context, request string, limit int) ([]analysis.FeatureMatch, error) {
	// Create a synthetic feature signature from the request text
	keywords := e.extractKeywords(request)
	symbols := e.extractSymbols(request)

	// Use the FeatureAnalyzer's FindSimilarFeatures with a dummy feature ID
	// Since we can't call unexported methods, we'll use a simpler approach
	// Create a temporary feature node to compare against
	dummyFeature := &graph.Node{
		ID:   uuid.New(),
		Type: graph.NodeTypeTask,
	}
	featureData := map[string]interface{}{
		"name":        "text_request",
		"description": request,
		"keywords":    keywords,
		"symbols":     symbols,
	}
	dummyFeature.Data, _ = json.Marshal(featureData)

	// Use the existing FindSimilarFeatures by creating a temporary feature
	// This is a workaround since we can't access unexported methods
	allFeatures, err := e.getAllFeaturesFromStore(ctx)
	if err != nil {
		return nil, err
	}

	var matches []analysis.FeatureMatch
	for _, feature := range allFeatures {
		sig, err := e.FeatureAnalyzer.GetFeatureSignature(ctx, feature)
		if err != nil {
			continue
		}

		textSim := e.cosineSimilarityText(keywords, sig.Keywords) +
			e.cosineSimilarityText(symbols, sig.Symbols)*0.5
		textSim /= 1.5

		graphSim := 0.0 // No graph comparison for text-only request

		// Use default similarity config values
		combinedSim := 0.6*textSim + 0.4*graphSim

		if combinedSim >= 0.3 {
			match := analysis.FeatureMatch{
				FeatureID:       feature.ID,
				FeatureName:     e.getFeatureName(feature),
				Similarity:      combinedSim,
				TextSimilarity:  textSim,
				GraphSimilarity: graphSim,
				MatchReason:     e.generateMatchReason(textSim, graphSim, keywords, symbols, sig),
				MatchedSymbols:  e.findMatchedSymbols(symbols, sig.Symbols),
				SharedDeps:      e.findSharedDeps([]string{}, sig.Dependencies),
			}
			matches = append(matches, match)
		}
	}

	// Sort by similarity descending
	for i := 0; i < len(matches)-1; i++ {
		for j := i + 1; j < len(matches); j++ {
			if matches[i].Similarity < matches[j].Similarity {
				matches[i], matches[j] = matches[j], matches[i]
			}
		}
	}

	if limit > 0 && len(matches) > limit {
		matches = matches[:limit]
	}

	return matches, nil
}

func (e *GraphEngine) getAllFeaturesFromStore(ctx context.Context) ([]*graph.Node, error) {
	return e.Manager.Store().ListNodes(ctx, graph.NodeTypeTask, 0, 0)
}

func (e *GraphEngine) getFeatureName(node *graph.Node) string {
	var data map[string]interface{}
	if err := node.UnmarshalData(&data); err != nil {
		return node.ID.String()
	}
	if name, ok := data["name"].(string); ok {
		return name
	}
	if title, ok := data["title"].(string); ok {
		return title
	}
	return node.ID.String()
}

// DetectEntryPointsFromText detects user entry points from a text prompt (not feature UUID).
func (e *GraphEngine) DetectEntryPointsFromText(ctx context.Context, prompt string) ([]analysis.EntryPoint, error) {
	var entryPoints []analysis.EntryPoint

	// Detect TUI commands
	if strings.Contains(strings.ToLower(prompt), "command") ||
		strings.Contains(strings.ToLower(prompt), "tui") ||
		strings.Contains(strings.ToLower(prompt), "keybind") {
		entryPoints = append(entryPoints, analysis.EntryPoint{
			EntryID:     uuid.New(),
			EntryType:   analysis.EntryPointTypeTUICommand,
			Name:        e.extractCommandName(prompt),
			Description: "TUI command from user request",
			Location:    "tui_keymap",
			FeatureID:   uuid.Nil,
			Visibility:  analysis.VisibilityLevelUserVisible,
			IsPrimary:   true,
		})
	}

	// Detect API endpoints
	if strings.Contains(strings.ToLower(prompt), "api") ||
		strings.Contains(strings.ToLower(prompt), "endpoint") ||
		strings.Contains(strings.ToLower(prompt), "http") ||
		strings.Contains(strings.ToLower(prompt), "rest") {
		entryPoints = append(entryPoints, analysis.EntryPoint{
			EntryID:     uuid.New(),
			EntryType:   analysis.EntryPointTypeAPIEndpoint,
			Name:        e.extractEndpointName(prompt),
			Description: "API endpoint from user request",
			Location:    "/api/v1/" + e.extractEndpointName(prompt),
			FeatureID:   uuid.Nil,
			Visibility:  analysis.VisibilityLevelUserVisible,
			IsPrimary:   true,
		})
	}

	// Detect CLI flags
	if strings.Contains(strings.ToLower(prompt), "cli") ||
		strings.Contains(strings.ToLower(prompt), "flag") ||
		strings.Contains(strings.ToLower(prompt), "argument") {
		entryPoints = append(entryPoints, analysis.EntryPoint{
			EntryID:     uuid.New(),
			EntryType:   analysis.EntryPointTypeCLIFlag,
			Name:        e.extractFlagName(prompt),
			Description: "CLI flag from user request",
			Location:    "cli_flags",
			FeatureID:   uuid.Nil,
			Visibility:  analysis.VisibilityLevelUserVisible,
			IsPrimary:   false,
		})
	}

	// Detect config keys
	if strings.Contains(strings.ToLower(prompt), "config") ||
		strings.Contains(strings.ToLower(prompt), "setting") {
		entryPoints = append(entryPoints, analysis.EntryPoint{
			EntryID:     uuid.New(),
			EntryType:   analysis.EntryPointTypeConfigKey,
			Name:        e.extractConfigKey(prompt),
			Description: "Configuration from user request",
			Location:    "config",
			FeatureID:   uuid.Nil,
			Visibility:  analysis.VisibilityLevelDeveloperVisible,
			IsPrimary:   false,
		})
	}

	return entryPoints, nil
}

// GetWiringPatternByText gets wiring pattern from a text prompt (not feature UUID).
func (e *GraphEngine) GetWiringPatternByText(ctx context.Context, prompt string) (*analysis.WiringPattern, error) {
	// Create a synthetic feature from the prompt
	feature := &graph.Node{ID: uuid.New(), Type: graph.NodeTypeTask}
	featureData := map[string]interface{}{
		"name":        "user_request_feature",
		"description": prompt,
	}
	feature.Data, _ = json.Marshal(featureData)

	return e.WiringAnalyzer.GetWiringPattern(ctx, feature.ID)
}

// CreateWorkflowWithSignature creates a workflow with the correct signature (taskID, category, description, priority).
func (e *GraphEngine) CreateWorkflowWithSignature(ctx context.Context, taskID uuid.UUID, category, description string, priority int) (*workflow.TodoWorkflow, error) {
	workflow, err := e.TodoEngine.CreateWorkflow(ctx, taskID, category, description, description)
	if err != nil {
		return nil, err
	}
	// Priority is handled via the todo items added to the workflow
	return workflow, nil
}

// WireFeatureWithSignature wires a feature with the correct signature (feature description, similar features).
func (e *GraphEngine) WireFeatureWithSignature(ctx context.Context, featureDesc string, similarFeatures []analysis.FeatureMatch) (*wiring.IntegrationResult, error) {
	featureAppearance := e.buildFeatureAppearanceFromDesc(uuid.New(), featureDesc, similarFeatures)
	return e.WiringEngine.WireFeature(ctx, featureAppearance, e.toSimilarFeaturesFromMatches(similarFeatures))
}

// ValidateAppearanceAndGenerateExamples validates appearance and generates usage examples.
func (e *GraphEngine) ValidateAppearanceAndGenerateExamples(ctx context.Context, featureAppearance *wiring.FeatureAppearance) (*wiring.ValidationResult, []wiring.UsageExample, error) {
	validationResult := e.AppearanceValidator.ValidateAppearance(ctx, featureAppearance)
	examples, err := e.AppearanceValidator.GenerateUsageExamples(ctx, featureAppearance)
	return validationResult, examples, err
}

// StartCleanupScheduler starts the periodic cleanup scheduler.
func (e *GraphEngine) StartCleanupScheduler(ctx context.Context) error {
	return e.CleanupScheduler.ScheduleCleanup(e.CleanupScheduler.GetConfig().Interval)
}

// StopCleanupScheduler stops the periodic cleanup scheduler.
func (e *GraphEngine) StopCleanupScheduler() error {
	return e.CleanupScheduler.Stop()
}

// RunMaintenanceNow runs full graph maintenance immediately.
func (e *GraphEngine) RunMaintenanceNow(ctx context.Context) (*healing.CleanupStats, error) {
	return e.GraphMaintenance.RunFullMaintenance(ctx)
}

// GetGraphStats returns statistics about the graph.
func (e *GraphEngine) GetGraphStats(ctx context.Context) (map[string]int, error) {
	return e.Manager.GetStats(ctx)
}

// Helper methods

func (e *GraphEngine) buildFeatureAppearance(taskID uuid.UUID, request string, matches []analysis.FeatureMatch, entryPoints []analysis.EntryPoint, wiringPattern *analysis.WiringPattern) *wiring.FeatureAppearance {
	appearance := &wiring.FeatureAppearance{
		FeatureID:   taskID,
		EntryPoints: make(map[wiring.EntryPointType][]wiring.EntryPointInterface),
		ConfigKeys:  []wiring.ConfigEntryPoint{},
		EnvironmentVars: []string{},
		Documentation: wiring.FeatureDocumentation{
			Readme: request,
		},
		Tests:        wiring.FeatureTests{},
		Dependencies: []uuid.UUID{},
		Conflicts:    []uuid.UUID{},
		Version:      "1.0.0",
	}

	// Convert analysis entry points to wiring entry points
	for _, ep := range entryPoints {
		wiringEP := e.convertAnalysisEntryPoint(ep)
		if wiringEP != nil {
			appearance.EntryPoints[wiringEP.GetEntryPointType()] = append(
				appearance.EntryPoints[wiringEP.GetEntryPointType()], wiringEP)
		}
	}

	// Add config keys based on wiring pattern
	if wiringPattern != nil {
		for _, configKey := range wiringPattern.ConfigKeys {
			appearance.ConfigKeys = append(appearance.ConfigKeys, wiring.ConfigEntryPoint{
				EntryPoint: wiring.EntryPoint{
					ID:          uuid.New(),
					Type:        wiring.EntryPointTypeConfig,
					Name:        configKey,
					Description: "Config key from wiring pattern: " + configKey,
					Path:        configKey,
					Handler:     "config.Get" + strings.Title(strings.ReplaceAll(configKey, ".", "")),
					Priority:    50,
					Enabled:     true,
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				},
				ConfigKey:       configKey,
				EnvVar:          strings.ToUpper(strings.ReplaceAll(configKey, ".", "_")),
				ConfigType:      "string",
				DefaultValue:    "",
				Description:     "Config key from wiring pattern: " + configKey,
				ValidationRegex: "",
				Required:        false,
				Secret:          false,
				Deprecated:      false,
			})
		}
	}

	return appearance
}

func (e *GraphEngine) buildFeatureAppearanceFromDesc(featureID uuid.UUID, desc string, matches []analysis.FeatureMatch) *wiring.FeatureAppearance {
	entryPoints, _ := e.DetectEntryPointsFromText(context.Background(), desc)
	return e.buildFeatureAppearance(featureID, desc, matches, entryPoints, nil)
}

func (e *GraphEngine) convertAnalysisEntryPoint(ep analysis.EntryPoint) wiring.EntryPointInterface {
	switch ep.EntryType {
	case analysis.EntryPointTypeTUICommand:
		return &wiring.TUIEntryPoint{
			EntryPoint: wiring.EntryPoint{
				ID:          ep.EntryID,
				Type:        wiring.EntryPointTypeTUI,
				Name:        ep.Name,
				Description: ep.Description,
				Path:        ep.Location,
				Handler:     "commands." + strings.Title(strings.ReplaceAll(ep.Name, "-", "")) + "Command",
				Priority:    100,
				Enabled:     true,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			},
			CommandName:  ep.Name,
			MenuPath:     "User Commands",
			KeyBinding:   "",
			Aliases:      []string{},
			Category:     "User",
			Hidden:       false,
			RequiresAuth: false,
		}
	case analysis.EntryPointTypeAPIEndpoint:
		return &wiring.APIEntryPoint{
			EntryPoint: wiring.EntryPoint{
				ID:          ep.EntryID,
				Type:        wiring.EntryPointTypeAPI,
				Name:        ep.Name,
				Description: ep.Description,
				Path:        ep.Location,
				Handler:     "api." + strings.Title(strings.ReplaceAll(ep.Name, "/", "")) + "Handler",
				Priority:    100,
				Enabled:     true,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			},
			Method:        "POST",
			Route:         ep.Location,
			Websocket:     false,
			Middleware:    []string{"logging", "recovery"},
			AuthRequired:  true,
			RateLimit:     &wiring.RateLimitConfig{Requests: 60, Window: time.Minute, Burst: 5},
		}
	case analysis.EntryPointTypeCLIFlag:
		return &wiring.CLIEntryPoint{
			EntryPoint: wiring.EntryPoint{
				ID:          ep.EntryID,
				Type:        wiring.EntryPointTypeCLI,
				Name:        ep.Name,
				Description: ep.Description,
				Path:        ep.Location,
				Handler:     "commands." + strings.Title(ep.Name) + "Cmd",
				Priority:    90,
				Enabled:     true,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			},
			FlagName:    ep.Name,
			ShortFlag:   "",
			CommandName: ep.Name,
			Args:        []wiring.CLIArg{},
			EnvVar:      "",
			Required:    false,
			Hidden:      false,
		}
	case analysis.EntryPointTypeConfigKey:
		return &wiring.ConfigEntryPoint{
			EntryPoint: wiring.EntryPoint{
				ID:          ep.EntryID,
				Type:        wiring.EntryPointTypeConfig,
				Name:        ep.Name,
				Description: ep.Description,
				Path:        ep.Location,
				Handler:     "config.Get" + strings.Title(strings.ReplaceAll(ep.Name, ".", "")),
				Priority:    50,
				Enabled:     true,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			},
			ConfigKey:       ep.Name,
			EnvVar:          strings.ToUpper(strings.ReplaceAll(ep.Name, ".", "_")),
			ConfigType:      "string",
			DefaultValue:    "",
			Description:     ep.Description,
			ValidationRegex: "",
			Required:        false,
			Secret:          false,
			Deprecated:      false,
		}
	}
	return nil
}

func (e *GraphEngine) toSimilarFeatures() []*wiring.SimilarFeature {
	return []*wiring.SimilarFeature{}
}

func (e *GraphEngine) toSimilarFeaturesFromMatches(matches []analysis.FeatureMatch) []*wiring.SimilarFeature {
	var similar []*wiring.SimilarFeature
	for _, match := range matches {
		similar = append(similar, &wiring.SimilarFeature{
			FeatureID:   match.FeatureID,
			Name:        match.FeatureName,
			Similarity:  match.Similarity,
			Patterns:    []wiring.WiringPattern{},
			EntryPoints: []wiring.EntryPointType{},
		})
	}
	return similar
}

func (e *GraphEngine) extractKeywords(text string) []string {
	var keywords []string
	for _, word := range strings.Fields(strings.ToLower(text)) {
		if len(word) >= 3 {
			keywords = append(keywords, word)
		}
	}
	return keywords
}

func (e *GraphEngine) extractSymbols(text string) []string {
	var symbols []string
	for _, word := range strings.Fields(text) {
		if strings.Contains(word, "(") || strings.Contains(word, ".") ||
			strings.Contains(word, "_") || (len(word) > 2 && word[0] >= 'A' && word[0] <= 'Z') {
			symbols = append(symbols, word)
		}
	}
	return symbols
}

func (e *GraphEngine) cosineSimilarityText(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	setA := make(map[string]bool)
	for _, s := range a {
		setA[s] = true
	}
	intersection := 0
	for _, s := range b {
		if setA[s] {
			intersection++
		}
	}
	union := len(a) + len(b) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func (e *GraphEngine) findMatchedSymbols(a, b []string) []string {
	setB := make(map[string]bool)
	for _, s := range b {
		setB[s] = true
	}
	var matched []string
	for _, s := range a {
		if setB[s] {
			matched = append(matched, s)
		}
	}
	return matched
}

func (e *GraphEngine) findSharedDeps(a, b []string) []string {
	setB := make(map[string]bool)
	for _, s := range b {
		setB[s] = true
	}
	var shared []string
	for _, s := range a {
		if setB[s] {
			shared = append(shared, s)
		}
	}
	return shared
}

func (e *GraphEngine) generateMatchReason(textSim, graphSim float64, keywords, symbols []string, sig analysis.FeatureSignature) string {
	var reasons []string
	if textSim > 0.5 {
		reasons = append(reasons, "high textual similarity")
	}
	if graphSim > 0.5 {
		reasons = append(reasons, "close graph proximity")
	}
	if len(e.findMatchedSymbols(symbols, sig.Symbols)) > 0 {
		reasons = append(reasons, "shared symbols")
	}
	if len(e.findSharedDeps([]string{}, sig.Dependencies)) > 0 {
		reasons = append(reasons, "shared dependencies")
	}
	if len(reasons) == 0 {
		return "low similarity match"
	}
	return strings.Join(reasons, "; ")
}

func (e *GraphEngine) extractCommandName(content string) string {
	parts := strings.Fields(content)
	for i, part := range parts {
		if strings.HasPrefix(part, ":") || strings.HasPrefix(part, "cmd") {
			if i+1 < len(parts) {
				return parts[i+1]
			}
		}
	}
	return "unknown_command"
}

func (e *GraphEngine) extractEndpointName(path string) string {
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}

func (e *GraphEngine) extractFlagName(path string) string {
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}

func (e *GraphEngine) extractConfigKey(path string) string {
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}

func extractErrorMessages(errors []wiring.ValidationError) []string {
	var messages []string
	for _, err := range errors {
		messages = append(messages, err.Message)
	}
	return messages
}

// Getters for internal components (for testing/debugging)

func (e *GraphEngine) GetFeatureAnalyzer() *analysis.FeatureAnalyzer {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.FeatureAnalyzer
}

func (e *GraphEngine) GetEntryPointDetector() *analysis.EntryPointDetector {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.EntryPointDetector
}

func (e *GraphEngine) GetWiringAnalyzer() *analysis.WiringAnalyzer {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.WiringAnalyzer
}

func (e *GraphEngine) GetImpactAnalyzer() *analysis.ImpactAnalyzer {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.ImpactAnalyzer
}

func (e *GraphEngine) GetTodoEngine() *workflow.TodoWorkflowEngine {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.TodoEngine
}

func (e *GraphEngine) GetWiringEngine() *wiring.WiringEngine {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.WiringEngine
}

func (e *GraphEngine) GetAppearanceValidator() *wiring.AppearanceValidator {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.AppearanceValidator
}

func (e *GraphEngine) GetContinuityManager() *continuity.ContinuityManager {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.ContinuityManager
}

func (e *GraphEngine) GetTaskRouter() *continuity.TaskRouter {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.TaskRouter
}

func (e *GraphEngine) GetContextResumer() *continuity.ContextResumer {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.ContextResumer
}

func (e *GraphEngine) GetUndoManager() *continuity.UndoManager {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.UndoManager
}

func (e *GraphEngine) GetFixValidator() *healing.FixValidator {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.FixValidator
}

func (e *GraphEngine) GetContextCleanup() *healing.ContextCleanup {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.ContextCleanup
}

func (e *GraphEngine) GetGraphMaintenance() *healing.GraphMaintenance {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.GraphMaintenance
}

func (e *GraphEngine) GetCleanupScheduler() *healing.CleanupScheduler {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.CleanupScheduler
}