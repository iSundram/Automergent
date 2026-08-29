# Codex System Prompts Analysis

This document provides a comprehensive analysis of the Codex system prompt architecture, covering construction, loading, configuration, dynamic injection, templates, model variations, user instruction merging, and versioning.

---

## 1. System Prompt Construction

### 1.1 Core Components

The system prompt in Codex is composed of multiple layers that are assembled at runtime:

| Layer | Source | Type | Description |
|-------|--------|------|-------------|
| **Base Instructions** | `protocol/src/prompts/base_instructions/default.md` | Static file | Default system prompt (275 lines) defining agent personality, capabilities, and guidelines |
| **Model Instructions Template** | `ModelInfo.model_messages.instructions_template` | Dynamic | Per-model instruction template from model catalog |
| **Personality Injection** | `ModelInstructionsVariables` | Template var | Personality-specific text (default/friendly/pragmatic) |
| **Developer Instructions** | `Config.developer_instructions` | User config | User-provided developer message (separate message) |
| **AGENTS.md Instructions** | Discovered from workspace | Contextual user | Project-level instructions from AGENTS.md files |
| **User Instructions** | `UserInstructionsProvider` | Host-provided | Host-supplied instructions (e.g., from Codex home) |
| **World State Sections** | Various `WorldStateSection` implementations | Dynamic | Permissions, tools, collaboration mode, environment, etc. |
| **Extension Contributions** | `ContextContributor` trait | Dynamic | Plugin/skill/MCP contributed prompt fragments |

### 1.2 Base Instructions Default (`protocol/src/prompts/base_instructions/default.md`)

The default base instructions are embedded at compile time via `include_str!`:

```rust
// protocol/src/models.rs:1337
pub const BASE_INSTRUCTIONS_DEFAULT: &str = include_str!("prompts/base_instructions/default.md");
```

Key sections:
- **Identity**: "You are a coding agent running in the Codex CLI..."
- **Personality**: Concise, direct, friendly
- **AGENTS.md spec**: How to discover and apply AGENTS.md files
- **Responsiveness**: Preamble messages, planning guidelines
- **Task execution**: Validation, coding guidelines
- **Tool guidelines**: Shell commands, update_plan tool

### 1.3 Prompt Assembly Flow

```
Session::new() [session.rs:585]
  → Resolve base_instructions priority [session.rs:652-656]:
      1. config.base_instructions (explicit override)
      2. conversation_history.get_base_instructions() (persisted)
      3. model_info.get_model_instructions(personality) (model template)
  → SessionConfiguration.base_instructions set [session.rs:692]
  → build_world_state_for_step() [world_state.rs:33]
      → ModelInstructionsState [model.rs:14]
      → PersonalityState [personality.rs]
      → AgentsMdState [agents_md.rs:27]
      → PermissionsState/CompactPermissionsState
      → CollaborationModeState
      → EnvironmentsState
      → Apps/Plugins instructions
      → Extension contributions
  → build_initial_context_with_world_state() [mod.rs:3456]
      → Renders world state sections into developer/user messages
      → push_prompt_fragment() [mod.rs:991] routes to slots:
          - DeveloperPolicy
          - DeveloperCapabilities
          - ContextualUser
          - SeparateDeveloper
  → build_developer_update_item() / build_contextual_user_message() [updates.rs]
      → Creates ResponseItem::Message with role "developer" or "user"
```

---

## 2. How System Prompts Are Loaded/Configured

### 2.1 Configuration Loading (`core/src/config/mod.rs`)

```rust
// Lines 3774-3789
let file_base_instructions = Self::try_read_non_empty_file(
    fs,
    model_instructions_path,  // from config.model_instructions_file
    "model instructions file",
).await?;
let base_instructions = base_instructions
    .or(file_base_instructions)
    .or(cfg.instructions.clone());  // CLI --instructions flag
let base_instructions_provenance = base_instructions
    .as_ref()
    .map(|_| BaseInstructionsProvenance::Custom);
```

### 2.2 Config Sources (Priority Order)

1. **CLI argument**: `--instructions "text"` or `--model-instructions-file path`
2. **Config file**: `config.toml` → `base_instructions` field
3. **Model catalog**: `ModelInfo.model_messages.instructions_template`
4. **Default**: `BASE_INSTRUCTIONS_DEFAULT` (embedded)

### 2.3 Provenance Tracking (`protocol/src/models.rs:1339-1348`)

