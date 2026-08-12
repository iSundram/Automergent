package lsp

import "encoding/json"

// TextDocumentIdentifier identifies a text document.
type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

// TextDocumentPositionParams for position-based queries.
type TextDocumentPositionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

// VersionedTextDocumentIdentifier for documents with versions.
type VersionedTextDocumentIdentifier struct {
	TextDocumentIdentifier
	Version int `json:"version"`
}

// TextDocumentItem for opening documents.
type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

// Location represents a location in a document.
type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

// LocationLink for enhanced definition results.
type LocationLink struct {
	OriginSelectionRange *Range `json:"originSelectionRange,omitempty"`
	TargetURI            string `json:"targetUri"`
	TargetRange          Range  `json:"targetRange"`
	TargetSelectionRange Range  `json:"targetSelectionRange"`
}

// SymbolKind represents the kind of a symbol.
type SymbolKind int

const (
	SymbolKindFile          SymbolKind = 1
	SymbolKindModule        SymbolKind = 2
	SymbolKindNamespace     SymbolKind = 3
	SymbolKindPackage       SymbolKind = 4
	SymbolKindClass         SymbolKind = 5
	SymbolKindMethod        SymbolKind = 6
	SymbolKindProperty      SymbolKind = 7
	SymbolKindField         SymbolKind = 8
	SymbolKindConstructor   SymbolKind = 9
	SymbolKindEnum          SymbolKind = 10
	SymbolKindInterface     SymbolKind = 11
	SymbolKindFunction      SymbolKind = 12
	SymbolKindVariable      SymbolKind = 13
	SymbolKindConstant      SymbolKind = 14
	SymbolKindString        SymbolKind = 15
	SymbolKindNumber        SymbolKind = 16
	SymbolKindBoolean       SymbolKind = 17
	SymbolKindArray         SymbolKind = 18
	SymbolKindObject        SymbolKind = 19
	SymbolKindKey           SymbolKind = 20
	SymbolKindNull          SymbolKind = 21
	SymbolKindEnumMember    SymbolKind = 22
	SymbolKindStruct        SymbolKind = 23
	SymbolKindEvent         SymbolKind = 24
	SymbolKindOperator      SymbolKind = 25
	SymbolKindTypeParameter SymbolKind = 26
)

func (k SymbolKind) String() string {
	names := map[SymbolKind]string{
		SymbolKindFile:          "File",
		SymbolKindModule:        "Module",
		SymbolKindNamespace:     "Namespace",
		SymbolKindPackage:       "Package",
		SymbolKindClass:         "Class",
		SymbolKindMethod:        "Method",
		SymbolKindProperty:      "Property",
		SymbolKindField:         "Field",
		SymbolKindConstructor:   "Constructor",
		SymbolKindEnum:          "Enum",
		SymbolKindInterface:     "Interface",
		SymbolKindFunction:      "Function",
		SymbolKindVariable:      "Variable",
		SymbolKindConstant:      "Constant",
		SymbolKindString:        "String",
		SymbolKindNumber:        "Number",
		SymbolKindBoolean:       "Boolean",
		SymbolKindArray:         "Array",
		SymbolKindObject:        "Object",
		SymbolKindKey:           "Key",
		SymbolKindNull:          "Null",
		SymbolKindEnumMember:    "EnumMember",
		SymbolKindStruct:        "Struct",
		SymbolKindEvent:         "Event",
		SymbolKindOperator:      "Operator",
		SymbolKindTypeParameter: "TypeParameter",
	}
	if name, ok := names[k]; ok {
		return name
	}
	return "Unknown"
}

// DocumentSymbol represents a symbol in a document.
type DocumentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail,omitempty"`
	Kind           SymbolKind       `json:"kind"`
	Tags           []int            `json:"tags,omitempty"`
	Deprecated     bool             `json:"deprecated,omitempty"`
	Range          Range            `json:"range"`
	SelectionRange Range            `json:"selectionRange"`
	Children       []DocumentSymbol `json:"children,omitempty"`
}

// SymbolInformation for workspace symbols.
type SymbolInformation struct {
	Name          string     `json:"name"`
	Kind          SymbolKind `json:"kind"`
	Tags          []int      `json:"tags,omitempty"`
	Deprecated    bool       `json:"deprecated,omitempty"`
	Location      Location   `json:"location"`
	ContainerName string     `json:"containerName,omitempty"`
}

// WorkspaceSymbol for workspace symbol search results.
type WorkspaceSymbol struct {
	Name          string     `json:"name"`
	Kind          SymbolKind `json:"kind"`
	Tags          []int      `json:"tags,omitempty"`
	ContainerName string     `json:"containerName,omitempty"`
	Location      Location   `json:"location"`
}

