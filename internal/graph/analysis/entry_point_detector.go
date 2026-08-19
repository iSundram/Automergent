package analysis

import (
	"context"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/iSundram/Automergent/internal/graph"
)

type EntryPointDetector struct {
	store *graph.Store
	query *graph.QueryEngine
	mu    sync.RWMutex
}

func NewEntryPointDetector(store *graph.Store, query *graph.QueryEngine) *EntryPointDetector {
	return &EntryPointDetector{
		store: store,
		query: query,
	}
}

func (epd *EntryPointDetector) FindUserEntryPoints(ctx context.Context, featureID uuid.UUID) ([]EntryPoint, error) {
	var entryPoints []EntryPoint

	feature, err := epd.store.GetNode(ctx, featureID)
	if err != nil {
		return nil, err
	}

	tuiCommands := epd.findTUICommands(ctx, feature)
	entryPoints = append(entryPoints, tuiCommands...)

	apiEndpoints := epd.findAPIEndpoints(ctx, feature)
	entryPoints = append(entryPoints, apiEndpoints...)

	cliFlags := epd.findCLIFlags(ctx, feature)
	entryPoints = append(entryPoints, cliFlags...)

	configKeys := epd.findConfigKeys(ctx, feature)
	entryPoints = append(entryPoints, configKeys...)

	return entryPoints, nil
}

func (epd *EntryPointDetector) findTUICommands(ctx context.Context, feature *graph.Node) []EntryPoint {
	var eps []EntryPoint

	edges, err := epd.store.GetEdgesFrom(ctx, feature.ID, graph.EdgeTypeContains)
	if err != nil {
		return eps
	}

	for _, edge := range edges {
		node, err := epd.store.GetNode(ctx, edge.ToID)
		if err != nil {
			continue
		}

		if node.Type == graph.NodeTypeMemory {
			var data map[string]interface{}
			if err := node.UnmarshalData(&data); err != nil {
				continue
			}

			if memType, ok := data["type"].(string); ok && memType == "skill" {
				if content, ok := data["content"].(string); ok {
					if strings.Contains(strings.ToLower(content), "command") ||
						strings.Contains(strings.ToLower(content), "tui") ||
						strings.Contains(strings.ToLower(content), "keybind") {
						eps = append(eps, EntryPoint{
							EntryID:     uuid.New(),
							EntryType:   EntryPointTypeTUICommand,
							Name:        epd.extractCommandName(content),
							Description: content,
							Location:    "tui_keymap",
							FeatureID:   feature.ID,
							Visibility:  VisibilityLevelUserVisible,
							IsPrimary:   epd.isPrimaryCommand(content),
							Metadata: map[string]interface{}{
								"source_node_id": node.ID.String(),
							},
						})
					}
				}
			}
		}
	}

	return eps
}

func (epd *EntryPointDetector) findAPIEndpoints(ctx context.Context, feature *graph.Node) []EntryPoint {
	var eps []EntryPoint

	edges, err := epd.store.GetEdgesFrom(ctx, feature.ID, graph.EdgeTypeContains)
	if err != nil {
		return eps
	}

	for _, edge := range edges {
		node, err := epd.store.GetNode(ctx, edge.ToID)
		if err != nil {
			continue
		}

		if node.Type == graph.NodeTypeFile {
			var data map[string]interface{}
			if err := node.UnmarshalData(&data); err != nil {
				continue
			}

			if path, ok := data["path"].(string); ok {
				if strings.Contains(path, "api") || strings.Contains(path, "handler") ||
					strings.Contains(path, "endpoint") || strings.Contains(path, "route") {
					if language, ok := data["language"].(string); ok && (language == "go" || language == "typescript" || language == "python") {
						eps = append(eps, EntryPoint{
							EntryID:     uuid.New(),
							EntryType:   EntryPointTypeAPIEndpoint,
							Name:        epd.extractEndpointName(path),
							Description: "API endpoint in " + path,
							Location:    path,
							FeatureID:   feature.ID,
							Visibility:  VisibilityLevelUserVisible,
							IsPrimary:   epd.isPrimaryEndpoint(path),
							Metadata: map[string]interface{}{
								"file_node_id": node.ID.String(),
								"language":     language,
							},
						})
					}
				}
			}
		}
	}

	return eps
}