```rust
pub enum BaseInstructionsProvenance {
    Custom,                    // User explicitly configured
    Model { model: String },   // Generated from model's template
}
```

Persisted in thread metadata and rollout for session continuity.

---

## 3. Dynamic Prompt Injection

### 3.1 Prompt Fragment System (`extension-api/src/contributors/prompt.rs`)

```rust
pub enum PromptSlot {
    DeveloperPolicy,      // Core policies, permissions
    DeveloperCapabilities, // Tool descriptions, capabilities
    ContextualUser,       // AGENTS.md, user instructions, env context
    SeparateDeveloper,    // Isolated developer messages (e.g., guardian)
}

pub struct PromptFragment {
    slot: PromptSlot,
    text: String,
}
```

### 3.2 Extension Contribution Points

**Thread-scoped** (`ContextContributor::contribute_thread_context`):
- Runs once per thread/session
- Stable context: skills, MCP servers, plugins

**Turn-scoped** (`ContextContributor::contribute_turn_context`):
- Runs per turn
- Dynamic context: current files, diagnostics, etc.

**World State** (`ContextContributor::contribute_world_state`):
- Contributes structured sections with diffing support

### 3.3 Fragment Routing (`mod.rs:991-999`)

```rust
fn push_prompt_fragment(fragment, developer_sections, contextual_user_sections, separate_developer_sections) {
    match fragment.slot() {
        PromptSlot::DeveloperPolicy | PromptSlot::DeveloperCapabilities => {
            developer_sections.push(fragment.text());
        }
        PromptSlot::ContextualUser => {
            contextual_user_sections.push(fragment.text());
        }
        PromptSlot::SeparateDeveloper => {
            separate_developer_sections.push(fragment.text());
        }
    }
}
```

---

## 4. Prompt Templates and Variables

### 4.1 Model Instructions Template (`protocol/src/openai_models.rs:522-537`)

```rust
pub struct ModelMessages {
    pub instructions_template: Option<String>,      // Template with {{ personality }}
    pub instructions_variables: Option<ModelInstructionsVariables>,
    pub approvals: Option<ApprovalMessages>,
    pub collaboration_modes: Option<CollaborationModeMessages>,
    pub auto_review: Option<AutoReviewMessages>,
    pub permissions: Option<PermissionMessages>,
    pub multi_agent: Option<MultiAgentMessages>,
    pub token_budget: Option<ModelTokenBudgetConfig>,
}
```

### 4.2 Personality Variable Substitution (`openai_models.rs:501-519`)

```rust
const PERSONALITY_PLACEHOLDER: &str = "{{ personality }}";

pub fn get_model_instructions(&self, personality: Option<Personality>) -> String {
    if let Some(model_messages) = &self.model_messages
        && let Some(template) = &model_messages.instructions_template
    {
        if model_messages.instructions_variables.is_none() {
            return template.clone();  // No variables → literal template
        }
        let personality_message = model_messages
            .get_personality_message(personality)
            .unwrap_or_default();
        template.replace(PERSONALITY_PLACEHOLDER, personality_message.as_str())
    } else {
        warn!(model = %self.slug, "Model has no instruction template");
        String::new()
    }
}
```

### 4.3 Personality Variables (`openai_models.rs:617-642`)

```rust
pub struct ModelInstructionsVariables {
    pub personality_default: Option<String>,
    pub personality_friendly: Option<String>,
    pub personality_pragmatic: Option<String>,
}

impl ModelInstructionsVariables {
    pub fn is_complete(&self) -> bool {
        self.personality_default.is_some()
            && self.personality_friendly.is_some()
            && self.personality_pragmatic.is_some()
    }
    
    pub fn get_personality_message(&self, personality: Option<Personality>) -> Option<String> {
        match personality {
            Some(Personality::None) => Some(String::new()),
            Some(Personality::Friendly) => self.personality_friendly.clone(),
            Some(Personality::Pragmatic) => self.personality_pragmatic.clone(),
            None => self.personality_default.clone(),
        }
    }
}
```

### 4.4 Template Completeness Check

```rust
fn supports_personality(&self) -> bool {
    self.has_personality_placeholder()
        && self.instructions_variables.as_ref().is_some_and(ModelInstructionsVariables::is_complete)
}
```

---

## 5. Model-Specific Prompt Variations

### 5.1 Model Catalog Integration

Models are defined in the model catalog with per-model `ModelMessages`:

