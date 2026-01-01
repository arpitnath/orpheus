## 📝 Phase Documentation Requirement

**CRITICAL:** After completing EACH phase, document what you did.

### **Documentation Directory:**
```
claude_ref_dev_docs/  (gitignored, local only)
```

### **After Each Phase, Create:**

**File:** `claude_ref_dev_docs/phase-{N}-{name}.md`

**Template:**
```markdown
# Phase {N}: {Phase Name}

**Date:** {date}
**Duration:** {time taken}
**Status:** Complete / Failed

## What Was Done

{List of changes made}

## Files Modified

{List with line counts}

## Commands Executed

{bash commands used}

## Testing Results

{What was tested, results}

## Issues Encountered

{Any problems and how they were solved}

## Next Phase

{What comes next}
```

**Example:**
```
claude_ref_dev_docs/
├── phase-1-module-rename.md
├── phase-2-directory-restructure.md
├── phase-3-binary-rename.md
├── phase-4-config-rename.md
└── phase-5-integration-test.md
```

**Why:** So future sessions can see exactly what happened in each phase.

---

## ⚠️ Working Rules

**1. Plan Mode First:**
- Use EnterPlanMode for restructuring plan
- Present complete plan to Arpit
- Wait for approval before executing

**2. No Commits Without Permission:**
- NEVER run `git commit` unless Arpit says "commit this"
- You can stage changes (`git add`) but don't commit

**3. Document Everything:**
- After each phase completes, write history doc
- Be specific: what changed, why, how to verify

**4. Test at Checkpoints:**
- After module rename: `go build ./...`
- After restructure: `go build ./daemon/...`
- After config: Integration test with agent deploy

---
