#!/usr/bin/env bash
set -e

TASK_FILE="TASKS.md"

if [ ! -f "$TASK_FILE" ]; then
  echo "Error: $TASK_FILE not found! Create a TASKS.md file listing your tasks."
  exit 1
fi

echo "====================================================="
echo " Starting Autonomous Muse Code Execution Loop"
echo "====================================================="

while true; do
  # Find the first unchecked task (- [ ])
  NEXT_TASK=$(grep -m 1 '^- \[ \]' "$TASK_FILE" | sed 's/^- \[ \] //')

  if [ -z "$NEXT_TASK" ]; then
    echo "All tasks in $TASK_FILE are complete!"
    break
  fi

  echo ""
  echo ">>> Active Task: $NEXT_TASK"

  PROMPT="You are working on DocFlow AI.
Refer strictly to AGENTS.md for architecture, database schemas, API specs, and coding rules.
Task: $NEXT_TASK

Instructions:
1. Implement or modify the necessary Go code and SQL files.
2. Write corresponding unit or integration tests adhering to AGENTS.md Section 9.
3. Run tests using 'go test -v ./...' inside the repository to ensure everything passes and compile errors are resolved.
4. When finished and tests pass, exit."

  echo ">>> Launching Muse Code..."
  # Execute headless with auto-approvals
  muse exec --yolo "$PROMPT"

  # Independent ground-truth verification
  echo ">>> Verifying tests independently..."
  if go test ./...; then
    echo ">>> Tests passed! Staging and committing changes..."
    git add .
    git commit -m "feat(auto): $NEXT_TASK" || echo "No changes to commit."
    
    # Mark task as completed in TASKS.md (- [x])
    # macOS-compatible sed in-place replacement
    sed -i '' "s/^- \[ \] $(echo "$NEXT_TASK" | sed 's/[][\/.^$*]/\\&/g')/- [x] $NEXT_TASK/" "$TASK_FILE"
    echo ">>> Task marked complete in $TASK_FILE."
  else
    echo ">>> Tests failed! Feeding errors back to Muse Code for automated repair..."
    FAIL_LOG=$(go test -v ./... 2>&1 | tail -n 30)
    REPAIR_PROMPT="The implementation for '$NEXT_TASK' caused test/build failures:
$FAIL_LOG

Refer to AGENTS.md and fix the compiler/test failures. Ensure 'go test ./...' passes."
    muse exec --yolo "$REPAIR_PROMPT"
  fi

  sleep 2
done

echo "====================================================="
echo " Autonomous run finished successfully."
echo "====================================================="
