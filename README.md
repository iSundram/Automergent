<div align="center" style="background:#2b2b2b; border-radius:8px; overflow:hidden; margin:24px 0">

<!-- ═══════════════════════════════════════════════════════════════════
     SECTION 1 · HEADER BAR
     ═══════════════════════════════════════════════════════════════════ -->

<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 960 62" width="100%" style="display:block">
  <defs>
    <style>
      .mono { font-family:'SF Mono','Cascadia Code','Fira Code','JetBrains Mono','Consolas',monospace }
    </style>
  </defs>

  <!-- background -->
  <rect width="960" height="62" fill="#2b2b2b"/>

  <!-- brand glyph ✦ -->
  <text x="24" y="38" class="mono" fill="#89b4fa" font-size="20" font-weight="700">✦</text>
  <!-- brand name -->
  <text x="50" y="38" class="mono" fill="#89b4fa" font-size="20" font-weight="700">AUTOMERGENT</text>

  <!-- phase chip · README -->
  <rect x="224" y="22" width="100" height="22" rx="4" fill="#89b4fa"/>
  <text x="246" y="37" class="mono" fill="#2b2b2b" font-size="12" font-weight="700">README</text>

  <!-- center: tagline -->
  <text x="480" y="38" class="mono" fill="#b3b3b3" font-size="13" text-anchor="middle">terminal-native AI agent</text>

  <!-- right: version info -->
  <text x="936" y="38" class="mono" fill="#6c7086" font-size="12" text-anchor="end">v0.1.0 · go1.25</text>

  <!-- separator -->
  <line x1="0" y1="61" x2="960" y2="61" stroke="#4a4a4a" stroke-width="1"/>
</svg>


<!-- ═══════════════════════════════════════════════════════════════════
     SECTION 2 · "WHAT IS THIS" — CONVERSATION TURN
     ═══════════════════════════════════════════════════════════════════ -->

<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 960 270" width="100%" style="display:block">
  <defs>
    <style>
      .mono { font-family:'SF Mono','Cascadia Code','Fira Code','JetBrains Mono','Consolas',monospace }
    </style>
  </defs>

  <rect width="960" height="270" fill="#2b2b2b"/>

  <!-- assistant label -->
  <text x="24" y="36" class="mono" fill="#89b4fa" font-size="14" font-weight="700">✦</text>

  <!-- title -->
  <text x="50" y="36" class="mono" fill="#ffffff" font-size="15" font-weight="700">What is Automergent?</text>

  <!-- body lines -->
  <text x="50" y="62" class="mono" fill="#d4d4d4" font-size="13">An AI-powered coding agent that lives in your terminal.</text>
  <text x="50" y="80" class="mono" fill="#d4d4d4" font-size="13">It reads, writes, searches, and edits your codebase through a</text>
  <text x="50" y="98" class="mono" fill="#d4d4d4" font-size="13">rich TUI — built on the Charm stack (bubbletea, lipgloss, glamour).</text>
  <text x="50" y="116" class="mono" fill="#d4d4d4" font-size="13">It thinks, plans, and executes — with your approval at every step.</text>

  <!-- ── tool card: grep (search) ── -->
  <rect x="24" y="134" width="912" height="24" rx="4" fill="#3c3c3c" opacity="0.4"/>
  <text x="36" y="150" class="mono" fill="#6c7086" font-size="12">●</text>
  <text x="54" y="150" class="mono" fill="#89dceb" font-size="12" font-weight="700">Grep</text>
  <text x="98" y="150" class="mono" fill="#d4d4d4" font-size="12">  "func main"</text>
  <text x="228" y="150" class="mono" fill="#6c7086" font-size="12">· 12 matches in 4 files</text>
  <text x="896" y="150" class="mono" fill="#6c7086" font-size="12" text-anchor="end">0.3s</text>
  <!-- result rows -->
  <text x="62" y="170" class="mono" fill="#6c7086" font-size="11">⎿  cmd/automergent/main.go:24</text>
  <text x="62" y="186" class="mono" fill="#6c7086" font-size="11">⎿  internal/tui/app/app.go:8</text>

  <!-- ── tool card: read_file ── -->
  <rect x="24" y="200" width="912" height="24" rx="4" fill="#3c3c3c" opacity="0.4"/>
  <text x="36" y="216" class="mono" fill="#6c7086" font-size="12">●</text>
  <text x="54" y="216" class="mono" fill="#89dceb" font-size="12" font-weight="700">ReadFile</text>
  <text x="134" y="216" class="mono" fill="#d4d4d4" font-size="12">  "internal/tools/registry.go"</text>
  <text x="380" y="216" class="mono" fill="#6c7086" font-size="12">· 214 lines</text>
  <text x="896" y="216" class="mono" fill="#6c7086" font-size="12" text-anchor="end">0.1s</text>

  <!-- separator -->
  <line x1="0" y1="240" x2="960" y2="240" stroke="#4a4a4a" stroke-width="1"/>

  <!-- spacing -->
