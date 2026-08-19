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

type FeatureAnalyzer struct {
	store  *graph.Store
	query  *graph.QueryEngine
	config SimilarityConfig
	mu     sync.RWMutex

	tfidfCache    map[uuid.UUID][]float64
	vocabulary    map[string]int
	idfValues     map[string]float64
	cacheMu       sync.RWMutex
}

func NewFeatureAnalyzer(store *graph.Store, query *graph.QueryEngine, config SimilarityConfig) *FeatureAnalyzer {
	if config.TextWeight == 0 && config.GraphWeight == 0 {
		config = DefaultSimilarityConfig()
	}
	return &FeatureAnalyzer{
		store:        store,
		query:        query,
		config:       config,
		tfidfCache:   make(map[uuid.UUID][]float64),
		vocabulary:   make(map[string]int),
		idfValues:    make(map[string]float64),
	}
}

func (fa *FeatureAnalyzer) FindSimilarFeatures(ctx context.Context, request FeatureMatchRequest, limit int) ([]FeatureMatch, error) {
	if limit <= 0 {
		limit = fa.config.MaxResults
	}

	targetFeature, err := fa.getFeatureByID(ctx, request.FeatureID)
	if err != nil {
		return nil, err
	}

	targetSig, err := fa.GetFeatureSignature(ctx, targetFeature)
	if err != nil {
		return nil, err
	}

	allFeatures, err := fa.getAllFeatures(ctx)
	if err != nil {
		return nil, err
	}

	var matches []FeatureMatch
	for _, feature := range allFeatures {
		if feature.ID == request.FeatureID {
			continue
		}

		sig, err := fa.GetFeatureSignature(ctx, feature)
		if err != nil {
			continue
		}

		textSim := fa.cosineSimilarity(targetSig.TFIDFVector, sig.TFIDFVector)
		graphSim := fa.graphProximitySimilarity(ctx, request.FeatureID, feature.ID)

		combinedSim := fa.config.TextWeight*textSim + fa.config.GraphWeight*graphSim

		if combinedSim >= fa.config.MinSimilarity {
			match := FeatureMatch{
				FeatureID:       feature.ID,
				FeatureName:     fa.getFeatureName(feature),
				Similarity:      combinedSim,
				TextSimilarity:  textSim,
				GraphSimilarity: graphSim,
				MatchReason:     fa.generateMatchReason(textSim, graphSim, targetSig, sig),
				MatchedSymbols:  fa.findMatchedSymbols(targetSig.Symbols, sig.Symbols),
				SharedDeps:      fa.findSharedDeps(targetSig.Dependencies, sig.Dependencies),
			}
			matches = append(matches, match)
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Similarity > matches[j].Similarity
	})

	if len(matches) > limit {
		matches = matches[:limit]
	}

	return matches, nil
}

type FeatureMatchRequest struct {
	FeatureID uuid.UUID `json:"feature_id"`
	Config    *SimilarityConfig `json:"config,omitempty"`
}

func (fa *FeatureAnalyzer) GetFeatureSignature(ctx context.Context, feature *graph.Node) (FeatureSignature, error) {
	var data map[string]interface{}
	if err := feature.UnmarshalData(&data); err != nil {
		return FeatureSignature{}, err
	}

	name := fa.getFeatureName(feature)
	keywords := fa.extractKeywords(data)
	symbols := fa.extractSymbols(data)
	deps := fa.extractDependencies(ctx, feature)
	entryPoints := fa.extractEntryPoints(data)

	fa.mu.RLock()
	vocab := fa.vocabulary
	idf := fa.idfValues
	fa.mu.RUnlock()

	if len(vocab) == 0 {
		fa.buildVocabulary(ctx)
		fa.mu.RLock()
		vocab = fa.vocabulary
		idf = fa.idfValues
		fa.mu.RUnlock()
	}

	tfidfVector := fa.computeTFIDF(keywords, symbols, deps, entryPoints, vocab, idf)

	return FeatureSignature{
		FeatureID:   feature.ID,
		FeatureName: name,
		Keywords:    keywords,
		Symbols:     symbols,
		Dependencies: deps,
		EntryPoints: entryPoints,
		TFIDFVector: tfidfVector,
	}, nil
}