```rust
// openai_models.rs:683-725 (ModelsResponse serialization)
pub fn serialize_model_infos_with_legacy_base<S>(...) {
    models.iter().map(|model| ModelInfoWithLegacyBaseInstructionsRef {
        model,
        base_instructions: model.get_model_instructions(/*personality*/ None),
    }).collect().serialize(serializer)
}
```

### 5.2 Model-Specific Sections

| Section | Source | Model Variation |
|---------|--------|-----------------|
| Base instructions | `ModelInfo.get_model_instructions()` | Template + personality |
| Approval messages | `ModelMessages.approvals` | Per-model approval prompts |
| Collaboration modes | `ModelMessages.collaboration_modes` | Plan/default mode text |
| Auto-review policy | `ModelMessages.auto_review.policy_template` | Model-specific review prompt |
| Permissions | `ModelMessages.permissions` | Per-model permission prompts |
| Multi-agent roles | `ModelMessages.multi_agent.role` | Root/subagent instructions |
| Token budget | `ModelMessages.token_budget` | Model-specific thresholds |

### 5.3 Personality Support Detection

```rust
// openai_models.rs:495-499
pub fn supports_personality(&self) -> bool {
    self.model_messages
        .as_ref()
        .is_some_and(ModelMessages::supports_personality)
}
```

---

## 6. How User Instructions Merge with System Prompts

### 6.1 AGENTS.md Discovery (`core/src/agents_md.rs:52-91`)

```rust
pub(crate) async fn load_project_instructions(
    config: &Config,
    user_instructions: Option<UserInstructions>,  // Host-provided
    environments: &TurnEnvironmentSnapshot,
) -> Option<LoadedAgentsMd> {
    let mut loaded = LoadedAgentsMd::from_user_instructions(user_instructions);
    // Discovers AGENTS.md from project root to CWD
    // Concatenates with separator: "\n\n--- project-doc ---\n\n"
}
```

### 6.2 Merge Strategy (`agents_md.rs:325-396`)

```rust
fn legacy_text(&self) -> String {
    let mut output = String::new();
    let mut has_previous = false;
    
    // 1. Host user instructions first
    if let Some(instructions) = &self.user_instructions {
        output.push_str(&instructions.text);
        has_previous = true;
    }
    
    // 2. Project AGENTS.md files (root → CWD order)
    for entry in &self.entries {
        let is_project = matches!(&entry.provenance, InstructionProvenance::Project { .. });
        if has_previous {
            let separator = if is_project && !previous_was_project {
                AGENTS_MD_SEPARATOR  // "\n\n--- project-doc ---\n\n"
            } else {
                "\n\n"
            };
            output.push_str(separator);
        }
        output.push_str(&entry.contents);
        has_previous = true;
    }
    output
}
```

### 6.3 Precedence Rules (from `default.md:17-26`)

```
Direct system/developer/user instructions (prompt) 
    > More-deeply-nested AGENTS.md files 
    > Root AGENTS.md files
```

### 6.4 Contextual User Fragment Rendering (`context/user_instructions.rs`)

```rust
impl ContextualUserFragment for UserInstructions {
    fn role(&self) -> &'static str { "user" }
    fn markers(&self) -> (&'static str, &'static str) {
        ("# AGENTS.md instructions", "</INSTRUCTIONS>")
    }
    fn body(&self) -> String {
        let directory = self.directory.as_ref()
            .map(|d| format!(" for {d}")).unwrap_or_default();
        format!("{directory}\n\n<INSTRUCTIONS>\n{}\n", self.text)
    }
}
```

---

## 7. Prompt Versioning/Changes

### 7.1 Version Tracking

- **Provenance**: `BaseInstructionsProvenance` tracks origin (Custom vs Model)
- **Rollout persistence**: Base instructions stored in `SessionMeta` (`protocol.rs:2890`)
- **Model catalog versioning**: Models have `upgrade` field with migration markdown

### 7.2 Migration Handling (`openai_models.rs:728-779`)

```rust
pub fn deserialize_model_infos_with_legacy_base<'de, D>(...) {
    // Promotes legacy top-level base_instructions field
    // into ModelMessages.instructions_template when missing
    if let Some(base_instructions) = base_instructions
        && model.model_messages.as_ref()
            .and_then(|m| m.instructions_template.as_ref())
            .is_none()
    {
        messages.instructions_template = Some(base_instructions);
    }
}
```

### 7.3 Session Continuity (`session.rs:624-644`)

