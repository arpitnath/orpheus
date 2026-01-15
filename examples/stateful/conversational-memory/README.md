# Conversational Therapist Agent

**Stateful therapeutic chatbot demonstrating workspace persistence across container restarts.**

## What This Demonstrates

This agent showcases **workspace persistence**: state stored in `/workspace` survives container restarts, making it possible to build truly stateful agents that remember context across sessions, crashes, and restarts.

### Demonstration

1. **Start conversation** → Therapist responds with empathy
2. **Continue conversation** → Therapist references previous message (session affinity)
3. **Kill the container** → Simulate crash/OOM
4. **Resume conversation** → **NEW worker, FULL context preserved!**

State persistence enables continuous conversation context across container lifecycles.

## Architecture

### Persistence
- **Storage**: SQLite database in `/workspace/patient_sessions.db`
- **Schema**:
  - `sessions` - Patient session metadata
  - `conversations` - Full conversation history with sentiment
  - `topics` - Extracted topics and frequency

### Intelligence
- **LLM**: Ollama/Mistral (via ServiceManager, no API keys)
- **Sentiment Analysis**: TextBlob (simple, no ML model)
- **Context Window**: Last 10 conversation exchanges
- **Session Affinity**: Routes same patient to same worker

### Key Features
- ✅ Conversation history survives container restarts
- ✅ Emotional trend tracking (positive/negative/neutral)
- ✅ Session-aware routing (X-Session-ID header)
- ✅ Zero external dependencies (fully self-contained)

## Prerequisites

1. **Orpheus CLI** installed
2. **Ollama** running with Mistral model:
   ```bash
   ollama pull mistral
   ```

## Quick Start

### 1. Deploy the Agent

```bash
cd examples/conversational-therapist-agent/
orpheus deploy .
```

### 2. Run the Interactive Demo

```bash
./demo.sh
```

The demo will:
1. Start a therapy session
2. Continue the conversation
3. Kill the container
4. Resume seamlessly (proves state survived)
5. Show progress report

## Manual Testing

### Send a Message

```bash
orpheus invoke conversational-therapist-agent '{
  "message": "I have been feeling really anxious about work lately"
}' -H "X-Session-ID: patient-alice"
```

**Response:**
```json
{
  "status": "success",
  "response": "I hear that work has been causing you anxiety...",
  "patient_sentiment": {
    "score": -0.35,
    "label": "negative"
  },
  "session_stats": {
    "total_interactions": 1,
    "avg_sentiment": -0.35,
    "emotional_trend": "stable"
  },
  "persistence_proof": "State stored in /workspace/patient_sessions.db (survives container restarts)"
}
```

### Continue Conversation (Session Affinity)

```bash
orpheus invoke conversational-therapist-agent '{
  "message": "My boss keeps giving me impossible deadlines"
}' -H "X-Session-ID: patient-alice"
```

**The therapist will reference your previous message** (loaded from SQLite).

### Kill the Container

```bash
# Find the container
limactl shell orpheus -- runc list | grep conversational-therapist

# Kill it
limactl shell orpheus -- runc kill <container-id> SIGKILL

# Check workspace persists
orpheus workspace info conversational-therapist-agent
# Should show: patient_sessions.db exists
```

### Resume After Kill

```bash
orpheus invoke conversational-therapist-agent '{
  "message": "Actually I talked to my boss and things are better now"
}' -H "X-Session-ID: patient-alice"
```

**Magic**: New worker spawns, loads SQLite, **has full conversation history**.

### View Progress Report

```bash
orpheus invoke conversational-therapist-agent '{
  "action": "show_progress"
}' -H "X-Session-ID: patient-alice"
```

**Response:**
```json
{
  "status": "success",
  "session_summary": {
    "session_id": "patient-alice",
    "total_interactions": 3,
    "avg_sentiment": -0.12,
    "emotional_trend": "improving"
  },
  "sentiment_progression": [
    {"message_num": 1, "sentiment": "negative"},
    {"message_num": 2, "sentiment": "negative"},
    {"message_num": 3, "sentiment": "positive"}
  ]
}
```

## Key Features

### Workspace Persistence
- **Persistent Storage** - `/workspace` survives container restarts
- **Session Affinity** - same user → same worker → hot cache
- **Local Storage** - SQLite file serves as the database
- **Fast Recovery** - new worker loads local file instantly

### Real-World Use Cases
This pattern enables:
- **Stateful AI agents** (conversational context, personalization)
- **Long-term memory** (user preferences, interaction history)
- **Progressive refinement** (learning from past interactions)
- **Session continuity** (survive crashes without losing state)

## Architecture Details

### Session Affinity (agent.yaml)
```yaml
session:
  enabled: true
  key: "X-Session-ID"  # Routes same patient to same worker
  ttl: "2h"            # Keep worker alive for 2 hours
  wait_timeout: "200ms"
```

### Workspace Persistence
```python
WORKSPACE_DIR = Path(os.getenv("WORKSPACE_DIR", "/workspace"))
DB_PATH = WORKSPACE_DIR / "patient_sessions.db"

# SQLite file stored in persistent volume
# Survives: container restarts, crashes, OOM kills
```

### Model Integration (ServiceManager)
```python
# Ollama endpoint injected by Orpheus ServiceManager
MODEL_URL = os.getenv("MODEL_URL") or "http://localhost:11434"
MODEL_NAME = os.getenv("OLLAMA_MODEL", "mistral")

# No API keys, no authentication, fully self-contained
```

## File Structure

```
conversational-therapist-agent/
├── agent.yaml           # Orpheus config (session affinity + model)
├── agent.py             # Main handler (SQLite + conversation logic)
├── requirements.txt     # textblob, requests
├── demo.sh              # Interactive demo script
└── README.md            # This file
```

## Implementation Highlights

**SQLite Schema** (200 lines total):
- Sessions table (metadata)
- Conversations table (history + sentiment)
- Topics table (extracted themes)

**Sentiment Analysis** (TextBlob):
- Simple polarity score (-1.0 to 1.0)
- Labels: positive, negative, neutral
- No ML model needed (fast, deterministic)

**Conversation Context**:
- Last 10 exchanges loaded from SQLite
- Passed to LLM prompt
- Therapist references previous topics

**Actions**:
- `chat` (default) - Normal conversation
- `show_progress` - Session statistics and trends

## Troubleshooting

### Agent won't deploy
```bash
# Check Ollama is running
ollama list

# Ensure Mistral is available
ollama pull mistral
```

### Container not found during kill
```bash
# List all containers
limactl shell orpheus -- runc list

# If container auto-restarted, that's fine - proceed with demo
```

### Workspace not persisting
```bash
# Check workspace configuration
orpheus workspace info conversational-therapist-agent

# Should show files in /workspace
```

## Extending This Agent

**Add more intelligence:**
- Topic extraction (spaCy NLP)
- Crisis detection (high negative sentiment → alert)
- Progress visualization (sentiment chart over time)
- Multi-modal input (voice transcription)

**Scale to production:**
- Add authentication (patient ID verification)
- HIPAA compliance (encrypt SQLite file)
- Backup automation (periodic snapshots)
- Analytics dashboard (aggregate trends)

## Learn More

- **Orpheus Docs**: [Session Affinity](https://docs.orpheus.ai/session-affinity)
- **Workspace Guide**: [Persistent Storage](https://docs.orpheus.ai/workspace)
- **ServiceManager**: [Model Integration](https://docs.orpheus.ai/service-manager)

## License

MIT License - This is a demo agent for educational purposes.

---

**Built with Orpheus** - Stateful agent architecture
