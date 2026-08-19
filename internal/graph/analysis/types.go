package analysis

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type FeatureMatch struct {
	FeatureID      uuid.UUID `json:"feature_id"`
	FeatureName    string    `json:"feature_name"`
	Similarity     float64   `json:"similarity"`
	TextSimilarity float64   `json:"text_similarity"`
	GraphSimilarity float64  `json:"graph_similarity"`
	MatchReason    string    `json:"match_reason"`
	MatchedSymbols []string  `json:"matched_symbols"`
	SharedDeps     []string  `json:"shared_dependencies"`
}

type WiringPattern struct {
	PatternID     uuid.UUID              `json:"pattern_id"`
	Name          string                 `json:"name"`
	Description   string                 `json:"description"`
	Steps         []WiringStep           `json:"steps"`
	ConfigKeys    []string               `json:"config_keys"`
	Providers     []string               `json:"providers"`
	Clients       []string               `json:"clients"`
	RequestFlow   []string               `json:"request_flow"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	Frequency     int                    `json:"frequency"`
	Confidence    float64                `json:"confidence"`
}

type WiringStep struct {
	StepOrder    int                    `json:"step_order"`
	FromType     string                 `json:"from_type"`
	ToType       string                 `json:"to_type"`
	EdgeType     string                 `json:"edge_type"`
	Description  string                 `json:"description"`
	Properties   map[string]interface{} `json:"properties,omitempty"`
}

type EntryPoint struct {
	EntryID        uuid.UUID         `json:"entry_id"`
	EntryType      EntryPointType    `json:"entry_type"`
	Name           string            `json:"name"`
	Description    string            `json:"description"`
	Location       string            `json:"location"`
	FeatureID      uuid.UUID         `json:"feature_id"`
	Parameters     map[string]string `json:"parameters,omitempty"`
	Visibility     VisibilityLevel   `json:"visibility"`
	IsPrimary      bool              `json:"is_primary"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

type EntryPointType string

const (
	EntryPointTypeTUICommand   EntryPointType = "tui_command"
	EntryPointTypeAPIEndpoint  EntryPointType = "api_endpoint"
	EntryPointTypeCLIFlag      EntryPointType = "cli_flag"
	EntryPointTypeConfigKey    EntryPointType = "config_key"
	EntryPointTypeEvent        EntryPointType = "event"
	EntryPointTypeHook         EntryPointType = "hook"
	EntryPointTypeInterface    EntryPointType = "interface"
	EntryPointTypeCallback     EntryPointType = "callback"
)

type VisibilityLevel string

const (
	VisibilityLevelUserVisible    VisibilityLevel = "user_visible"
	VisibilityLevelDeveloperVisible VisibilityLevel = "developer_visible"
	VisibilityLevelInternal       VisibilityLevel = "internal"
	VisibilityLevelHidden         VisibilityLevel = "hidden"
)

type ImpactScope struct {
	ScopeID          uuid.UUID         `json:"scope_id"`
	SourceFile       string            `json:"source_file"`
	SourceNodeID     uuid.UUID         `json:"source_node_id"`
	AffectedFeatures []FeatureImpact   `json:"affected_features"`
	AffectedTests    []TestImpact      `json:"affected_tests"`
	Dependencies     []DependencyImpact `json:"dependencies"`
	Dependents       []DependentImpact `json:"dependents"`
	ConfigFiles      []string          `json:"config_files"`
	BlastRadius      int               `json:"blast_radius"`
	RiskLevel        RiskLevel         `json:"risk_level"`
	GeneratedAt      time.Time         `json:"generated_at"`
}

type FeatureImpact struct {
	FeatureID   uuid.UUID `json:"feature_id"`
	FeatureName string    `json:"feature_name"`
	ImpactType  string    `json:"impact_type"`
	Severity    float64   `json:"severity"`
	Path        []string  `json:"path"`
}

type TestImpact struct {
	TestFile    string  `json:"test_file"`
	TestName    string  `json:"test_name"`
	Coverage    float64 `json:"coverage"`
	Severity    float64 `json:"severity"`
}

type DependencyImpact struct {
	DepID       uuid.UUID `json:"dep_id"`
	DepName     string    `json:"dep_name"`
	DepType     string    `json:"dep_type"`
	ImpactDepth int       `json:"impact_depth"`
	Severity    float64   `json:"severity"`
}

type DependentImpact struct {
	DepID       uuid.UUID `json:"dep_id"`
	DepName     string    `json:"dep_name"`
	DepType     string    `json:"dep_type"`
	ImpactDepth int       `json:"impact_depth"`
	Severity    float64   `json:"severity"`
}

type RiskLevel string

const (
	RiskLevelLow      RiskLevel = "low"
	RiskLevelMedium   RiskLevel = "medium"
	RiskLevelHigh     RiskLevel = "high"
	RiskLevelCritical RiskLevel = "critical"
)

type SimilarityConfig struct {
	TextWeight       float64 `json:"text_weight"`
	GraphWeight      float64 `json:"graph_weight"`
	MinSimilarity    float64 `json:"min_similarity"`
	MaxResults       int     `json:"max_results"`
	TFIDFMaxFeatures int     `json:"tfidf_max_features"`
	GraphMaxDepth    int     `json:"graph_max_depth"`
	IncludeSymbols   bool    `json:"include_symbols"`
	IncludeDeps      bool    `json:"include_dependencies"`
}

func DefaultSimilarityConfig() SimilarityConfig {
	return SimilarityConfig{
		TextWeight:       0.6,
		GraphWeight:      0.4,
		MinSimilarity:    0.3,
		MaxResults:       10,
		TFIDFMaxFeatures: 1000,
		GraphMaxDepth:    3,
		IncludeSymbols:   true,
		IncludeDeps:      true,
	}
}

type AnalysisResult struct {
	ResultID      uuid.UUID              `json:"result_id"`
	AnalysisType  string                 `json:"analysis_type"`
	TargetID      uuid.UUID              `json:"target_id"`
	TargetType    string                 `json:"target_type"`
	Data          json.RawMessage        `json:"data"`
	Confidence    float64                `json:"confidence"`
	ProcessingTime int64                 `json:"processing_time_ms"`
	GeneratedAt   time.Time              `json:"generated_at"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

type FeatureSignature struct {
	FeatureID     uuid.UUID `json:"feature_id"`
	FeatureName   string    `json:"feature_name"`
	Keywords      []string  `json:"keywords"`
	Symbols       []string  `json:"symbols"`
	Dependencies  []string  `json:"dependencies"`
	EntryPoints   []string  `json:"entry_points"`
	TFIDFVector   []float64 `json:"tfidf_vector,omitempty"`
	GraphEmbedding []float64 `json:"graph_embedding,omitempty"`
}

type IntegrationPoint struct {
	PointID       uuid.UUID              `json:"point_id"`
	PointType     IntegrationPointType   `json:"point_type"`
	Name          string                 `json:"name"`
	Description   string                 `json:"description"`
	SourceFeature uuid.UUID              `json:"source_feature"`
	TargetFeature uuid.UUID              `json:"target_feature"`
	Interface     string                 `json:"interface"`
	Direction     IntegrationDirection   `json:"direction"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

type IntegrationPointType string

const (
	IntegrationPointTypeEvent     IntegrationPointType = "event"
	IntegrationPointTypeHook      IntegrationPointType = "hook"
	IntegrationPointTypeInterface IntegrationPointType = "interface"
	IntegrationPointTypeCallback  IntegrationPointType = "callback"
	IntegrationPointTypeChannel   IntegrationPointType = "channel"
	IntegrationPointTypeQueue     IntegrationPointType = "queue"
)

type IntegrationDirection string

const (
	IntegrationDirectionInbound  IntegrationDirection = "inbound"
	IntegrationDirectionOutbound IntegrationDirection = "outbound"
	IntegrationDirectionBidirectional IntegrationDirection = "bidirectional"
)

type IntegrationPath struct {
	PathID       uuid.UUID    `json:"path_id"`
	FromFeature  uuid.UUID    `json:"from_feature"`
	ToFeature    uuid.UUID    `json:"to_feature"`
	Steps        []PathStep   `json:"steps"`
	TotalCost    float64      `json:"total_cost"`
	Confidence   float64      `json:"confidence"`
	Description  string       `json:"description"`
}

type PathStep struct {
	StepOrder    int                    `json:"step_order"`
	FromNodeID   uuid.UUID              `json:"from_node_id"`
	ToNodeID     uuid.UUID              `json:"to_node_id"`
	EdgeType     string                 `json:"edge_type"`
	Description  string                 `json:"description"`
	Properties   map[string]interface{} `json:"properties,omitempty"`
}

type DependencyChain struct {
	ChainID       uuid.UUID         `json:"chain_id"`
	RootNodeID    uuid.UUID         `json:"root_node_id"`
	RootFile      string            `json:"root_file"`
	Nodes         []ChainNode       `json:"nodes"`
	MaxDepth      int               `json:"max_depth"`
	TotalNodes    int               `json:"total_nodes"`
	CircularRefs  []CircularRef     `json:"circular_refs,omitempty"`
}

type ChainNode struct {
	NodeID       uuid.UUID `json:"node_id"`
	NodeType     string    `json:"node_type"`
	FilePath     string    `json:"file_path"`
	Depth        int       `json:"depth"`
	DepType      string    `json:"dep_type"`
	IsOptional   bool      `json:"is_optional"`
	IsExternal   bool      `json:"is_external"`
}

type CircularRef struct {
	Nodes     []uuid.UUID `json:"nodes"`
	Files     []string    `json:"files"`
	Severity  float64     `json:"severity"`
}

func (fm *FeatureMatch) MarshalJSON() ([]byte, error) {
	type Alias FeatureMatch
	return json.Marshal(&struct {
		*Alias
		Similarity     float64 `json:"similarity"`
		TextSimilarity float64 `json:"text_similarity"`
		GraphSimilarity float64 `json:"graph_similarity"`
	}{
		Alias:          (*Alias)(fm),
		Similarity:     fm.Similarity,
		TextSimilarity: fm.TextSimilarity,
		GraphSimilarity: fm.GraphSimilarity,
	})
}

func (wp *WiringPattern) MarshalJSON() ([]byte, error) {
	type Alias WiringPattern
	return json.Marshal(&struct {
		*Alias
		Confidence float64 `json:"confidence"`
	}{
		Alias:      (*Alias)(wp),
		Confidence: wp.Confidence,
	})
}

func (ep *EntryPoint) MarshalJSON() ([]byte, error) {
	type Alias EntryPoint
	return json.Marshal(&struct {
		*Alias
	}{
		Alias: (*Alias)(ep),
	})
}

func (is *ImpactScope) MarshalJSON() ([]byte, error) {
	type Alias ImpactScope
	return json.Marshal(&struct {
		*Alias
		GeneratedAt time.Time `json:"generated_at"`
	}{
		Alias:      (*Alias)(is),
		GeneratedAt: is.GeneratedAt,
	})
}

func (sc *SimilarityConfig) MarshalJSON() ([]byte, error) {
	type Alias SimilarityConfig
	return json.Marshal((*Alias)(sc))
}

func (ar *AnalysisResult) MarshalJSON() ([]byte, error) {
	type Alias AnalysisResult
	return json.Marshal(&struct {
		*Alias
		GeneratedAt time.Time `json:"generated_at"`
	}{
		Alias:      (*Alias)(ar),
		GeneratedAt: ar.GeneratedAt,
	})
}

func (fs *FeatureSignature) MarshalJSON() ([]byte, error) {
	type Alias FeatureSignature
	return json.Marshal((*Alias)(fs))
}

func (ip *IntegrationPoint) MarshalJSON() ([]byte, error) {
	type Alias IntegrationPoint
	return json.Marshal((*Alias)(ip))
}

func (ip *IntegrationPath) MarshalJSON() ([]byte, error) {
	type Alias IntegrationPath
	return json.Marshal((*Alias)(ip))
}

func (dc *DependencyChain) MarshalJSON() ([]byte, error) {
	type Alias DependencyChain
	return json.Marshal((*Alias)(dc))
}