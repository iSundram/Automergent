package graph

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"

	"github.com/google/uuid"
)

type QueryEngine struct {
	store *Store
}

func NewQueryEngine(store *Store) *QueryEngine {
	return &QueryEngine{store: store}
}

type TraversalOptions struct {
	MaxDepth       int
	EdgeTypes      []EdgeType
	NodeTypes      []NodeType
	Direction      TraversalDirection
	FilterFunc     func(*Node) bool
	IncludeStart   bool
}

type TraversalDirection int

const (
	TraversalOutbound TraversalDirection = iota
	TraversalInbound
	TraversalBoth
)

type TraversalResult struct {
	Nodes []*Node
	Edges []*Edge
	Paths [][]*Node
}

func (q *QueryEngine) Traverse(ctx context.Context, startID uuid.UUID, opts TraversalOptions) (*TraversalResult, error) {
	if opts.MaxDepth <= 0 {
		opts.MaxDepth = 10
	}

	visited := make(map[uuid.UUID]bool)
	var result TraversalResult
	var mu sync.Mutex

	startNode, err := q.store.GetNode(ctx, startID)
	if err != nil {
		return nil, err
	}

	if opts.IncludeStart {
		mu.Lock()
		result.Nodes = append(result.Nodes, startNode)
		visited[startID] = true
		mu.Unlock()
	}

	var traverse func(ctx context.Context, currentID uuid.UUID, depth int, path []*Node)
	traverse = func(ctx context.Context, currentID uuid.UUID, depth int, path []*Node) {
		if depth >= opts.MaxDepth {
			return
		}

		var edges []*Edge
		switch opts.Direction {
		case TraversalOutbound:
			edges, _ = q.store.GetEdgesFrom(ctx, currentID, "")
		case TraversalInbound:
			edges, _ = q.store.GetEdgesTo(ctx, currentID, "")
		case TraversalBoth:
			outEdges, _ := q.store.GetEdgesFrom(ctx, currentID, "")
			inEdges, _ := q.store.GetEdgesTo(ctx, currentID, "")
			edges = append(outEdges, inEdges...)
		}

		for _, edge := range edges {
			if len(opts.EdgeTypes) > 0 {
				match := false
				for _, et := range opts.EdgeTypes {
					if edge.Type == et {
						match = true
						break
					}
				}
				if !match {
					continue
				}
			}

			nextID := edge.ToID
			if opts.Direction == TraversalInbound {
				nextID = edge.FromID
			}

			if visited[nextID] {
				continue
			}

			node, err := q.store.GetNode(ctx, nextID)
			if err != nil {
				continue
			}

			if len(opts.NodeTypes) > 0 {
				match := false
				for _, nt := range opts.NodeTypes {
					if node.Type == nt {
						match = true
						break
					}
				}
				if !match {
					continue
				}
			}

			if opts.FilterFunc != nil && !opts.FilterFunc(node) {
				continue
			}

			mu.Lock()
			visited[nextID] = true
			result.Nodes = append(result.Nodes, node)
			result.Edges = append(result.Edges, edge)
			newPath := append(append([]*Node{}, path...), node)
			result.Paths = append(result.Paths, newPath)
			mu.Unlock()

			traverse(ctx, nextID, depth+1, newPath)
		}
	}

	traverse(ctx, startID, 0, []*Node{startNode})
	return &result, nil
}

type PatternMatch struct {
	NodeType   NodeType
	EdgeType   EdgeType
	Direction  TraversalDirection
	Properties map[string]interface{}
}

type PatternQuery struct {
	Patterns []PatternMatch
	Limit    int
}