</svg>


<!-- ═══════════════════════════════════════════════════════════════════
     SECTION 3 · CAPABILITIES — TOOL CARD GRID
     ═══════════════════════════════════════════════════════════════════ -->

<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 960 420" width="100%" style="display:block">
  <defs>
    <style>
      .mono { font-family:'SF Mono','Cascadia Code','Fira Code','JetBrains Mono','Consolas',monospace }
    </style>
  </defs>

  <rect width="960" height="420" fill="#2b2b2b"/>

  <!-- section label -->
  <text x="24" y="28" class="mono" fill="#6c7086" font-size="11">✦  capabilities</text>

  <!-- ─── ROW 1 ─── -->

  <!-- card: Filesystem -->
  <rect x="24" y="44" width="450" height="72" rx="6" fill="#3c3c3c" opacity="0.35"/>
  <rect x="24" y="44" width="4" height="72" rx="2" fill="#89b4fa"/>
  <text x="42" y="66" class="mono" fill="#89b4fa" font-size="13" font-weight="700">Filesystem</text>
  <text x="42" y="84" class="mono" fill="#b3b3b3" font-size="11">read_file · write_file · edit_file</text>
  <text x="42" y="100" class="mono" fill="#6c7086" font-size="11">list_dir · glob · grep · tree</text>

  <!-- card: Terminal -->
  <rect x="486" y="44" width="450" height="72" rx="6" fill="#3c3c3c" opacity="0.35"/>
  <rect x="486" y="44" width="4" height="72" rx="2" fill="#a6e3a1"/>
  <text x="504" y="66" class="mono" fill="#a6e3a1" font-size="13" font-weight="700">Terminal</text>
  <text x="504" y="84" class="mono" fill="#b3b3b3" font-size="11">shell · exec · background processes</text>
  <text x="504" y="100" class="mono" fill="#6c7086" font-size="11">interactive sessions · stdin/stdout</text>

  <!-- ─── ROW 2 ─── -->

  <!-- card: Code Intelligence -->
  <rect x="24" y="128" width="450" height="72" rx="6" fill="#3c3c3c" opacity="0.35"/>
  <rect x="24" y="128" width="4" height="72" rx="2" fill="#cba6f7"/>
  <text x="42" y="150" class="mono" fill="#cba6f7" font-size="13" font-weight="700">Code Intelligence</text>
  <text x="42" y="168" class="mono" fill="#b3b3b3" font-size="11">tree-sitter parsing · AST queries</text>
  <text x="42" y="184" class="mono" fill="#6c7086" font-size="11">syntax-aware edits · symbol search</text>

  <!-- card: Web -->
  <rect x="486" y="128" width="450" height="72" rx="6" fill="#3c3c3c" opacity="0.35"/>
  <rect x="486" y="128" width="4" height="72" rx="2" fill="#f9e2af"/>
  <text x="504" y="150" class="mono" fill="#f9e2af" font-size="13" font-weight="700">Web</text>
  <text x="504" y="168" class="mono" fill="#b3b3b3" font-size="11">fetch · search · scrape</text>
  <text x="504" y="184" class="mono" fill="#6c7086" font-size="11">live web context for your codebase</text>

  <!-- ─── ROW 3 ─── -->

  <!-- card: LSP / Diagnostics -->
  <rect x="24" y="212" width="450" height="72" rx="6" fill="#3c3c3c" opacity="0.35"/>
  <rect x="24" y="212" width="4" height="72" rx="2" fill="#f38ba8"/>
  <text x="42" y="234" class="mono" fill="#f38ba8" font-size="13" font-weight="700">Diagnostics</text>
  <text x="42" y="252" class="mono" fill="#b3b3b3" font-size="11">lsp_diagnostics · lint errors</text>
  <text x="42" y="268" class="mono" fill="#6c7086" font-size="11">type checking · build verification</text>

  <!-- card: Agent Orchestration -->
  <rect x="486" y="212" width="450" height="72" rx="6" fill="#3c3c3c" opacity="0.35"/>
  <rect x="486" y="212" width="4" height="72" rx="2" fill="#89dceb"/>
  <text x="504" y="234" class="mono" fill="#89dceb" font-size="13" font-weight="700">Agent Orchestration</text>
  <text x="504" y="252" class="mono" fill="#b3b3b3" font-size="11">sub-agents · task planning</text>
  <text x="504" y="268" class="mono" fill="#6c7086" font-size="11">background docks · concurrent execution</text>

  <!-- ─── ROW 4 ─── -->

  <!-- card: MCP -->
  <rect x="24" y="296" width="450" height="72" rx="6" fill="#3c3c3c" opacity="0.35"/>
  <rect x="24" y="296" width="4" height="72" rx="2" fill="#b3b3b3"/>
  <text x="42" y="318" class="mono" fill="#ffffff" font-size="13" font-weight="700">MCP Protocol</text>
  <text x="42" y="336" class="mono" fill="#b3b3b3" font-size="11">Model Context Protocol servers</text>
  <text x="42" y="352" class="mono" fill="#6c7086" font-size="11">extensible tool ecosystem</text>

  <!-- card: Session & Config -->
  <rect x="486" y="296" width="450" height="72" rx="6" fill="#3c3c3c" opacity="0.35"/>
  <rect x="486" y="296" width="4" height="72" rx="2" fill="#d4d4d4"/>
  <text x="504" y="318" class="mono" fill="#ffffff" font-size="13" font-weight="700">Session &amp; Config</text>
  <text x="504" y="336" class="mono" fill="#b3b3b3" font-size="11">viper config · session persistence</text>
  <text x="504" y="352" class="mono" fill="#6c7086" font-size="11">conversation history · project memory</text>

  <!-- separator -->
  <line x1="0" y1="384" x2="960" y2="384" stroke="#4a4a4a" stroke-width="1"/>