func (fa *FeatureAnalyzer) buildVocabulary(ctx context.Context) {
	fa.mu.Lock()
	defer fa.mu.Unlock()

	features, err := fa.getAllFeatures(ctx)
	if err != nil {
		return
	}

	docCount := float64(len(features))
	termDocFreq := make(map[string]int)

	for _, feature := range features {
		var data map[string]interface{}
		if err := feature.UnmarshalData(&data); err != nil {
			continue
		}

		terms := fa.extractAllTerms(data)
		seen := make(map[string]bool)
		for _, term := range terms {
			if !seen[term] {
				termDocFreq[term]++
				seen[term] = true
			}
		}
	}

	vocab := make(map[string]int)
	idf := make(map[string]float64)
	idx := 0
	for term, df := range termDocFreq {
		if df > 0 && df < len(features) {
			vocab[term] = idx
			idf[term] = math.Log(docCount / float64(df))
			idx++
			if idx >= fa.config.TFIDFMaxFeatures {
				break
			}
		}
	}

	fa.vocabulary = vocab
	fa.idfValues = idf
}

func (fa *FeatureAnalyzer) computeTFIDF(keywords, symbols, deps, entryPoints []string, vocab map[string]int, idf map[string]float64) []float64 {
	vector := make([]float64, len(vocab))
	allTerms := append(append(append(keywords, symbols...), deps...), entryPoints...)

	termFreq := make(map[string]int)
	for _, term := range allTerms {
		termFreq[strings.ToLower(term)]++
	}

	maxFreq := 0
	for _, f := range termFreq {
		if f > maxFreq {
			maxFreq = f
		}
	}

	for term, freq := range termFreq {
		if idx, ok := vocab[term]; ok {
			tf := float64(freq) / float64(maxFreq)
			vector[idx] = tf * idf[term]
		}
	}

	norm := 0.0
	for _, v := range vector {
		norm += v * v
	}
	norm = math.Sqrt(norm)
	if norm > 0 {
		for i := range vector {
			vector[i] /= norm
		}
	}

	return vector
}

func (fa *FeatureAnalyzer) cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	dot := 0.0
	normA := 0.0
	normB := 0.0
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func (fa *FeatureAnalyzer) graphProximitySimilarity(ctx context.Context, from, to uuid.UUID) float64 {
	result, err := fa.query.ShortestPath(ctx, from, to, nil)
	if err != nil {
		return 0
	}

	if result == nil || len(result.Path) <= 1 {
		return 0
	}

	distance := float64(len(result.Path) - 1)
	maxDepth := float64(fa.config.GraphMaxDepth)
	if distance > maxDepth {
		return 0
	}

	return 1.0 - (distance / maxDepth)
}

func (fa *FeatureAnalyzer) extractKeywords(data map[string]interface{}) []string {
	var keywords []string
	for k, v := range data {
		if strings.Contains(strings.ToLower(k), "name") ||
			strings.Contains(strings.ToLower(k), "title") ||
			strings.Contains(strings.ToLower(k), "description") ||
			strings.Contains(strings.ToLower(k), "tag") {
			if str, ok := v.(string); ok {
				keywords = append(keywords, fa.tokenize(str)...)
			}
		}
	}
	return fa.deduplicate(keywords)
}

func (fa *FeatureAnalyzer) extractSymbols(data map[string]interface{}) []string {
	var symbols []string
	for k, v := range data {
		if strings.Contains(strings.ToLower(k), "symbol") ||
			strings.Contains(strings.ToLower(k), "function") ||
			strings.Contains(strings.ToLower(k), "method") ||
			strings.Contains(strings.ToLower(k), "struct") ||
			strings.Contains(strings.ToLower(k), "interface") ||
			strings.Contains(strings.ToLower(k), "type") {
			switch val := v.(type) {
			case string:
				symbols = append(symbols, val)
			case []interface{}:
				for _, item := range val {
					if s, ok := item.(string); ok {
						symbols = append(symbols, s)
					}
				}
			}
		}
	}
	return fa.deduplicate(symbols)
}

