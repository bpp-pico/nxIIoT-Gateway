@MEMORY.md

# nxIIoT Gateway — Claude Code operating rules

Project orientation lives in [README.md](README.md) (dev commands, current status), [HANDOFF.md](HANDOFF.md) (architecture decisions, known gaps, what's next), [industrial_iot_gateway_handoff_dev_plan.md](industrial_iot_gateway_handoff_dev_plan.md) (full design spec), and [DEPLOY_PLAN.md](DEPLOY_PLAN.md) (Raspberry Pi deployment session logs). Read [spec.md](spec.md) at the start of a session before doing anything — it's the save point for where things actually stand right now.

## Operating Rules

### NO MAGIC — don't guess
All assumptions explicit. If context is missing, state assumptions.
Don't hallucinate hidden infra or invent unspecified services.
If you don't know where something lives, ask — don't guess the path.

### VERIFY BEFORE DONE — no "done" without evidence
Never claim a change is complete without running verification.
"I edited the file" is not done. "I edited the file and here's the output" is done.
No "should work now." Evidence before assertions, always.

### DISSENT — argue before you commit
Before any major change, surface concerns:
- What's the blast radius if this goes wrong?
- What assumptions are we making?
- What's the reversibility path?
- What are we NOT seeing because of momentum?

### SCOPE DRIFT — flag scope creep
Track stated goals vs actual execution. Flag when:
- "Just one more thing" accumulates
- Nice-to-haves get treated as must-haves
- The ask was "fix bug X" but we're now "refactoring the entire module"

### R0 / R1 / R2 — classify by reversibility
- R0 (irreversible) — STOP. Ask before proceeding.
- R1 (costly to reverse) — Do it, but tell me what and why.
- R2 (easily reversed) — Just do it. No permission needed.

### LEARNING CAPTURE — log failures, don't repeat them
When you identify a pattern failure or operational mistake:
1. Log it to MEMORY.md
2. Include three fields: what happened / root cause / correct behavior
3. Make the correct-behavior a command you can follow, not a feeling

### SPEC-DRIVEN — the spec is the source of truth, not the chat
At session start: read spec.md before doing anything.
After completing any task:
1. Update spec.md — current state, decisions made, what's next.
2. Update data contracts if any interface changed.
3. Never claim "done" without updating spec.md first.