</svg>


<!-- ═══════════════════════════════════════════════════════════════════
     SECTION 4 · LIVE TUI SNAPSHOT
     ═══════════════════════════════════════════════════════════════════ -->

<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 960 370" width="100%" style="display:block">
  <defs>
    <style>
      .mono { font-family:'SF Mono','Cascadia Code','Fira Code','JetBrains Mono','Consolas',monospace }
    </style>
  </defs>

  <rect width="960" height="370" fill="#2b2b2b"/>

  <!-- header bar -->
  <rect x="0" y="0" width="960" height="38" fill="#2b2b2b"/>
  <text x="24" y="24" class="mono" fill="#89b4fa" font-size="13" font-weight="700">✦ AUTOMERGENT</text>
  <rect x="184" y="10" width="92" height="20" rx="4" fill="#a6e3a1"/>
  <text x="200" y="24" class="mono" fill="#2b2b2b" font-size="11" font-weight="700">EXECUTE</text>
  <text x="480" y="24" class="mono" fill="#b3b3b3" font-size="11" text-anchor="middle">gemini-2.5-pro</text>
  <text x="896" y="24" class="mono" fill="#b3b3b3" font-size="11" text-anchor="end">$0.0042 │ 12.3k/200k</text>
  <line x1="0" y1="37" x2="960" y2="37" stroke="#4a4a4a" stroke-width="1"/>

  <!-- user message (right-aligned) -->
  <text x="760" y="66" class="mono" fill="#6c7086" font-size="11"> You </text>
  <rect x="626" y="74" width="310" height="26" rx="4" fill="#3c3c3c"/>
  <text x="638" y="91" class="mono" fill="#d4d4d4" font-size="12">refactor the auth middleware</text>

  <!-- assistant label -->
  <text x="36" y="124" class="mono" fill="#89b4fa" font-size="13" font-weight="700">✦</text>

  <!-- assistant body -->
  <text x="52" y="124" class="mono" fill="#d4d4d4" font-size="12">I'll read the current auth middleware, then refactor it.</text>

  <!-- tool card 1: read_file -->
  <rect x="36" y="140" width="888" height="24" rx="4" fill="#3c3c3c" opacity="0.4"/>
  <text x="48" y="156" class="mono" fill="#6c7086" font-size="11">●</text>
  <text x="64" y="156" class="mono" fill="#89dceb" font-size="11" font-weight="700">ReadFile</text>
  <text x="138" y="156" class="mono" fill="#d4d4d4" font-size="11">  "internal/middleware/auth.go"</text>
  <text x="372" y="156" class="mono" fill="#6c7086" font-size="11">· 86 lines</text>
  <text x="884" y="156" class="mono" fill="#6c7086" font-size="11" text-anchor="end">0.08s</text>
  <text x="72" y="172" class="mono" fill="#6c7086" font-size="10">⎿  file read successfully</text>

  <!-- tool card 2: edit_file -->
  <rect x="36" y="182" width="888" height="24" rx="4" fill="#3c3c3c" opacity="0.4"/>
  <text x="48" y="198" class="mono" fill="#6c7086" font-size="11">●</text>
  <text x="64" y="198" class="mono" fill="#89dceb" font-size="11" font-weight="700">EditFile</text>
  <text x="136" y="198" class="mono" fill="#d4d4d4" font-size="11">  "internal/middleware/auth.go"</text>
  <text x="370" y="198" class="mono" fill="#6c7086" font-size="11">· 3 changes</text>
  <text x="884" y="198" class="mono" fill="#6c7086" font-size="11" text-anchor="end">0.12s</text>
  <text x="72" y="214" class="mono" fill="#a6e3a1" font-size="10">⎿  ✓ 3 hunks applied</text>

  <!-- tool card 3: shell -->
  <rect x="36" y="224" width="888" height="24" rx="4" fill="#3c3c3c" opacity="0.4"/>
  <text x="48" y="240" class="mono" fill="#6c7086" font-size="11">●</text>
  <text x="64" y="240" class="mono" fill="#89dceb" font-size="11" font-weight="700">Shell</text>
  <text x="112" y="240" class="mono" fill="#d4d4d4" font-size="11">  "go build ./..."</text>
  <text x="262" y="240" class="mono" fill="#6c7086" font-size="11">· exit 0</text>
  <text x="884" y="240" class="mono" fill="#6c7086" font-size="11" text-anchor="end">2.4s</text>
  <text x="72" y="256" class="mono" fill="#a6e3a1" font-size="10">⎿  ✓ build succeeded</text>

  <!-- assistant reply -->
  <text x="36" y="282" class="mono" fill="#89b4fa" font-size="13" font-weight="700">✦</text>
  <text x="52" y="282" class="mono" fill="#d4d4d4" font-size="12">Done. Refactored auth middleware — extracted token validation</text>
  <text x="52" y="298" class="mono" fill="#d4d4d4" font-size="12">into a separate function, added context-based user lookup.</text>

  <!-- input line -->
  <line x1="0" y1="316" x2="960" y2="316" stroke="#4a4a4a" stroke-width="1"/>
  <text x="24" y="334" class="mono" fill="#6c7086" font-size="12">└─ shift+tab browse · ctrl+p palette</text>

  <!-- status bar -->
  <rect x="0" y="340" width="960" height="30" fill="#2b2b2b"/>
  <line x1="0" y1="340" x2="960" y2="340" stroke="#4a4a4a" stroke-width="1"/>
  <rect x="20" y="346" width="98" height="18" rx="3" fill="#89b4fa"/>
  <text x="30" y="359" class="mono" fill="#2b2b2b" font-size="10" font-weight="700">ACCEPT EDITS</text>
  <text x="134" y="359" class="mono" fill="#b3b3b3" font-size="10">Ready</text>
  <text x="750" y="359" class="mono" fill="#b3b3b3" font-size="10">ctx 12% ##──────</text>
  <text x="900" y="359" class="mono" fill="#89b4fa" font-size="10"> main</text>