func (epd *EntryPointDetector) findCLIFlags(ctx context.Context, feature *graph.Node) []EntryPoint {
	var eps []EntryPoint

	edges, err := epd.store.GetEdgesFrom(ctx, feature.ID, graph.EdgeTypeContains)
	if err != nil {
		return eps
	}

	for _, edge := range edges {
		node, err := epd.store.GetNode(ctx, edge.ToID)
		if err != nil {
			continue
		}

		if node.Type == graph.NodeTypeFile {
			var data map[string]interface{}
			if err := node.UnmarshalData(&data); err != nil {
				continue
			}

			if path, ok := data["path"].(string); ok {
				if strings.Contains(path, "cmd") || strings.Contains(path, "cli") ||
					strings.Contains(path, "flag") || strings.Contains(path, "argument") {
					if language, ok := data["language"].(string); ok && language == "go" {
						eps = append(eps, EntryPoint{
							EntryID:     uuid.New(),
							EntryType:   EntryPointTypeCLIFlag,
							Name:        epd.extractFlagName(path),
							Description: "CLI flag in " + path,
							Location:    path,
							FeatureID:   feature.ID,
							Visibility:  VisibilityLevelUserVisible,
							IsPrimary:   false,
							Metadata: map[string]interface{}{
								"file_node_id": node.ID.String(),
							},
						})
					}
				}
			}
		}
	}

	return eps
}

func (epd *EntryPointDetector) findConfigKeys(ctx context.Context, feature *graph.Node) []EntryPoint {
	var eps []EntryPoint

	edges, err := epd.store.GetEdgesFrom(ctx, feature.ID, graph.EdgeTypeContains)
	if err != nil {
		return eps
	}

	for _, edge := range edges {
		node, err := epd.store.GetNode(ctx, edge.ToID)
		if err != nil {
			continue
		}

		if node.Type == graph.NodeTypeFile {
			var data map[string]interface{}
			if err := node.UnmarshalData(&data); err != nil {
				continue
			}

			if path, ok := data["path"].(string); ok {
				if strings.Contains(path, "config") || strings.Contains(path, "settings") ||
					strings.Contains(path, ".yaml") || strings.Contains(path, ".yml") ||
					strings.Contains(path, ".json") || strings.Contains(path, ".toml") {
					eps = append(eps, EntryPoint{
						EntryID:     uuid.New(),
						EntryType:   EntryPointTypeConfigKey,
						Name:        epd.extractConfigKey(path),
						Description: "Configuration in " + path,
						Location:    path,
						FeatureID:   feature.ID,
						Visibility:  VisibilityLevelDeveloperVisible,
						IsPrimary:   false,
						Metadata: map[string]interface{}{
							"file_node_id": node.ID.String(),
						},
					})
				}
			}
		}
	}

	return eps
}

func (epd *EntryPointDetector) FindIntegrationPoints(ctx context.Context, featureID uuid.UUID) ([]IntegrationPoint, error) {
	var points []IntegrationPoint

	feature, err := epd.store.GetNode(ctx, featureID)
	if err != nil {
		return nil, err
	}

	events := epd.findEvents(ctx, feature)
	points = append(points, events...)

	hooks := epd.findHooks(ctx, feature)
	points = append(points, hooks...)

	interfaces := epd.findInterfaces(ctx, feature)
	points = append(points, interfaces...)

	callbacks := epd.findCallbacks(ctx, feature)
	points = append(points, callbacks...)

	return points, nil
}

