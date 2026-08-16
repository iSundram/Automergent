# Automergent TUI Design System

This document is the design contract for Automergent's terminal interface. Read it before creating, redesigning, or extending any existing TUI component. New work should feel like part of one quiet, capable engineering environment rather than a collection of unrelated terminal widgets.

## Product Character

Automergent is a professional coding agent. Its interface should feel focused, calm, precise, and trustworthy. It is an operational workspace used repeatedly for reading code, making decisions, approving actions, reviewing changes, and following long-running work.

The interface is not a marketing page. Avoid decorative layouts, novelty animation, excessive framing, large banners, bright surfaces, and prose that explains obvious controls. Every visible element should help the user understand state, inspect work, or take an action.

The visual hierarchy should communicate:

1. What Automergent is doing now.
2. What content belongs to the user, assistant, or a tool.
3. What requires a decision.
4. What actions and navigation are currently available.
5. What information is secondary and may be scanned later.

## Core Principles

### Use the Terminal Background

The default terminal background is the primary canvas. Do not place ordinary content on a second background color. Command rows, assistant responses, tool details, empty states, help content, and confirmation information should normally inherit the terminal background.

A surface color is reserved for the user's submitted message and rare, genuinely bounded content. It must never become the default way to create hierarchy.

### Prefer Structure Over Boxes

Use whitespace, alignment, typography, a subtle left rail, and thin separator rules before adding borders. Avoid cards inside cards. Avoid full rounded boxes around assistant responses, tools, palettes, and page sections.

Borders are appropriate only when the boundary itself carries meaning, such as a focused modal-like chooser or a compact user-message surface. Even then, use as little border as possible.

### Preserve Reading Width

Assistant responses should use the available conversation width directly. Do not put them in bubbles. Markdown should wrap to the active viewport width and remain readable at narrow terminal sizes.

Tool output may be indented by a subtle rail but should not lose large portions of horizontal space to nested borders and padding.

### Stable Layout Is More Important Than Decoration

Dynamic text, status changes, icons, spinners, scrollbars, and selection states must not cause unrelated elements to move. Compute widths with terminal-cell-aware functions. Never align styled text with byte counts or rune counts when display width can differ.

### State Must Be Visible

Active model, provider, mode, running state, selected command, permission request, review mode, scroll position, and disabled actions should have visible states. Do not rely only on color. Pair color with an icon, rail, label, or concise status text.

## Theme Tokens

The Modern theme is grayscale-first with restrained semantic color.

| Token | Modern value | Intended use |
| --- | --- | --- |
| `Background` | `#2b2b2b` | Primary terminal canvas |
| `Surface` | `#3c3c3c` | User message surface and rare bounded emphasis |
| `Overlay` | `#4a4a4a` | Exceptional overlay depth only |
| `Text` | `#ffffff` | Primary content |
| `Subtext` | `#d4d4d4` | Soft selected text and secondary content |
| `Muted` | `#b3b3b3` | Hints, descriptions, metadata, inactive controls |
| `Accent` | `#ffffff` | Brand, selection rail, focused command name |
| `AccentAlt` | `#d4d4d4` | Softer accent where pure white is too strong |
| `BorderNormal` | `#4a4a4a` | Rules, rails, scrollbar tracks |
| `BorderFocused` | `#ffffff` | Focused boundary only |
| `Green` | `#a6e3a1` | Success and current markers |
| `Red` | `#f38ba8` | Errors and rejection |
| `Yellow` | `#f9e2af` | Warnings and pending permission |
| `Blue` | `#89b4fa` | File or informational semantics |
| `Magenta` | `#cba6f7` | Rare semantic distinction |
| `Cyan` | `#89dceb` | Code-related semantic distinction |

Do not introduce arbitrary colors inside components. Add or reuse a semantic theme token. Other themes may map the tokens differently, so component code must not assume Modern's literal values.

## Typography and Text

- Use normal terminal text for content.
- Use bold sparingly for brand names, section headings, command names, and the most important current state.
- Use uppercase for compact interface labels such as `COMMANDS`, `SESSION`, `PARAMETERS`, or `CURRENT`; do not uppercase paragraphs.
- Use muted text for descriptions, paths, timings, shortcut explanations, and inactive states.
- Use dimmed white (`Subtext` or `AccentAlt`) for selected-row text when pure white appears harsh.
- Do not use negative letter spacing or viewport-scaled type.
- Keep labels concise. Status messages should usually fit on one line.
- Avoid emoji when a Nerd Font or existing icon is available. Emoji width and presentation vary between terminals.
- Use Unicode ellipsis `…` for visible truncation when the surrounding file already supports Unicode.

