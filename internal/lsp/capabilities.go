package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ServerCapabilities tracks what an LSP server supports.
type ServerCapabilities struct {
	DefinitionProvider         bool `json:"definitionProvider,omitempty"`
	TypeDefinitionProvider     bool `json:"typeDefinitionProvider,omitempty"`
	ImplementationProvider     bool `json:"implementationProvider,omitempty"`
	ReferencesProvider         bool `json:"referencesProvider,omitempty"`
	DocumentSymbolProvider     bool `json:"documentSymbolProvider,omitempty"`
	WorkspaceSymbolProvider    bool `json:"workspaceSymbolProvider,omitempty"`
	HoverProvider              bool `json:"hoverProvider,omitempty"`
	CompletionProvider         bool `json:"completionProvider,omitempty"`
	SignatureHelpProvider      bool `json:"signatureHelpProvider,omitempty"`
	RenameProvider             bool `json:"renameProvider,omitempty"`
	CodeActionProvider         bool `json:"codeActionProvider,omitempty"`
	DocumentFormattingProvider bool `json:"documentFormattingProvider,omitempty"`
	CallHierarchyProvider      bool `json:"callHierarchyProvider,omitempty"`
	TypeHierarchyProvider      bool `json:"typeHierarchyProvider,omitempty"`
	SemanticTokensProvider     bool `json:"semanticTokensProvider,omitempty"`
	FoldingRangeProvider       bool `json:"foldingRangeProvider,omitempty"`
	SelectionRangeProvider     bool `json:"selectionRangeProvider,omitempty"`
	DiagnosticProvider         bool `json:"diagnosticProvider,omitempty"`
}

// InitializeParams for LSP initialize request.
type InitializeParams struct {
	ProcessID    int                `json:"processId"`
	RootURI      string             `json:"rootUri"`
	Capabilities ClientCapabilities `json:"capabilities"`
	Trace        string             `json:"trace,omitempty"`
}

// ClientCapabilities we announce to the server.
type ClientCapabilities struct {
	TextDocument TextDocumentClientCapabilities `json:"textDocument"`
	Workspace    WorkspaceClientCapabilities    `json:"workspace"`
}

// TextDocumentClientCapabilities for text document operations.
type TextDocumentClientCapabilities struct {
	Synchronization    *SyncCapability           `json:"synchronization,omitempty"`
	Completion         *CompletionCapability     `json:"completion,omitempty"`
	Hover              *HoverCapability          `json:"hover,omitempty"`
	SignatureHelp      *SignatureCapability      `json:"signatureHelp,omitempty"`
	Definition         *DefinitionCapability     `json:"definition,omitempty"`
	TypeDefinition     *TypeDefCapability        `json:"typeDefinition,omitempty"`
	Implementation     *ImplCapability           `json:"implementation,omitempty"`
	References         *RefCapability            `json:"references,omitempty"`
	DocumentSymbol     *DocSymbolCapability      `json:"documentSymbol,omitempty"`
	CodeAction         *CodeActionCapability     `json:"codeAction,omitempty"`
	Rename             *RenameCapability         `json:"rename,omitempty"`
	PublishDiagnostics *DiagCapability           `json:"publishDiagnostics,omitempty"`
	CallHierarchy      *CallHierarchyCapability  `json:"callHierarchy,omitempty"`
	SemanticTokens     *SemanticTokensCapability `json:"semanticTokens,omitempty"`
}

// WorkspaceClientCapabilities for workspace operations.
type WorkspaceClientCapabilities struct {
	WorkspaceFolders bool                       `json:"workspaceFolders,omitempty"`
	Symbol           *WorkspaceSymbolCapability `json:"symbol,omitempty"`
	ApplyEdit        bool                       `json:"applyEdit,omitempty"`
}

// Various capability structs.
type SyncCapability struct {
	DynamicRegistration bool `json:"dynamicRegistration,omitempty"`
	WillSave            bool `json:"willSave,omitempty"`
	WillSaveWaitUntil   bool `json:"willSaveWaitUntil,omitempty"`
	DidSave             bool `json:"didSave,omitempty"`
}

type CompletionCapability struct {
	DynamicRegistration bool `json:"dynamicRegistration,omitempty"`
	CompletionItem      struct {
		SnippetSupport      bool     `json:"snippetSupport,omitempty"`
		DocumentationFormat []string `json:"documentationFormat,omitempty"`
	} `json:"completionItem,omitempty"`
}