func (q *QueryEngine) MatchPattern(ctx context.Context, query PatternQuery) ([][]*Node, error) {
	if len(query.Patterns) == 0 {
		return nil, fmt.Errorf("no patterns specified")
	}

	if query.Limit <= 0 {
		query.Limit = 100
	}

	firstPattern := query.Patterns[0]
	startNodes, err := q.store.ListNodes(ctx, firstPattern.NodeType, query.Limit*10, 0)
	if err != nil {
		return nil, err
	}

	var results [][]*Node
	for _, startNode := range startNodes {
		matches := q.matchFromNode(ctx, startNode, query.Patterns, 0, []*Node{startNode})
		results = append(results, matches...)
		if len(results) >= query.Limit {
			break
		}
	}

	return results[:min(len(results), query.Limit)], nil
}

func (q *QueryEngine) matchFromNode(ctx context.Context, current *Node, patterns []PatternMatch, idx int, path []*Node) [][]*Node {
	if idx >= len(patterns) {
		return [][]*Node{path}
	}

	pattern := patterns[idx]
	var edges []*Edge
	var err error

	switch pattern.Direction {
	case TraversalOutbound:
		edges, err = q.store.GetEdgesFrom(ctx, current.ID, pattern.EdgeType)
	case TraversalInbound:
		edges, err = q.store.GetEdgesTo(ctx, current.ID, pattern.EdgeType)
	case TraversalBoth:
		outEdges, _ := q.store.GetEdgesFrom(ctx, current.ID, pattern.EdgeType)
		inEdges, _ := q.store.GetEdgesTo(ctx, current.ID, pattern.EdgeType)
		edges = append(outEdges, inEdges...)
	}

	if err != nil || len(edges) == 0 {
		return nil
	}

	var results [][]*Node
	for _, edge := range edges {
		nextID := edge.ToID
		if pattern.Direction == TraversalInbound {
			nextID = edge.FromID
		}

		node, err := q.store.GetNode(ctx, nextID)
		if err != nil {
			continue
		}

		if pattern.NodeType != "" && node.Type != pattern.NodeType {
			continue
		}

		if len(pattern.Properties) > 0 {
			if !q.matchProperties(node, pattern.Properties) {
				continue
			}
		}

		newPath := append(append([]*Node{}, path...), node)
		matches := q.matchFromNode(ctx, node, patterns, idx+1, newPath)
		results = append(results, matches...)
	}

	return results
}

func (q *QueryEngine) matchProperties(node *Node, props map[string]interface{}) bool {
	var data map[string]interface{}
	if err := node.UnmarshalData(&data); err != nil {
		return false
	}

	for k, v := range props {
		if data[k] != v {
			return false
		}
	}
	return true
}

type ShortestPathResult struct {
	Path  []*Node
	Edges []*Edge
	Cost  float64
}

func (q *QueryEngine) ShortestPath(ctx context.Context, from, to uuid.UUID, edgeTypes []EdgeType) (*ShortestPathResult, error) {
	if from == to {
		node, err := q.store.GetNode(ctx, from)
		if err != nil {
			return nil, err
		}
		return &ShortestPathResult{Path: []*Node{node}, Cost: 0}, nil
	}

	type queueItem struct {
		nodeID  uuid.UUID
		path    []uuid.UUID
		edges   []*Edge
		cost    float64
	}

	visited := make(map[uuid.UUID]float64)
	queue := []queueItem{{nodeID: from, path: []uuid.UUID{from}, cost: 0}}

	for len(queue) > 0 {
		sort.Slice(queue, func(i, j int) bool {
			return queue[i].cost < queue[j].cost
		})

		current := queue[0]
		queue = queue[1:]

		if current.cost > visited[current.nodeID] {
			continue
		}

		if current.nodeID == to {
			nodes := make([]*Node, len(current.path))
			for i, id := range current.path {
				node, err := q.store.GetNode(ctx, id)
				if err != nil {
					return nil, err
				}
				nodes[i] = node
			}
			return &ShortestPathResult{
				Path:  nodes,
				Edges: current.edges,
				Cost:  current.cost,
			}, nil
		}

		var edges []*Edge
		if len(edgeTypes) == 0 {
			edges, _ = q.store.GetEdgesFrom(ctx, current.nodeID, "")
		} else {
			for _, et := range edgeTypes {
				e, _ := q.store.GetEdgesFrom(ctx, current.nodeID, et)
				edges = append(edges, e...)
			}
		}

		for _, edge := range edges {
			nextID := edge.ToID
			newCost := current.cost + 1

			if existingCost, ok := visited[nextID]; ok && newCost >= existingCost {
				continue
			}

			visited[nextID] = newCost
			newPath := append(append([]uuid.UUID{}, current.path...), nextID)
			newEdges := append(append([]*Edge{}, current.edges...), edge)
			queue = append(queue, queueItem{
				nodeID: nextID,
				path:   newPath,
				edges:  newEdges,
				cost:   newCost,
			})
		}
	}

	return nil, fmt.Errorf("no path found between %s and %s", from, to)
}

