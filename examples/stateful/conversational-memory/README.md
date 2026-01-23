# Conversational Memory Agent

This agent demonstrates **stateful interactions** using both **Session Affinity** and **Workspace Persistence**. It remembers the chat history for a specific user.

## How it Works

1.  **Session Routing:** Orpheus routes requests with the same `--session` ID to the same worker if possible.
2.  **In-Memory Cache:** The worker keeps recent history in a Python dictionary for instant access.
3.  **Workspace Backup:** Every message is also backed up to a JSON file in `/workspace/sessions/`.
    *   If the worker restarts, it reloads history from the workspace.
    *   This guarantees durability even if the node crashes.

## Key Features

*   **Session ID:** You pass a `session_id` (or use the CLI `--session` flag).
*   **Dual-Layer State:** Fast in-memory access + durable disk storage.
*   **Context Window:** The agent "remembers" what you said in previous turns.

## Usage

```bash
# Start a conversation
orpheus run conversational-memory '{"message": "My name is Alice"}' --session user-123

# Follow up (it remembers!)
orpheus run conversational-memory '{"message": "What is my name?"}' --session user-123
```