type HoverCapability struct {
	DynamicRegistration bool     `json:"dynamicRegistration,omitempty"`
	ContentFormat       []string `json:"contentFormat,omitempty"`
}

type SignatureCapability struct {
	DynamicRegistration  bool `json:"dynamicRegistration,omitempty"`
	SignatureInformation struct {
		DocumentationFormat []string `json:"documentationFormat,omitempty"`
	} `json:"signatureInformation,omitempty"`
}

type DefinitionCapability struct {
	DynamicRegistration bool `json:"dynamicRegistration,omitempty"`
	LinkSupport         bool `json:"linkSupport,omitempty"`
}

type TypeDefCapability struct {
	DynamicRegistration bool `json:"dynamicRegistration,omitempty"`
	LinkSupport         bool `json:"linkSupport,omitempty"`
}

type ImplCapability struct {
	DynamicRegistration bool `json:"dynamicRegistration,omitempty"`
	LinkSupport         bool `json:"linkSupport,omitempty"`
}

type RefCapability struct {
	DynamicRegistration bool `json:"dynamicRegistration,omitempty"`
}

type DocSymbolCapability struct {
	DynamicRegistration               bool `json:"dynamicRegistration,omitempty"`
	HierarchicalDocumentSymbolSupport bool `json:"hierarchicalDocumentSymbolSupport,omitempty"`
}

type CodeActionCapability struct {
	DynamicRegistration      bool `json:"dynamicRegistration,omitempty"`
	CodeActionLiteralSupport struct {
		CodeActionKind struct {
			ValueSet []string `json:"valueSet,omitempty"`
		} `json:"codeActionKind,omitempty"`
	} `json:"codeActionLiteralSupport,omitempty"`
	IsPreferredSupport bool `json:"isPreferredSupport,omitempty"`
	ResolveSupport     struct {
		Properties []string `json:"properties,omitempty"`
	} `json:"resolveSupport,omitempty"`
}

type RenameCapability struct {
	DynamicRegistration bool `json:"dynamicRegistration,omitempty"`
	PrepareSupport      bool `json:"prepareSupport,omitempty"`
}

type DiagCapability struct {
	RelatedInformation bool `json:"relatedInformation,omitempty"`
	TagSupport         struct {
		ValueSet []int `json:"valueSet,omitempty"`
	} `json:"tagSupport,omitempty"`
	CodeDescriptionSupport bool `json:"codeDescriptionSupport,omitempty"`
	DataSupport            bool `json:"dataSupport,omitempty"`
}

type CallHierarchyCapability struct {
	DynamicRegistration bool `json:"dynamicRegistration,omitempty"`
}

type SemanticTokensCapability struct {
	DynamicRegistration bool `json:"dynamicRegistration,omitempty"`
	Requests            struct {
		Full  bool `json:"full,omitempty"`
		Range bool `json:"range,omitempty"`
	} `json:"requests,omitempty"`
	TokenTypes     []string `json:"tokenTypes,omitempty"`
	TokenModifiers []string `json:"tokenModifiers,omitempty"`
}

type WorkspaceSymbolCapability struct {
	DynamicRegistration bool `json:"dynamicRegistration,omitempty"`
}

// InitializeResult from the server.
type InitializeResult struct {
	Capabilities json.RawMessage `json:"capabilities"`
	ServerInfo   *ServerInfo     `json:"serverInfo,omitempty"`
}

// ServerInfo about the LSP server.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// IntelligentClient wraps a basic Client with high-level operations.
type IntelligentClient struct {
	*Client
	mu                   sync.RWMutex
	rootURI              string
	capabilities         ServerCapabilities
	initialized          bool
	openFiles            map[string]int // URI -> version
	serverInfo           *ServerInfo
	requestTimeout       time.Duration
	semanticLegend       *SemanticTokensLegend
	notificationHandlers map[string]func(json.RawMessage)
}

// NewIntelligentClient creates a new intelligent LSP client.
func NewIntelligentClient(client *Client) *IntelligentClient {
	return &IntelligentClient{
		Client:               client,
		openFiles:            make(map[string]int),
		requestTimeout:       30 * time.Second,
		notificationHandlers: make(map[string]func(json.RawMessage)),
	}
}

// SetRequestTimeout sets the timeout for LSP requests.
func (ic *IntelligentClient) SetRequestTimeout(d time.Duration) {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	ic.requestTimeout = d
}

