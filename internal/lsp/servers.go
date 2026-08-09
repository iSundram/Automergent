package lsp

// KnownServers maps file extensions to LSP server commands.
var KnownServers = map[string][]string{
	".go":   {"gopls"},
	".ts":   {"typescript-language-server", "--stdio"},
	".tsx":  {"typescript-language-server", "--stdio"},
	".js":   {"typescript-language-server", "--stdio"},
	".jsx":  {"typescript-language-server", "--stdio"},
	".py":   {"pylsp"},
	".rs":   {"rust-analyzer"},
	".java": {"jdtls"},
	".c":    {"clangd"},
	".cpp":  {"clangd"},
	".h":    {"clangd"},
	".rb":   {"solargraph", "stdio"},
	".lua":  {"lua-language-server"},
}

// ServerForFile returns the LSP server command for a given file extension.
func ServerForFile(ext string) ([]string, bool) {
	cmd, ok := KnownServers[ext]
	return cmd, ok
}