func (q *QueryEngine) AllShortestPaths(ctx context.Context, from, to uuid.UUID, edgeTypes []EdgeType, maxPaths int) ([]*ShortestPathResult, error) {
	if maxPaths <= 0 {
		maxPaths = 10
	}

	type queueItem struct {
		nodeID  uuid.UUID
		path    []uuid.UUID
		edges   []*Edge
		cost    float64
	}

	visited := make(map[uuid.UUID]float64)
	queue := []queueItem{{nodeID: from, path: []uuid.UUID{from}, cost: 0}}
	var results []*ShortestPathResult
	minCost := math.MaxFloat64

	for len(queue) > 0 && len(results) < maxPaths {
		sort.Slice(queue, func(i, j int) bool {
			return queue[i].cost < queue[j].cost
		})

		current := queue[0]
		queue = queue[1:]

		if current.cost > visited[current.nodeID] {
			continue
		}

		if current.nodeID == to {
			if current.cost < minCost {
				minCost = current.cost
				results = nil
			}
			if current.cost == minCost {
				nodes := make([]*Node, len(current.path))
				for i, id := range current.path {
					node, err := q.store.GetNode(ctx, id)
					if err != nil {
						continue
					}
					nodes[i] = node
				}
				results = append(results, &ShortestPathResult{
					Path:  nodes,
					Edges: current.edges,
					Cost:  current.cost,
				})
			}
			continue
		}

		var edges []*Edge
		if len(edgeTypes) == 0 {
			edges, _ = q.store.GetEdgesFrom(ctx, current.nodeID, "")
		} else {
			for _, et := range edgeTypes {
				e, _ := q.store.GetEdgesFrom(ctx, current.nodeID, et)
				edges = append(edges, e...)
			}
		}

		for _, edge := range edges {
			nextID := edge.ToID
			newCost := current.cost + 1

			if existingCost, ok := visited[nextID]; ok && newCost > existingCost {
				continue
			}

			visited[nextID] = newCost
			newPath := append(append([]uuid.UUID{}, current.path...), nextID)
			newEdges := append(append([]*Edge{}, current.edges...), edge)
			queue = append(queue, queueItem{
				nodeID: nextID,
				path:   newPath,
				edges:  newEdges,
				cost:   newCost,
			})
		}
	}

	return results, nil
}

type Subgraph struct {
	Nodes []*Node
	Edges []*Edge
}

func (q *QueryEngine) ExtractSubgraph(ctx context.Context, nodeIDs []uuid.UUID, includeEdges bool) (*Subgraph, error) {
	nodeSet := make(map[uuid.UUID]bool)
	for _, id := range nodeIDs {
		nodeSet[id] = true
	}

	var nodes []*Node
	var edges []*Edge

	for id := range nodeSet {
		node, err := q.store.GetNode(ctx, id)
		if err != nil {
			continue
		}
		nodes = append(nodes, node)

		if includeEdges {
			outEdges, _ := q.store.GetEdgesFrom(ctx, id, "")
			for _, edge := range outEdges {
				if nodeSet[edge.ToID] {
					edges = append(edges, edge)
				}
			}
		}
	}

	return &Subgraph{Nodes: nodes, Edges: edges}, nil
}