```rust
let base_instructions_provenance = if config.base_instructions.is_some() {
    Some(config.base_instructions_provenance.clone().unwrap_or(BaseInstructionsProvenance::Custom))
} else if let Some(inherited_base_instructions) = initial_history.get_base_instructions() {
    // Check if inherited matches current model template
    provenance.or_else(|| {
        (text == model_info.get_model_instructions(config.personality)).then(|| {
            BaseInstructionsProvenance::Model { model: model_info.slug.clone() }
        })
    })
} else {
    Some(BaseInstructionsProvenance::Model { model: model_info.slug.clone() })
};
```

### 7.4 Model Upgrade Path

Models can declare upgrades with migration instructions:
```rust
// openai_models.rs:644-657
pub struct ModelInfoUpgrade {
    pub model: String,
    pub migration_markdown: String,
    pub retirement_at: Option<DateTime<Utc>>,
}
```

---

## 8. Key Code References Summary

| Feature | File | Lines |
|---------|------|-------|
| Default base instructions | `protocol/src/prompts/base_instructions/default.md` | 1-275 |
| BASE_INSTRUCTIONS_DEFAULT const | `protocol/src/models.rs` | 1337 |
| BaseInstructions struct | `protocol/src/models.rs` | 1351-1368 |
| Model instructions template | `protocol/src/openai_models.rs` | 522-537 |
| get_model_instructions() | `protocol/src/openai_models.rs` | 501-519 |
| Personality variables | `protocol/src/openai_models.rs` | 617-642 |
| Config loading | `core/src/config/mod.rs` | 3774-3789 |
| Session base_instructions resolution | `core/src/session/session.rs` | 652-656 |
| World state building | `core/src/session/world_state.rs` | 33-298 |
| Initial context assembly | `core/src/session/mod.rs` | 3456-3659 |
| Prompt fragment system | `extension-api/src/contributors/prompt.rs` | 1-50 |
| ContextContributor trait | `extension-api/src/contributors.rs` | 82-118 |
| AGENTS.md loading | `core/src/agents_md.rs` | 52-160 |
| AGENTS.md merge | `core/src/agents_md.rs` | 325-396 |
| Message building | `core/src/context_manager/updates.rs` | 11-65 |
| Provenance enum | `protocol/src/models.rs` | 1339-1348 |
| Model catalog serialization | `protocol/src/openai_models.rs` | 683-779 |

---

## 9. Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────┐
                        MODEL REQUEST                             
└─────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────┐
                    build_initial_context_with_world_state       
└─────────────────────────────────────────────────────────────────┘
                                  │
        ┌─────────────────────────┼─────────────────────────┐
        ▼                         ▼                         ▼
┌───────────────┐       ┌─────────────────┐       ┌─────────────────┐
│ DEVELOPER     │       │ CONTEXTUAL USER │       │ SEPARATE        │
│ MESSAGES      │       │ MESSAGES        │       │ DEVELOPER       │
│ (policy,      │       │ (AGENTS.md,     │       │ (guardian,      │
│  capabilities)│       │  user instr,    │       │  multi-agent)   │
└───────┬───────┘       └────────┬────────┘       └────────┬────────┘
        │                        │                        │
        └────────────────────────┼────────────────────────┘
                                 ▼
                    ┌───────────────────────┐
                    │ build_developer_      │
                    │ update_item() /       │
                    │ build_contextual_     │
                    │ user_message()        │
                    └───────────┬───────────┘
                                ▼
                    ┌───────────────────────┐
                    │ ResponseItem::Message │
                    │   (to model API)      │
                    └───────────────────────┘
```

---

## 10. Summary

The Codex system prompt architecture is a **layered, composable system** that:

1. **Starts with a compiled-in default** (`BASE_INSTRUCTIONS_DEFAULT`)
2. **Allows model-specific overrides** via model catalog templates with personality variables
3. **Supports user configuration** via CLI, config file, and host-provided instructions
4. **Discovers and merges project instructions** (AGENTS.md) with clear precedence
5. **Injects dynamic context** via extension contributors (skills, MCP, plugins)
6. **Structures output into semantic slots** (developer policy, capabilities, contextual user, separate developer)
7. **Tracks provenance** for auditability and session continuity
8. **Handles model upgrades** with migration paths

The separation of **developer** (system-level policies/capabilities) and **contextual user** (project/user instructions) messages allows the model to distinguish between immutable system guidance and mutable project context.