func (fa *FeatureAnalyzer) extractDependencies(ctx context.Context, feature *graph.Node) []string {
	var deps []string
	edges, err := fa.store.GetEdgesFrom(ctx, feature.ID, graph.EdgeTypeDependsOn)
	if err != nil {
		return deps
	}
	for _, edge := range edges {
		node, err := fa.store.GetNode(ctx, edge.ToID)
		if err == nil {
			var data map[string]interface{}
			if node.UnmarshalData(&data) == nil {
				if name, ok := data["name"].(string); ok {
					deps = append(deps, name)
				} else if title, ok := data["title"].(string); ok {
					deps = append(deps, title)
				}
			}
		}
	}
	return fa.deduplicate(deps)
}

func (fa *FeatureAnalyzer) extractEntryPoints(data map[string]interface{}) []string {
	var eps []string
	for k, v := range data {
		if strings.Contains(strings.ToLower(k), "entry") ||
			strings.Contains(strings.ToLower(k), "command") ||
			strings.Contains(strings.ToLower(k), "endpoint") ||
			strings.Contains(strings.ToLower(k), "handler") {
			if str, ok := v.(string); ok {
				eps = append(eps, str)
			}
		}
	}
	return fa.deduplicate(eps)
}

func (fa *FeatureAnalyzer) extractAllTerms(data map[string]interface{}) []string {
	var terms []string
	for k, v := range data {
		terms = append(terms, fa.tokenize(k)...)
		if str, ok := v.(string); ok {
			terms = append(terms, fa.tokenize(str)...)
		}
	}
	return terms
}

func (fa *FeatureAnalyzer) tokenize(text string) []string {
	text = strings.ToLower(text)
	text = strings.ReplaceAll(text, "_", " ")
	text = strings.ReplaceAll(text, "-", " ")
	text = strings.ReplaceAll(text, ".", " ")
	text = strings.ReplaceAll(text, "/", " ")
	text = strings.ReplaceAll(text, "\\", " ")

	var tokens []string
	for _, word := range strings.Fields(text) {
		if len(word) >= 3 {
			tokens = append(tokens, word)
		}
	}
	return tokens
}

func (fa *FeatureAnalyzer) deduplicate(slice []string) []string {
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

func (fa *FeatureAnalyzer) findMatchedSymbols(a, b []string) []string {
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

func (fa *FeatureAnalyzer) findSharedDeps(a, b []string) []string {
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

func (fa *FeatureAnalyzer) generateMatchReason(textSim, graphSim float64, target, candidate FeatureSignature) string {
	var reasons []string
	if textSim > 0.5 {
		reasons = append(reasons, "high textual similarity")
	}
	if graphSim > 0.5 {
		reasons = append(reasons, "close graph proximity")
	}
	if len(fa.findMatchedSymbols(target.Symbols, candidate.Symbols)) > 0 {
		reasons = append(reasons, "shared symbols")
	}
	if len(fa.findSharedDeps(target.Dependencies, candidate.Dependencies)) > 0 {
		reasons = append(reasons, "shared dependencies")
	}
	if len(reasons) == 0 {
		return "low similarity match"
	}
	return strings.Join(reasons, "; ")
}

func (fa *FeatureAnalyzer) getFeatureName(node *graph.Node) string {
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

func (fa *FeatureAnalyzer) getFeatureByID(ctx context.Context, id uuid.UUID) (*graph.Node, error) {
	return fa.store.GetNode(ctx, id)
}

func (fa *FeatureAnalyzer) getAllFeatures(ctx context.Context) ([]*graph.Node, error) {
	return fa.store.ListNodes(ctx, graph.NodeTypeTask, 0, 0)
}