func (q *QueryEngine) GetNeighbors(ctx context.Context, nodeID uuid.UUID, direction TraversalDirection, edgeTypes []EdgeType) ([]*Node, []*Edge, error) {
	var allEdges []*Edge
	var err error

	switch direction {
	case TraversalOutbound:
		if len(edgeTypes) == 0 {
			allEdges, err = q.store.GetEdgesFrom(ctx, nodeID, "")
		} else {
			for _, et := range edgeTypes {
				e, _ := q.store.GetEdgesFrom(ctx, nodeID, et)
				allEdges = append(allEdges, e...)
			}
		}
	case TraversalInbound:
		if len(edgeTypes) == 0 {
			allEdges, err = q.store.GetEdgesTo(ctx, nodeID, "")
		} else {
			for _, et := range edgeTypes {
				e, _ := q.store.GetEdgesTo(ctx, nodeID, et)
				allEdges = append(allEdges, e...)
			}
		}
	case TraversalBoth:
		if len(edgeTypes) == 0 {
			outEdges, _ := q.store.GetEdgesFrom(ctx, nodeID, "")
			inEdges, _ := q.store.GetEdgesTo(ctx, nodeID, "")
			allEdges = append(outEdges, inEdges...)
		} else {
			for _, et := range edgeTypes {
				outEdges, _ := q.store.GetEdgesFrom(ctx, nodeID, et)
				inEdges, _ := q.store.GetEdgesTo(ctx, nodeID, et)
				allEdges = append(allEdges, outEdges...)
				allEdges = append(allEdges, inEdges...)
			}
		}
	}

	if err != nil {
		return nil, nil, err
	}

	nodeMap := make(map[uuid.UUID]*Node)
	for _, edge := range allEdges {
		var neighborID uuid.UUID
		if direction == TraversalInbound {
			neighborID = edge.FromID
		} else {
			neighborID = edge.ToID
		}

		if _, ok := nodeMap[neighborID]; !ok {
			node, err := q.store.GetNode(ctx, neighborID)
			if err == nil {
				nodeMap[neighborID] = node
			}
		}
	}

	var neighbors []*Node
	for _, node := range nodeMap {
		neighbors = append(neighbors, node)
	}

	return neighbors, allEdges, nil
}

func (q *QueryEngine) GetConnectedComponents(ctx context.Context, nodeType NodeType) ([][]*Node, error) {
	nodes, err := q.store.ListNodes(ctx, nodeType, 0, 0)
	if err != nil {
		return nil, err
	}

	nodeMap := make(map[uuid.UUID]*Node)
	for _, node := range nodes {
		nodeMap[node.ID] = node
	}

	visited := make(map[uuid.UUID]bool)
	var components [][]*Node

	for _, node := range nodes {
		if visited[node.ID] {
			continue
		}

		var component []*Node
		var dfs func(uuid.UUID)
		dfs = func(id uuid.UUID) {
			if visited[id] {
				return
			}
			visited[id] = true
			if n, ok := nodeMap[id]; ok {
				component = append(component, n)
			}

			edges, _ := q.store.GetEdgesFrom(ctx, id, "")
			for _, edge := range edges {
				if nodeMap[edge.ToID] != nil {
					dfs(edge.ToID)
				}
			}

			edges, _ = q.store.GetEdgesTo(ctx, id, "")
			for _, edge := range edges {
				if nodeMap[edge.FromID] != nil {
					dfs(edge.FromID)
				}
			}
		}

		dfs(node.ID)
		if len(component) > 0 {
			components = append(components, component)
		}
	}

	return components, nil
}

func (q *QueryEngine) CountEdges(ctx context.Context, fromID, toID uuid.UUID, edgeType EdgeType) (int, error) {
	query := `SELECT COUNT(*) FROM edges WHERE from_id = ? AND to_id = ?`
	args := []interface{}{fromID.String(), toID.String()}

	if edgeType != "" {
		query += ` AND type = ?`
		args = append(args, edgeType)
	}

	var count int
	err := q.store.db.QueryRowContext(ctx, query, args...).Scan(&count)
	return count, err
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}