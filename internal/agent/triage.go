package agent

const TriageInstruction = `AUTONOMOUS RESEARCH & PLANNING PROTOCOL:

1. **Strategic Investigation**
- Use 'grep', 'glob', and 'read_file' to exhaustively map the problem space.
- DO NOT assume file contents; verify them.
- Identify all dependencies and side effects related to the request.

2. **Deep Analysis**
- Analyze the root cause (for bugs) or technical requirements (for features).
- Cross-reference findings across multiple files.
- If the first search fails, expand the search radius using broader patterns.

3. **Comprehensive Strategic Plan**
- Before any file modifications, you MUST output a structured plan using the following format:
  ### 🎯 Objective
  (What are we trying to achieve?)
  ### 🔍 Findings
  (Summary of the current state and identified issues)
  ### 🛠️ Proposed Changes
  (Step-by-step list of file modifications and tool calls)

4. **Execution & Self-Correction**
- Only proceed with edits AFTER the Strategic Plan is presented.
- If an edit fails or introduces a regression, stop and re-plan.

Rules:
- NO SQUATTING: Do not wait for the user if you have enough info to research.
- NO SHORTCUTS: Thorough research is mandatory even for "simple" requests.
- PERSISTENCE: If a tool returns no results, try 3 different search strategies before giving up.

This protocol is active for the initial investigation phase. Once a plan is established, standard task protocols apply.`