</svg>


<!-- ═══════════════════════════════════════════════════════════════════
     SECTION 5 · INSTALLATION — TERMINAL BLOCK
     ═══════════════════════════════════════════════════════════════════ -->

<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 960 170" width="100%" style="display:block">
  <defs>
    <style>
      .mono { font-family:'SF Mono','Cascadia Code','Fira Code','JetBrains Mono','Consolas',monospace }
    </style>
  </defs>

  <rect width="960" height="170" fill="#2b2b2b"/>

  <!-- section label -->
  <text x="24" y="28" class="mono" fill="#6c7086" font-size="11">✦  install</text>

  <!-- terminal window -->
  <rect x="24" y="40" width="912" height="116" rx="8" fill="#1e1e2e"/>
  <rect x="24" y="40" width="912" height="28" rx="8" fill="#313244"/>
  <rect x="24" y="60" width="912" height="8" fill="#313244"/>
  <!-- dots -->
  <circle cx="44" cy="54" r="5" fill="#f38ba8"/>
  <circle cx="60" cy="54" r="5" fill="#f9e2af"/>
  <circle cx="76" cy="54" r="5" fill="#a6e3a1"/>
  <text x="480" y="58" class="mono" fill="#6c7086" font-size="10" text-anchor="middle">terminal</text>

  <!-- commands -->
  <text x="44" y="90" class="mono" fill="#b3b3b3" font-size="12">$</text>
  <text x="58" y="90" class="mono" fill="#89dceb" font-size="12">go install github.com/iSundram/Automergent@latest</text>

  <text x="44" y="110" class="mono" fill="#b3b3b3" font-size="12">$</text>
  <text x="58" y="110" class="mono" fill="#a6e3a1" font-size="12">automergent</text>

  <text x="44" y="130" class="mono" fill="#b3b3b3" font-size="12">$</text>
  <text x="58" y="130" class="mono" fill="#6c7086" font-size="12">✦  type a prompt or /help for commands</text>
