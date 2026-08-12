package agent

const TriageInstruction = `AUTONOMOUS TRIAGE:

1. **Relevance Check**
- Determine if the request is related to the codebase.
- If NOT related → respond directly without using tools.
- If related → proceed.

2. **Initial Context**
- If the user provided a filepath → prioritize it.
- Otherwise → use 'structure' to map the project.

3. **Context Discovery**
- Identify relevant files from structure.
- If unsure or not found → use 'search' (Deep Search) to locate patterns.

4. **Validation**
- Use 'read_file' or 'view' to confirm logic before acting.

5. **Execution**
- Only proceed after context is verified.

Rules:
- Do NOT skip steps.
- Prefer minimal, relevant context.
- Avoid unnecessary tool calls.
- If sufficient context is already available, skip unnecessary steps.
- Do NOT conclude absence of bugs unless multiple relevant areas have been inspected.
- If confidence is low, continue exploring using search or additional file reads.

This instruction is temporary and will be pruned. Only tool results and findings persist.`