// Hover represents hover information.
type Hover struct {
	Contents MarkupContent `json:"contents"`
	Range    *Range        `json:"range,omitempty"`
}

// MarkupContent for rich hover text.
type MarkupContent struct {
	Kind  string `json:"kind"` // "plaintext" or "markdown"
	Value string `json:"value"`
}

// ReferenceContext for find references.
type ReferenceContext struct {
	IncludeDeclaration bool `json:"includeDeclaration"`
}

// ReferenceParams for textDocument/references.
type ReferenceParams struct {
	TextDocumentPositionParams
	Context ReferenceContext `json:"context"`
}

// CallHierarchyItem represents an item in call hierarchy.
type CallHierarchyItem struct {
	Name           string      `json:"name"`
	Kind           SymbolKind  `json:"kind"`
	Tags           []int       `json:"tags,omitempty"`
	Detail         string      `json:"detail,omitempty"`
	URI            string      `json:"uri"`
	Range          Range       `json:"range"`
	SelectionRange Range       `json:"selectionRange"`
	Data           interface{} `json:"data,omitempty"`
}

// CallHierarchyIncomingCall represents an incoming call.
type CallHierarchyIncomingCall struct {
	From       CallHierarchyItem `json:"from"`
	FromRanges []Range           `json:"fromRanges"`
}

// CallHierarchyOutgoingCall represents an outgoing call.
type CallHierarchyOutgoingCall struct {
	To         CallHierarchyItem `json:"to"`
	FromRanges []Range           `json:"fromRanges"`
}

// TypeHierarchyItem for type hierarchy.
type TypeHierarchyItem struct {
	Name           string      `json:"name"`
	Kind           SymbolKind  `json:"kind"`
	Tags           []int       `json:"tags,omitempty"`
	Detail         string      `json:"detail,omitempty"`
	URI            string      `json:"uri"`
	Range          Range       `json:"range"`
	SelectionRange Range       `json:"selectionRange"`
	Data           interface{} `json:"data,omitempty"`
}

// WorkspaceEdit for refactoring operations.
type WorkspaceEdit struct {
	Changes         map[string][]TextEdit `json:"changes,omitempty"`
	DocumentChanges []DocumentChange      `json:"documentChanges,omitempty"`
}

// TextEdit for individual edits.
type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

// DocumentChange wrapper for document changes.
type DocumentChange struct {
	TextDocument VersionedTextDocumentIdentifier `json:"textDocument"`
	Edits        []TextEdit                      `json:"edits"`
}

// RenameParams for textDocument/rename.
type RenameParams struct {
	TextDocumentPositionParams
	NewName string `json:"newName"`
}

// PrepareRenameResult for rename preparation.
type PrepareRenameResult struct {
	Range       Range  `json:"range"`
	Placeholder string `json:"placeholder"`
}