</svg>


<!-- ═══════════════════════════════════════════════════════════════════
     SECTION 6 · THEMES PALETTE
     ═══════════════════════════════════════════════════════════════════ -->

<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 960 130" width="100%" style="display:block">
  <defs>
    <style>
      .mono { font-family:'SF Mono','Cascadia Code','Fira Code','JetBrains Mono','Consolas',monospace }
    </style>
  </defs>

  <rect width="960" height="130" fill="#2b2b2b"/>

  <!-- section label -->
  <text x="24" y="28" class="mono" fill="#6c7086" font-size="11">✦  themes</text>

  <!-- theme swatches -->
  <!-- modern (default) -->
  <rect x="24" y="44" width="52" height="36" rx="6" fill="#2b2b2b" stroke="#4a4a4a" stroke-width="1"/>
  <circle cx="50" cy="62" r="6" fill="#ffffff"/>
  <text x="50" y="96" class="mono" fill="#ffffff" font-size="10" text-anchor="middle">modern</text>

  <!-- catppuccin -->
  <rect x="104" y="44" width="52" height="36" rx="6" fill="#1e1e2e" stroke="#45475a" stroke-width="1"/>
  <circle cx="130" cy="62" r="6" fill="#cba6f7"/>
  <text x="130" y="96" class="mono" fill="#bac2de" font-size="10" text-anchor="middle">catppuccin</text>

  <!-- dracula -->
  <rect x="184" y="44" width="52" height="36" rx="6" fill="#282a36" stroke="#44475a" stroke-width="1"/>
  <circle cx="210" cy="62" r="6" fill="#bd93f9"/>
  <text x="210" y="96" class="mono" fill="#f8f8f2" font-size="10" text-anchor="middle">dracula</text>

  <!-- nord -->
  <rect x="264" y="44" width="52" height="36" rx="6" fill="#2e3440" stroke="#3b4252" stroke-width="1"/>
  <circle cx="290" cy="62" r="6" fill="#88c0d0"/>
  <text x="290" y="96" class="mono" fill="#d8dee9" font-size="10" text-anchor="middle">nord</text>

  <!-- gruvbox -->
  <rect x="344" y="44" width="52" height="36" rx="6" fill="#282828" stroke="#504945" stroke-width="1"/>
  <circle cx="370" cy="62" r="6" fill="#fe8019"/>
  <text x="370" y="96" class="mono" fill="#ebdbb2" font-size="10" text-anchor="middle">gruvbox</text>

  <!-- onedark -->
  <rect x="424" y="44" width="52" height="36" rx="6" fill="#282c34" stroke="#3e4451" stroke-width="1"/>
  <circle cx="450" cy="62" r="6" fill="#61afef"/>
  <text x="450" y="96" class="mono" fill="#abb2bf" font-size="10" text-anchor="middle">onedark</text>

  <!-- tokyonight -->
  <rect x="504" y="44" width="52" height="36" rx="6" fill="#1a1b26" stroke="#414868" stroke-width="1"/>
  <circle cx="530" cy="62" r="6" fill="#7aa2f7"/>
  <text x="530" y="96" class="mono" fill="#c0caf5" font-size="10" text-anchor="middle">tokyonight</text>

  <!-- solarized-dark -->
  <rect x="584" y="44" width="52" height="36" rx="6" fill="#002b36" stroke="#586e75" stroke-width="1"/>
  <circle cx="610" cy="62" r="6" fill="#268bd2"/>
  <text x="610" y="96" class="mono" fill="#839496" font-size="10" text-anchor="middle">solarized</text>

  <!-- monokai -->
  <rect x="664" y="44" width="52" height="36" rx="6" fill="#272822" stroke="#49483e" stroke-width="1"/>
  <circle cx="690" cy="62" r="6" fill="#a6e22e"/>
  <text x="690" y="96" class="mono" fill="#f8f8f2" font-size="10" text-anchor="middle">monokai</text>

  <!-- high-contrast -->
  <rect x="744" y="44" width="52" height="36" rx="6" fill="#000000" stroke="#666666" stroke-width="1"/>
  <circle cx="770" cy="62" r="6" fill="#00ff00"/>
  <text x="770" y="96" class="mono" fill="#ffffff" font-size="10" text-anchor="middle">high-contrast</text>

  <!-- separator -->
  <line x1="0" y1="112" x2="960" y2="112" stroke="#4a4a4a" stroke-width="1"/>