func (epd *EntryPointDetector) findEvents(ctx context.Context, feature *graph.Node) []IntegrationPoint {
	var points []IntegrationPoint

	edges, err := epd.store.GetEdgesFrom(ctx, feature.ID, graph.EdgeTypeContains)
	if err != nil {
		return points
	}

	for _, edge := range edges {
		node, err := epd.store.GetNode(ctx, edge.ToID)
		if err != nil {
			continue
		}

		if node.Type == graph.NodeTypeEvent {
			var data map[string]interface{}
			if err := node.UnmarshalData(&data); err != nil {
				continue
			}

			eventType := ""
			if t, ok := data["type"].(string); ok {
				eventType = t
			}

			direction := IntegrationDirectionOutbound
			if source, ok := data["source"].(string); ok && source != feature.ID.String() {
				direction = IntegrationDirectionInbound
			}

			points = append(points, IntegrationPoint{
				PointID:       uuid.New(),
				PointType:     IntegrationPointTypeEvent,
				Name:          eventType,
				Description:   "Event: " + eventType,
				SourceFeature: feature.ID,
				TargetFeature: uuid.Nil,
				Interface:     eventType,
				Direction:     direction,
				Metadata: map[string]interface{}{
					"event_node_id": node.ID.String(),
					"payload":       data["payload"],
				},
			})
		}
	}

	edges, err = epd.store.GetEdgesTo(ctx, feature.ID, graph.EdgeTypeTriggers)
	if err != nil {
		return points
	}

	for _, edge := range edges {
		node, err := epd.store.GetNode(ctx, edge.FromID)
		if err != nil {
			continue
		}

		if node.Type == graph.NodeTypeEvent {
			var data map[string]interface{}
			if err := node.UnmarshalData(&data); err != nil {
				continue
			}

			eventType := ""
			if t, ok := data["type"].(string); ok {
				eventType = t
			}

			points = append(points, IntegrationPoint{
				PointID:       uuid.New(),
				PointType:     IntegrationPointTypeEvent,
				Name:          eventType,
				Description:   "Triggered by event: " + eventType,
				SourceFeature: node.ID,
				TargetFeature: feature.ID,
				Interface:     eventType,
				Direction:     IntegrationDirectionInbound,
				Metadata: map[string]interface{}{
					"event_node_id": node.ID.String(),
				},
			})
		}
	}

	return points
}

func (epd *EntryPointDetector) findHooks(ctx context.Context, feature *graph.Node) []IntegrationPoint {
	var points []IntegrationPoint

	edges, err := epd.store.GetEdgesFrom(ctx, feature.ID, graph.EdgeTypeContains)
	if err != nil {
		return points
	}

	for _, edge := range edges {
		node, err := epd.store.GetNode(ctx, edge.ToID)
		if err != nil {
			continue
		}

		if node.Type == graph.NodeTypeMemory {
			var data map[string]interface{}
			if err := node.UnmarshalData(&data); err != nil {
				continue
			}

			if memType, ok := data["type"].(string); ok && memType == "hook" {
				if name, ok := data["name"].(string); ok {
					points = append(points, IntegrationPoint{
						PointID:       uuid.New(),
						PointType:     IntegrationPointTypeHook,
						Name:          name,
						Description:   "Hook: " + name,
						SourceFeature: feature.ID,
						TargetFeature: uuid.Nil,
						Interface:     name,
						Direction:     IntegrationDirectionBidirectional,
						Metadata: map[string]interface{}{
							"hook_node_id": node.ID.String(),
							"config":       data["config"],
						},
					})
				}
			}
		}
	}

	return points
}

func (epd *EntryPointDetector) findInterfaces(ctx context.Context, feature *graph.Node) []IntegrationPoint {
	var points []IntegrationPoint

	edges, err := epd.store.GetEdgesFrom(ctx, feature.ID, graph.EdgeTypeContains)
	if err != nil {
		return points
	}

	for _, edge := range edges {
		node, err := epd.store.GetNode(ctx, edge.ToID)
		if err != nil {
			continue
		}

		if node.Type == graph.NodeTypeFile {
			var data map[string]interface{}
			if err := node.UnmarshalData(&data); err != nil {
				continue
			}

			if path, ok := data["path"].(string); ok {
				if strings.Contains(path, "interface") || strings.Contains(path, "contract") {
					if language, ok := data["language"].(string); ok && (language == "go" || language == "typescript") {
						points = append(points, IntegrationPoint{
							PointID:       uuid.New(),
							PointType:     IntegrationPointTypeInterface,
							Name:          epd.extractInterfaceName(path),
							Description:   "Interface in " + path,
							SourceFeature: feature.ID,
							TargetFeature: uuid.Nil,
							Interface:     path,
							Direction:     IntegrationDirectionBidirectional,
							Metadata: map[string]interface{}{
								"file_node_id": node.ID.String(),
								"language":     language,
							},
						})
					}
				}
			}
		}
	}

	return points
}