// CodeAction represents a code action.
type CodeAction struct {
	Title       string       `json:"title"`
	Kind        string       `json:"kind,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
	IsPreferred bool         `json:"isPreferred,omitempty"`
	Disabled    *struct {
		Reason string `json:"reason"`
	} `json:"disabled,omitempty"`
	Edit    *WorkspaceEdit `json:"edit,omitempty"`
	Command *Command       `json:"command,omitempty"`
	Data    interface{}    `json:"data,omitempty"`
}

// CodeActionParams for textDocument/codeAction.
type CodeActionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Range        Range                  `json:"range"`
	Context      CodeActionContext      `json:"context"`
}

// CodeActionContext provides context for code actions.
type CodeActionContext struct {
	Diagnostics []Diagnostic `json:"diagnostics"`
	Only        []string     `json:"only,omitempty"`
}

// Command represents a command.
type Command struct {
	Title     string        `json:"title"`
	Command   string        `json:"command"`
	Arguments []interface{} `json:"arguments,omitempty"`
}

// CompletionItem represents a completion item.
type CompletionItem struct {
	Label               string           `json:"label"`
	LabelDetails        *CompletionLabel `json:"labelDetails,omitempty"`
	Kind                CompletionKind   `json:"kind,omitempty"`
	Tags                []int            `json:"tags,omitempty"`
	Detail              string           `json:"detail,omitempty"`
	Documentation       json.RawMessage  `json:"documentation,omitempty"`
	Deprecated          bool             `json:"deprecated,omitempty"`
	Preselect           bool             `json:"preselect,omitempty"`
	SortText            string           `json:"sortText,omitempty"`
	FilterText          string           `json:"filterText,omitempty"`
	InsertText          string           `json:"insertText,omitempty"`
	InsertTextFormat    int              `json:"insertTextFormat,omitempty"`
	InsertTextMode      int              `json:"insertTextMode,omitempty"`
	TextEdit            *TextEdit        `json:"textEdit,omitempty"`
	AdditionalTextEdits []TextEdit       `json:"additionalTextEdits,omitempty"`
	CommitCharacters    []string         `json:"commitCharacters,omitempty"`
	Command             *Command         `json:"command,omitempty"`
	Data                interface{}      `json:"data,omitempty"`
}

// CompletionLabel for additional label details.
type CompletionLabel struct {
	Detail      string `json:"detail,omitempty"`
	Description string `json:"description,omitempty"`
}

// CompletionKind represents the kind of completion.
type CompletionKind int

const (
	CompletionKindText          CompletionKind = 1
	CompletionKindMethod        CompletionKind = 2
	CompletionKindFunction      CompletionKind = 3
	CompletionKindConstructor   CompletionKind = 4
	CompletionKindField         CompletionKind = 5
	CompletionKindVariable      CompletionKind = 6
	CompletionKindClass         CompletionKind = 7
	CompletionKindInterface     CompletionKind = 8
	CompletionKindModule        CompletionKind = 9
	CompletionKindProperty      CompletionKind = 10
	CompletionKindUnit          CompletionKind = 11
	CompletionKindValue         CompletionKind = 12
	CompletionKindEnum          CompletionKind = 13
	CompletionKindKeyword       CompletionKind = 14
	CompletionKindSnippet       CompletionKind = 15
	CompletionKindColor         CompletionKind = 16
	CompletionKindFile          CompletionKind = 17
	CompletionKindReference     CompletionKind = 18
	CompletionKindFolder        CompletionKind = 19
	CompletionKindEnumMember    CompletionKind = 20
	CompletionKindConstant      CompletionKind = 21
	CompletionKindStruct        CompletionKind = 22
	CompletionKindEvent         CompletionKind = 23
	CompletionKindOperator      CompletionKind = 24
	CompletionKindTypeParameter CompletionKind = 25
)

// CompletionList for completion results.
type CompletionList struct {
	IsIncomplete bool             `json:"isIncomplete"`
	Items        []CompletionItem `json:"items"`
}

// SignatureHelp for signature information.
type SignatureHelp struct {
	Signatures      []SignatureInformation `json:"signatures"`
	ActiveSignature int                    `json:"activeSignature,omitempty"`
	ActiveParameter int                    `json:"activeParameter,omitempty"`
}

// SignatureInformation for a single signature.
type SignatureInformation struct {
	Label           string                 `json:"label"`
	Documentation   json.RawMessage        `json:"documentation,omitempty"`
	Parameters      []ParameterInformation `json:"parameters,omitempty"`
	ActiveParameter int                    `json:"activeParameter,omitempty"`
}

// ParameterInformation for function parameters.
type ParameterInformation struct {
	Label         interface{}     `json:"label"` // string or [int, int]
	Documentation json.RawMessage `json:"documentation,omitempty"`
}

// SemanticTokensParams for semantic tokens request.
type SemanticTokensParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// SemanticTokens response.
type SemanticTokens struct {
	ResultID string `json:"resultId,omitempty"`
	Data     []int  `json:"data"`
}

// SemanticTokensLegend describes token types and modifiers.
type SemanticTokensLegend struct {
	TokenTypes     []string `json:"tokenTypes"`
	TokenModifiers []string `json:"tokenModifiers"`
}

// FormattingOptions for document formatting.
type FormattingOptions struct {
	TabSize                int  `json:"tabSize"`
	InsertSpaces           bool `json:"insertSpaces"`
	TrimTrailingWhitespace bool `json:"trimTrailingWhitespace,omitempty"`
	InsertFinalNewline     bool `json:"insertFinalNewline,omitempty"`
	TrimFinalNewlines      bool `json:"trimFinalNewlines,omitempty"`
}

// DocumentFormattingParams for formatting.
type DocumentFormattingParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Options      FormattingOptions      `json:"options"`
}

// FoldingRange for code folding.
type FoldingRange struct {
	StartLine      int    `json:"startLine"`
	StartCharacter int    `json:"startCharacter,omitempty"`
	EndLine        int    `json:"endLine"`
	EndCharacter   int    `json:"endCharacter,omitempty"`
	Kind           string `json:"kind,omitempty"` // "comment", "imports", "region"
}

// SelectionRange for smart selection.
type SelectionRange struct {
	Range  Range           `json:"range"`
	Parent *SelectionRange `json:"parent,omitempty"`
}