## Spacing

Terminal spacing is measured in cells and lines.

- One cell separates tightly related tokens.
- Two cells separate icons from primary labels where visual clarity needs it.
- Three or more cells separate independent control groups.
- One blank line separates major content regions.
- Avoid large left margins. Center only compact empty states, welcome content, and true modal decisions.
- Input cursor and placeholder text must touch naturally without overlapping. Cursor width must be accounted for explicitly.
- Header groups should maintain at least one visible cell after the brand and before the next group.
- Labels and values should align from a stable left edge.

Padding should be responsive. At narrow widths remove descriptions and ornamental gaps before truncating command names or essential state.

## Primary Screen Anatomy

The normal screen contains:

```text
  Header: brand · phase/mode · provider/model · token/context state
  ─────────────────────────────────────────────────────────────
  Conversation: user messages, assistant responses, tools
  ─────────────────────────────────────────────────────────────
  Input or contextual replacement: editor, palette, confirmation
  Status bar: active state, shortcuts, elapsed time, diagnostics
```

The header and status bar are persistent orientation. The conversation receives the remaining height. The input area may be replaced by a command palette or confirmation, but replacement controls must not render beneath a stale input.

## Header

The header is a compact single-line workspace summary.

- Brand is the first strong signal when width allows: `⟡ AUTOMERGENT`.
- Phase or mode follows with intentional spacing.
- Provider/model information is centered or right-aligned based on available space.
- Tokens or context usage appear at the right edge.
- Hide low-priority labels progressively on narrow terminals.
- Truncate the model before allowing header groups to wrap incoherently.
- Never render a second background behind one header segment.
- A thin rule below the header separates it from the conversation.

## Conversation

### User Messages

User messages may use the `Surface` background to distinguish submitted input. Keep the design compact and curved only where terminal rendering remains visually complete. Do not create partial border rows whose interior background is missing.

Preferred direction:

```text
You     ▐  message content                                      ▌
```

or a clean background-only text surface when border glyphs create inconsistent fills. The user label should remain secondary to the message.

### Assistant Responses

Assistant responses are borderless and full-width.

```text
 ⟡ Automergent
Response content begins directly below and uses the available width.
```

- Do not use a bubble or card background.
- Preserve markdown hierarchy without oversized headings.
- Rules should fit the content width.
- Tables must degrade safely at narrow widths.
- Inline code should be visible but not become a large rounded badge.
- The welcome state disappears after the first conversation message.

### Tool Lifecycles

Tool rows should feel like an execution timeline, not nested cards.

```text
┃  󰆍  Run  󰄬 Completed                         go test ./...  1.2s
┃  Command  go test ./...
┃  Result   42 tests passed
```

- Inherit the default background.
- Use a subtle left rail for continuity.
- Keep icon, name, status, summary, and duration on a stable header row.
- Align detail field labels from the left.
- Hide verbose output behind review mode or truncation.
- Use consistent states: Running, Completed, Failed, Cancelled.
- Do not show redundant section headings when simple fields are enough.

## Input

The input is a working editor, not a decorative box.

- A thin top rule separates it from conversation content.
- Placeholder text is dimmed white when blurred.
- Focus changes must apply consistently to every placeholder segment.
- Cursor and text must never overlap.
- Enter sends; `/` opens commands; `@` searches files; `?` opens help.
- When scrolling/browsing the conversation, the input may disappear because it is not actionable.
- Confirmations replace the input at the same location.
- The input must not remain visible underneath a palette or permission prompt.

## Command Palette

The command palette is an inline command browser at the input position, not a floating card.

```text
──────────────────────────────────────────────────────────────
  /  COMMANDS                                        1 of 26

    AI & MODEL
  ▍ 󰊕  model <name>             Switch AI model
    󰒋  provider <name>          Switch AI provider

    SESSION
    󰐕  new                      Start a fresh session
    󰆓  sessions                 Browse previous sessions
──────────────────────────────────────────────────────────────
  ↑↓ navigate · enter select · esc close                    ┃
```