func (epd *EntryPointDetector) findCallbacks(ctx context.Context, feature *graph.Node) []IntegrationPoint {
	var points []IntegrationPoint

	edges, err := epd.store.GetEdgesFrom(ctx, feature.ID, graph.EdgeTypeContains)
	if err != nil {
		return points
	}

	for _, edge := range edges {
		node, err := epd.store.GetNode(ctx, edge.ToID)
		if err != nil {
			continue
		}

		if node.Type == graph.NodeTypeMemory {
			var data map[string]interface{}
			if err := node.UnmarshalData(&data); err != nil {
				continue
			}

			if memType, ok := data["type"].(string); ok && memType == "callback" {
				if name, ok := data["name"].(string); ok {
					points = append(points, IntegrationPoint{
						PointID:       uuid.New(),
						PointType:     IntegrationPointTypeCallback,
						Name:          name,
						Description:   "Callback: " + name,
						SourceFeature: feature.ID,
						TargetFeature: uuid.Nil,
						Interface:     name,
						Direction:     IntegrationDirectionInbound,
						Metadata: map[string]interface{}{
							"callback_node_id": node.ID.String(),
							"signature":        data["signature"],
						},
					})
				}
			}
		}
	}

	return points
}

func (epd *EntryPointDetector) DetectProductVisibility(ctx context.Context, featureID uuid.UUID) (map[EntryPointType]VisibilityLevel, error) {
	visibility := make(map[EntryPointType]VisibilityLevel)

	entryPoints, err := epd.FindUserEntryPoints(ctx, featureID)
	if err != nil {
		return visibility, err
	}

	for _, ep := range entryPoints {
		if existing, ok := visibility[ep.EntryType]; !ok || ep.Visibility < existing {
			visibility[ep.EntryType] = ep.Visibility
		}
	}

	integrationPoints, err := epd.FindIntegrationPoints(ctx, featureID)
	if err != nil {
		return visibility, err
	}

	for _, ip := range integrationPoints {
		epType := epd.integrationPointTypeToEntryPointType(ip.PointType)
		if _, ok := visibility[epType]; !ok || ip.Direction == IntegrationDirectionInbound {
			visibility[epType] = VisibilityLevelDeveloperVisible
		}
	}

	return visibility, nil
}

func (epd *EntryPointDetector) integrationPointTypeToEntryPointType(ipt IntegrationPointType) EntryPointType {
	switch ipt {
	case IntegrationPointTypeEvent:
		return EntryPointTypeEvent
	case IntegrationPointTypeHook:
		return EntryPointTypeHook
	case IntegrationPointTypeInterface:
		return EntryPointTypeInterface
	case IntegrationPointTypeCallback:
		return EntryPointTypeCallback
	default:
		return EntryPointTypeEvent
	}
}

func (epd *EntryPointDetector) extractCommandName(content string) string {
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

func (epd *EntryPointDetector) extractEndpointName(path string) string {
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}

func (epd *EntryPointDetector) extractFlagName(path string) string {
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}

func (epd *EntryPointDetector) extractConfigKey(path string) string {
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}

func (epd *EntryPointDetector) extractInterfaceName(path string) string {
	parts := strings.Split(path, "/")
	name := parts[len(parts)-1]
	name = strings.TrimSuffix(name, ".go")
	name = strings.TrimSuffix(name, ".ts")
	return name
}

func (epd *EntryPointDetector) isPrimaryCommand(content string) bool {
	return strings.Contains(strings.ToLower(content), "primary") ||
		strings.Contains(strings.ToLower(content), "main") ||
		strings.Contains(strings.ToLower(content), "default")
}

func (epd *EntryPointDetector) isPrimaryEndpoint(path string) bool {
	return strings.Contains(path, "main") || strings.Contains(path, "index") || strings.Contains(path, "root")
}