// OnNotification registers a handler for server notifications.
func (ic *IntelligentClient) OnNotification(method string, handler func(json.RawMessage)) {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	ic.notificationHandlers[method] = handler
}

// Initialize performs the LSP handshake with the server.
func (ic *IntelligentClient) Initialize(ctx context.Context, rootPath string) error {
	ic.mu.Lock()
	defer ic.mu.Unlock()

	if ic.initialized {
		return nil
	}

	ic.rootURI = PathToURI(rootPath)

	params := InitializeParams{
		ProcessID: os.Getpid(),
		RootURI:   ic.rootURI,
		Capabilities: ClientCapabilities{
			TextDocument: TextDocumentClientCapabilities{
				Synchronization: &SyncCapability{
					DynamicRegistration: true,
					WillSave:            true,
					DidSave:             true,
				},
				Completion: &CompletionCapability{
					DynamicRegistration: true,
				},
				Hover: &HoverCapability{
					DynamicRegistration: true,
					ContentFormat:       []string{"markdown", "plaintext"},
				},
				SignatureHelp: &SignatureCapability{
					DynamicRegistration: true,
				},
				Definition: &DefinitionCapability{
					DynamicRegistration: true,
					LinkSupport:         true,
				},
				TypeDefinition: &TypeDefCapability{
					DynamicRegistration: true,
					LinkSupport:         true,
				},
				Implementation: &ImplCapability{
					DynamicRegistration: true,
					LinkSupport:         true,
				},
				References: &RefCapability{
					DynamicRegistration: true,
				},
				DocumentSymbol: &DocSymbolCapability{
					DynamicRegistration:               true,
					HierarchicalDocumentSymbolSupport: true,
				},
				CodeAction: &CodeActionCapability{
					DynamicRegistration: true,
					IsPreferredSupport:  true,
				},
				Rename: &RenameCapability{
					DynamicRegistration: true,
					PrepareSupport:      true,
				},
				PublishDiagnostics: &DiagCapability{
					RelatedInformation:     true,
					CodeDescriptionSupport: true,
					DataSupport:            true,
				},
				CallHierarchy: &CallHierarchyCapability{
					DynamicRegistration: true,
				},
				SemanticTokens: &SemanticTokensCapability{
					DynamicRegistration: true,
				},
			},
			Workspace: WorkspaceClientCapabilities{
				WorkspaceFolders: true,
				ApplyEdit:        true,
				Symbol: &WorkspaceSymbolCapability{
					DynamicRegistration: true,
				},
			},
		},
		Trace: "off",
	}

	resp, err := ic.Client.Call(ctx, "initialize", params)
	if err != nil {
		return fmt.Errorf("initialize: %w", err)
	}

	var result InitializeResult
	if err := json.Unmarshal(resp, &result); err != nil {
		return fmt.Errorf("parse initialize result: %w", err)
	}

	ic.serverInfo = result.ServerInfo
	ic.parseCapabilities(result.Capabilities)

	// Send initialized notification
	if err := ic.Client.Notify("initialized", struct{}{}); err != nil {
		return fmt.Errorf("send initialized: %w", err)
	}

	ic.initialized = true
	return nil
}