Rules:

- Use the default terminal background for every row.
- The selected row uses an accent `▍` rail, accent icon, and soft/dimmed primary text.
- Selection must not introduce a different row background.
- Render stored icons consistently.
- Group commands by meaningful categories.
- Show usage hints beside command names.
- Align descriptions responsively; hide descriptions first on narrow terminals.
- Highlight matching query text.
- Search command names, aliases, descriptions, categories, and usage terms.
- Show `󰄬 Current` for the active provider, model, mode, or theme.
- Disabled commands remain readable and state why they are unavailable.
- Show a right-side scrollbar when results exceed visible height.
- Mouse wheel, arrows, page keys, and category navigation should update the same cursor state.
- Enter executes immediate commands or opens an inline nested selector.
- Escape returns from nested selectors before closing the whole palette.
- Empty and loading states use concise muted text.

## Confirmations and Permissions

Confirmations replace the input between the same separator lines. They should provide enough context for an informed decision.

```text
──────────────────────────────────────────────────────────────
 󰆍  Permission required
 Run  Execute shell command
 Command  make build
 Risk     Runs a local process

 y allow  ·  a always  ·  n reject  ·  f feedback
──────────────────────────────────────────────────────────────
```

- Start fields from the left; do not center command and risk details.
- State the tool, requested action, exact target, and concise risk.
- Never expose secret values in confirmation details or history.
- Use the existing permission pipeline for `/run`, `/test`, `/build`, and agent tool calls.
- `Allow` is one action, `Always` persists an appropriate scoped permission, `Reject` denies, and `Feedback` lets the user redirect.
- Folder trust is separate from individual tool permission. Session trust and remembered trust must behave differently and accurately.
- Keyboard actions must remain visible while the confirmation is active.

## Browsing and Scrollbars

- Conversation browsing hides the inactive input and gives height back to the viewport.
- Mouse wheel scrolling is enabled while browsing and in scrollable palettes.
- Scrollbars appear on the right side.
- The track uses `BorderNormal`; the thumb is more prominent but restrained.
- Thumb size reflects visible content versus total content.
- Scrollbars must not cover text. Reserve one display cell for them.
- Do not show a scrollbar when all content is visible.

## Empty and Welcome States

Empty states should be compact and centered without a card.

```text
                    ⟡  AUTOMERGENT

                Your workspace is ready
       Describe what you want to build, fix, or explore.

          / commands   ·   @ files   ·   ? help
```

- Keep the block narrow enough to read at a glance.
- Show the product identity, current readiness, one action sentence, and discoverable shortcuts.
- Adapt shortcut spacing for narrow terminals.
- Remove the state after the first message.

## Panels and Overlays

- File tree, diagnostics, diff, session browser, and help must follow the same token and spacing rules.
- Full-screen diff is appropriate because reviewing changes is a focused workflow.
- Side panels use stable widths and yield to conversation content at narrow sizes.
- Avoid nested cards and unnecessary rounded borders.
- A focused panel may use `BorderFocused`; inactive panels use `BorderNormal` and reduced emphasis.
- Panel titles remain compact and do not use hero-scale text.

## Installer

The installer is part of the same product and must use the same Modern theme character.

- Present progress as a clear sequence of steps.
- Use default background and restrained semantic colors.
- Show both supported launch commands: `amt` and `automergent`.
- Success output should clearly identify installed paths and the next command.
- Errors must state the failed step and actionable recovery.
- Do not use decorative celebration as the primary hierarchy.

## Session Completion

Session completion should summarize useful state without a large celebratory card.

Include:

- Session identifier.
- Duration.
- Resume command.
- Message and tool counts.
- Input and output token counts.
- A short next action where helpful.

Prefer aligned fields and a subtle heading. Avoid emoji-heavy framing and generic thank-you copy that pushes operational information down.

## Responsive Behavior

Components must be tested at wide, normal, narrow, and very narrow terminal widths.

Degradation order:

1. Remove ornamental gaps.
2. Hide descriptions and secondary metadata.
3. Shorten provider prefixes and model names.
4. Collapse optional header groups.
5. Truncate paths and long values with an ellipsis.
6. Stack only when a one-line layout is no longer coherent.

Never permit:

- Text crossing the right edge.
- Border glyphs wrapping onto unrelated lines.
- A label splitting one character per line.
- Status text covering controls.
- Scrollbars covering content.
- Different terminal widths producing leftover border fragments.

Use `lipgloss.Width`, cell-aware truncation, fixed component dimensions, and calculated available widths.

## Interaction Standards

- `Enter`: confirm, select, or send.
- `Esc`: back, close, or interrupt the active request when appropriate.
- `↑/↓`: move one item or line.
- `PageUp/PageDown`: move one viewport page.
- `Tab/Shift+Tab`: advance or reverse focus/category according to context.
- Mouse wheel: scroll the active scrollable region.
- Click: select rows where coordinate mapping is reliable.
- `Ctrl+C`: interrupt first; repeated press may exit according to the app's established behavior.

The footer must describe only controls that work in the current state.

## Writing Status and Error Messages

Good status text is short, specific, and describes the result:

- `Model changed to gemini-2.5-flash`
- `Review mode enabled`
- `Conversation exported to conversation.md`
- `No active request to cancel`

Avoid vague text such as `Done`, `Error`, or `Something went wrong` when the interface knows more.

Usage errors should show the accepted syntax without pretending the assistant produced a conversational response:

```text
Usage: /run <command>
```

Secrets must never appear in status messages, conversation history, tool summaries, tests, or logs.

## Command Design

Every slash command belongs in the central command registry and should define:

- Canonical name.
- Aliases.
- Icon.
- Category.
- Description.
- Usage hint.
- Whether selection executes immediately or opens argument entry/nested choices.
- Availability or disabled reason when context-dependent.

Execution behavior, palette metadata, and help documentation must not drift apart. New commands require tests for registry uniqueness, alias resolution, missing arguments, success feedback, and their context-dependent states.

Commands that run project processes must go through the agent/tool permission system. Do not execute shell commands directly from a slash-command handler.

## Accessibility and Terminal Compatibility

- Never rely on color alone.
- Maintain readable contrast between `Text`, `Subtext`, `Muted`, and the background.
- Provide textual states alongside icons.
- Account for Nerd Font glyph widths and provide reasonable fallback behavior.
- Avoid emoji-dependent alignment.
- Strip ANSI codes before asserting textual content in tests.
- Check output for accidental background escape codes when default background is required.

## Implementation Guidance

- Reuse `themes.Styles` and semantic tokens.
- Prefer shared render helpers for truncation, left/right alignment, scrollbars, and labeled fields.
- Keep view methods deterministic and side-effect free.
- Keep execution and persistence outside render components.
- Route input events to the visibly active component first.
- When a contextual control replaces input, recalculate layout height immediately.
- Preserve user changes in the working tree and avoid unrelated refactors.

## Required Tests

For any substantial TUI change, add focused coverage for the affected behaviors:

- Wide and narrow rendering.
- Cell width and truncation.
- Default-background requirements.
- Selection styling.
- Empty/loading/error states.
- Scrolling and cursor bounds.
- Mouse and keyboard navigation where supported.
- Visibility and replacement of input/confirm/palette regions.
- Command aliases and arguments.
- Current and disabled states.
- Welcome disappearance after the first message.
- Permission routing for executable commands.

Use focused tests during development:

```bash
env GOCACHE=/tmp/owecode-go-cache go test ./internal/tui/components ./internal/tui
env GOCACHE=/tmp/owecode-go-cache go test ./cmd/automergent -run '^$'
env GOCACHE=/tmp/owecode-go-cache make build
git diff --check
```

## Design Review Checklist

Before considering a TUI change complete, verify:

- It uses the default background unless a surface is semantically justified.
- It does not introduce a card inside another card.
- Selected rows do not rely on a different background.
- Text and borders do not wrap incoherently at narrow widths.
- Labels, values, icons, and durations align using display-cell widths.
- The active state is visible without relying only on color.
- The input is replaced, not duplicated, by contextual controls.
- Mouse and keyboard behavior match the visible footer hints.
- Scrollbars are on the right and do not cover content.
- Tool execution still uses confirmation and permission handling.
- Secrets are never rendered.
- The component fits the established Automergent tone.
- Focused tests pass and `bin/automergent` is rebuilt.

This file is the baseline. When a deliberate new pattern is introduced and accepted, update this document so subsequent TUI work remains consistent.
