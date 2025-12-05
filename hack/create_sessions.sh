#!/bin/bash

SESSION_NAME="raft_cluster"

# Array of commands to run, separated by '|' for easy iteration
declare -a COMMANDS=(
  "go run . server --port='5890' --store-path='./dist/store-0.gob' --id='0' --ids '1,2,3' --addresses=':5891,:5892,:5893' | tee dist/logs-0.log"
  "go run . server --port='5891' --store-path='./dist/store-1.gob' --id='1' --ids '0,2,3' --addresses=':5890,:5892,:5893' | tee dist/logs-1.log"
  "go run . server --port='5892' --store-path='./dist/store-2.gob' --id='2' --ids '0,1,3' --addresses=':5890,:5891,:5893' | tee dist/logs-2.log"
  "go run . server --port='5893' --store-path='./dist/store-3.gob' --id='3' --ids '0,1,2' --addresses=':5890,:5891,:5892' | tee dist/logs-3.log"
)

# --- 1. Create a new session and run the first command ---
# -d: Starts the session but doesn't attach to it (runs in the background)
# -s: Specifies the session name
tmux new-session -d -s $SESSION_NAME "${COMMANDS[0]}"

# --- 2. Create the remaining four panes in the window ---
# Use a loop starting from the second command (index 1)
for i in {1..3}; do
  # split-window: Creates a new pane in the current window
  # -t: Specifies the target window (in this case, the only window in the session)
  # -h/-v: Split horizontally or vertically (tmux handles the grid layout automatically)
  tmux split-window -t $SESSION_NAME:0 "${COMMANDS[$i]}"
done

# --- 3. Arrange panes in a grid layout ---
# select-layout: Arranges the panes. 'tiled' works well for 5 panes.
# For 5, it usually creates a 3x2 or similar, maximizing space.
tmux select-layout -t $SESSION_NAME:0 tiled

# --- 4. Attach to the new session ---
echo "Raft cluster started in tmux session '$SESSION_NAME'."
echo "To attach: tmux attach -t $SESSION_NAME"
tmux attach-session -t $SESSION_NAME