</svg>


<!-- ═══════════════════════════════════════════════════════════════════
     SECTION 7 · ARCHITECTURE — BOX-DRAWING TREE
     ═══════════════════════════════════════════════════════════════════ -->

<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 960 290" width="100%" style="display:block">
  <defs>
    <style>
      .mono { font-family:'SF Mono','Cascadia Code','Fira Code','JetBrains Mono','Consolas',monospace }
    </style>
  </defs>

  <rect width="960" height="290" fill="#2b2b2b"/>

  <!-- section label -->
  <text x="24" y="28" class="mono" fill="#6c7086" font-size="11">✦  architecture</text>

  <!-- root -->
  <text x="44" y="56" class="mono" fill="#89b4fa" font-size="12" font-weight="700">✦ Automergent</text>

  <!-- branch: cmd -->
  <text x="44" y="80" class="mono" fill="#4a4a4a" font-size="11">├──</text>
  <text x="76" y="80" class="mono" fill="#89b4fa" font-size="11" font-weight="700">cmd/</text>
  <text x="118" y="80" class="mono" fill="#6c7086" font-size="11">CLI entry points · cobra commands</text>

  <!-- branch: internal -->
  <text x="44" y="100" class="mono" fill="#4a4a4a" font-size="11">├──</text>
  <text x="76" y="100" class="mono" fill="#89b4fa" font-size="11" font-weight="700">internal/</text>

  <!-- sub: tui -->
  <text x="64" y="120" class="mono" fill="#4a4a4a" font-size="11">│   ├──</text>
  <text x="120" y="120" class="mono" fill="#cba6f7" font-size="11" font-weight="700">tui/</text>
  <text x="158" y="120" class="mono" fill="#6c7086" font-size="11">bubbletea interface · lipgloss styles · glamour markdown</text>

  <!-- sub: agent -->
  <text x="64" y="140" class="mono" fill="#4a4a4a" font-size="11">│   ├──</text>
  <text x="120" y="140" class="mono" fill="#a6e3a1" font-size="11" font-weight="700">agent/</text>
  <text x="172" y="140" class="mono" fill="#6c7086" font-size="11">AI orchestration · tool dispatch · planning</text>

  <!-- sub: tools -->
  <text x="64" y="160" class="mono" fill="#4a4a4a" font-size="11">│   ├──</text>
  <text x="120" y="160" class="mono" fill="#f9e2af" font-size="11" font-weight="700">tools/</text>
  <text x="170" y="160" class="mono" fill="#6c7086" font-size="11">48 tools · filesystem · shell · web · LSP · security</text>

  <!-- sub: logo -->
  <text x="64" y="180" class="mono" fill="#4a4a4a" font-size="11">│   ├──</text>
  <text x="120" y="180" class="mono" fill="#f38ba8" font-size="11" font-weight="700">logo/</text>
  <text x="166" y="180" class="mono" fill="#6c7086" font-size="11">SVG → terminal rasterizer · ANSI encoder</text>

  <!-- sub: config, session, ai, mcp, version -->
  <text x="64" y="200" class="mono" fill="#4a4a4a" font-size="11">│   ├──</text>
  <text x="120" y="200" class="mono" fill="#89dceb" font-size="11" font-weight="700">ai/</text>
  <text x="148" y="200" class="mono" fill="#6c7086" font-size="11">provider abstraction · gemini / openai</text>

  <text x="64" y="220" class="mono" fill="#4a4a4a" font-size="11">│   ├──</text>
  <text x="120" y="220" class="mono" fill="#d4d4d4" font-size="11" font-weight="700">config/</text>
  <text x="180" y="220" class="mono" fill="#6c7086" font-size="11">viper configuration · environment</text>

  <text x="64" y="240" class="mono" fill="#4a4a4a" font-size="11">│   └──</text>
  <text x="120" y="240" class="mono" fill="#b3b3b3" font-size="11" font-weight="700">session/</text>
  <text x="188" y="240" class="mono" fill="#6c7086" font-size="11">persistence · conversation history</text>

  <!-- branch: go.mod -->
  <text x="44" y="264" class="mono" fill="#4a4a4a" font-size="11">└──</text>
  <text x="76" y="264" class="mono" fill="#b3b3b3" font-size="11">go.mod</text>
  <text x="136" y="264" class="mono" fill="#6c7086" font-size="11">go 1.25 · charm stack v2 · cobra · viper · tree-sitter</text>

  <!-- separator -->
  <line x1="0" y1="280" x2="960" y2="280" stroke="#4a4a4a" stroke-width="1"/>