// parseCapabilities extracts server capabilities.
func (ic *IntelligentClient) parseCapabilities(raw json.RawMessage) {
	var caps struct {
		DefinitionProvider         interface{} `json:"definitionProvider"`
		TypeDefinitionProvider     interface{} `json:"typeDefinitionProvider"`
		ImplementationProvider     interface{} `json:"implementationProvider"`
		ReferencesProvider         interface{} `json:"referencesProvider"`
		DocumentSymbolProvider     interface{} `json:"documentSymbolProvider"`
		WorkspaceSymbolProvider    interface{} `json:"workspaceSymbolProvider"`
		HoverProvider              interface{} `json:"hoverProvider"`
		CompletionProvider         interface{} `json:"completionProvider"`
		SignatureHelpProvider      interface{} `json:"signatureHelpProvider"`
		RenameProvider             interface{} `json:"renameProvider"`
		CodeActionProvider         interface{} `json:"codeActionProvider"`
		DocumentFormattingProvider interface{} `json:"documentFormattingProvider"`
		CallHierarchyProvider      interface{} `json:"callHierarchyProvider"`
		TypeHierarchyProvider      interface{} `json:"typeHierarchyProvider"`
		SemanticTokensProvider     interface{} `json:"semanticTokensProvider"`
		FoldingRangeProvider       interface{} `json:"foldingRangeProvider"`
		SelectionRangeProvider     interface{} `json:"selectionRangeProvider"`
	}

	if err := json.Unmarshal(raw, &caps); err != nil {
		return
	}

	ic.capabilities = ServerCapabilities{
		DefinitionProvider:         caps.DefinitionProvider != nil && caps.DefinitionProvider != false,
		TypeDefinitionProvider:     caps.TypeDefinitionProvider != nil && caps.TypeDefinitionProvider != false,
		ImplementationProvider:     caps.ImplementationProvider != nil && caps.ImplementationProvider != false,
		ReferencesProvider:         caps.ReferencesProvider != nil && caps.ReferencesProvider != false,
		DocumentSymbolProvider:     caps.DocumentSymbolProvider != nil && caps.DocumentSymbolProvider != false,
		WorkspaceSymbolProvider:    caps.WorkspaceSymbolProvider != nil && caps.WorkspaceSymbolProvider != false,
		HoverProvider:              caps.HoverProvider != nil && caps.HoverProvider != false,
		CompletionProvider:         caps.CompletionProvider != nil,
		SignatureHelpProvider:      caps.SignatureHelpProvider != nil,
		RenameProvider:             caps.RenameProvider != nil && caps.RenameProvider != false,
		CodeActionProvider:         caps.CodeActionProvider != nil && caps.CodeActionProvider != false,
		DocumentFormattingProvider: caps.DocumentFormattingProvider != nil && caps.DocumentFormattingProvider != false,
		CallHierarchyProvider:      caps.CallHierarchyProvider != nil && caps.CallHierarchyProvider != false,
		TypeHierarchyProvider:      caps.TypeHierarchyProvider != nil && caps.TypeHierarchyProvider != false,
		SemanticTokensProvider:     caps.SemanticTokensProvider != nil,
		FoldingRangeProvider:       caps.FoldingRangeProvider != nil && caps.FoldingRangeProvider != false,
		SelectionRangeProvider:     caps.SelectionRangeProvider != nil && caps.SelectionRangeProvider != false,
	}
}

// Capabilities returns the server's capabilities.
func (ic *IntelligentClient) Capabilities() ServerCapabilities {
	ic.mu.RLock()
	defer ic.mu.RUnlock()
	return ic.capabilities
}

// ServerName returns the server's name if available.
func (ic *IntelligentClient) ServerName() string {
	ic.mu.RLock()
	defer ic.mu.RUnlock()
	if ic.serverInfo != nil {
		return ic.serverInfo.Name
	}
	return ""
}

// Shutdown gracefully shuts down the LSP connection.
func (ic *IntelligentClient) Shutdown(ctx context.Context) error {
	ic.mu.Lock()
	defer ic.mu.Unlock()

	if !ic.initialized {
		return nil
	}

	// Send shutdown request
	if _, err := ic.Client.Call(ctx, "shutdown", nil); err != nil {
		// Log but continue
	}

	// Send exit notification
	ic.Client.Notify("exit", nil)

	ic.initialized = false
	return ic.Client.Close()
}

// PathToURI converts a file path to a file:// URI.
func PathToURI(path string) string {
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}
	// On Windows, paths need different handling
	if strings.HasPrefix(absPath, "/") {
		return "file://" + absPath
	}
	return "file:///" + strings.ReplaceAll(absPath, "\\", "/")
}

// URIToPath converts a file:// URI back to a file path.
func URIToPath(uri string) string {
	if !strings.HasPrefix(uri, "file://") {
		return uri
	}

	u, err := url.Parse(uri)
	if err != nil {
		return strings.TrimPrefix(uri, "file://")
	}

	path := u.Path
	// Handle Windows paths
	if len(path) > 2 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}

	return path
}

// LanguageIDFromPath determines language ID from file extension.
func LanguageIDFromPath(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return "go"
	case ".ts":
		return "typescript"
	case ".tsx":
		return "typescriptreact"
	case ".js":
		return "javascript"
	case ".jsx":
		return "javascriptreact"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".c":
		return "c"
	case ".cpp", ".cc", ".cxx":
		return "cpp"
	case ".h", ".hpp":
		return "c"
	case ".rb":
		return "ruby"
	case ".lua":
		return "lua"
	case ".sh", ".bash":
		return "shellscript"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".md":
		return "markdown"
	case ".html", ".htm":
		return "html"
	case ".css":
		return "css"
	case ".scss":
		return "scss"
	case ".sql":
		return "sql"
	default:
		return "plaintext"
	}
}