</svg>


<!-- ═══════════════════════════════════════════════════════════════════
     SECTION 8 · FOOTER — STATUS BAR + TIPS
     ═══════════════════════════════════════════════════════════════════ -->

<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 960 130" width="100%" style="display:block">
  <defs>
    <style>
      .mono { font-family:'SF Mono','Cascadia Code','Fira Code','JetBrains Mono','Consolas',monospace }
    </style>
  </defs>

  <rect width="960" height="130" fill="#2b2b2b"/>

  <!-- tips row 1 -->
  <text x="24" y="28" class="mono" fill="#89b4fa" font-size="12" font-weight="700">✦</text>
  <text x="44" y="28" class="mono" fill="#ffffff" font-size="12" font-weight="700">Tips</text>

  <text x="24" y="50" class="mono" fill="#6c7086" font-size="11">├─</text>
  <text x="50" y="50" class="mono" fill="#a6e3a1" font-size="11">/help</text>
  <text x="100" y="50" class="mono" fill="#6c7086" font-size="11">  show all commands</text>

  <text x="280" y="50" class="mono" fill="#6c7086" font-size="11">├─</text>
  <text x="306" y="50" class="mono" fill="#a6e3a1" font-size="11">/theme</text>
  <text x="364" y="50" class="mono" fill="#6c7086" font-size="11">  switch color palette</text>

  <text x="560" y="50" class="mono" fill="#6c7086" font-size="11">├─</text>
  <text x="586" y="50" class="mono" fill="#a6e3a1" font-size="11">/model</text>
  <text x="644" y="50" class="mono" fill="#6c7086" font-size="11">  change AI provider</text>

  <!-- tips row 2 -->
  <text x="24" y="68" class="mono" fill="#6c7086" font-size="11">├─</text>
  <text x="50" y="68" class="mono" fill="#a6e3a1" font-size="11">ctrl+p</text>
  <text x="108" y="68" class="mono" fill="#6c7086" font-size="11">  command palette</text>

  <text x="280" y="68" class="mono" fill="#6c7086" font-size="11">├─</text>
  <text x="306" y="68" class="mono" fill="#a6e3a1" font-size="11">ctrl+e</text>
  <text x="364" y="68" class="mono" fill="#6c7086" font-size="11">  expand tool details</text>

  <text x="560" y="68" class="mono" fill="#6c7086" font-size="11">├─</text>
  <text x="586" y="68" class="mono" fill="#a6e3a1" font-size="11">shift+tab</text>
  <text x="664" y="68" class="mono" fill="#6c7086" font-size="11">  browse conversation</text>

  <!-- tips row 3 -->
  <text x="24" y="86" class="mono" fill="#6c7086" font-size="11">└─</text>
  <text x="50" y="86" class="mono" fill="#a6e3a1" font-size="11">esc</text>
  <text x="82" y="86" class="mono" fill="#6c7086" font-size="11">  cancel / close overlay</text>

  <!-- separator -->
  <line x1="0" y1="100" x2="960" y2="100" stroke="#4a4a4a" stroke-width="1"/>

  <!-- footer status bar -->
  <rect x="0" y="100" width="960" height="30" fill="#2b2b2b"/>
  <rect x="20" y="106" width="68" height="18" rx="3" fill="#89b4fa"/>
  <text x="30" y="119" class="mono" fill="#2b2b2b" font-size="10" font-weight="700">MANUAL</text>
  <text x="104" y="119" class="mono" fill="#6c7086" font-size="10">Ready</text>
  <text x="720" y="119" class="mono" fill="#6c7086" font-size="10">ctx 0% ----------</text>
  <text x="870" y="119" class="mono" fill="#89b4fa" font-size="10"> main</text>
</svg>

</div>
