# Gmail Automation Agent

An AI agent that automatically labels Gmail messages using OpenAI Agents SDK and Composio.

## 🌟 Features

### Core Capabilities
- 🤖 **AI-Powered Labeling**: Intelligent email categorization using GPT-4o-mini
- 🧠 **Stateful Memory**: Remembers past labeling decisions for consistency
- 🔐 **Automatic Authentication**: Handles Gmail OAuth flow automatically
- 📊 **Session Persistence**: SQLite-based conversation history
- ⚡ **Multi-User Support**: Separate sessions per user
- 🔄 **Error Recovery**: Graceful handling of auth failures and retries

### Label Categories
- **Work**: Professional emails, meeting invites, project discussions
- **Personal**: Friends, family, personal matters
- **Finance**: Bills, invoices, banking, payments
- **Social**: Social media notifications, newsletters
- **Shopping**: Order confirmations, shipping updates, retail

## 📋 Prerequisites

- Python 3.8+
- Gmail account
- [Composio API key](https://app.composio.dev)
- [OpenAI API key](https://platform.openai.com)

## 🚀 Quick Start

### 1. Install Dependencies

```bash
pip install -r requirements.txt
```

### 2. Set Environment Variables

```bash
# Copy example env file
cp .env.example .env

# Edit .env with your API keys
export COMPOSIO_API_KEY=your_composio_api_key
export OPENAI_API_KEY=sk-proj-your_openai_api_key
```

### 3. Test Locally

```bash
python test_local.py
```

On first run, the agent will:
1. Check for Gmail connection
2. If not connected, display OAuth URL
3. Wait for you to authenticate (opens in browser)
4. Continue automatically once authenticated
5. List your Gmail labels

### 4. Deploy to Orpheus

```bash
cd examples/gmail-automation
orpheus deploy .
```

## 🏗️ Architecture

### Technology Stack

- **OpenAI Agents SDK**: Memory and state management
- **Composio**: Gmail OAuth and API integration
- **SQLite**: Session persistence
- **LangChain**: Composio provider integration

### System Flow

```
┌─────────────────────────────────────────────────────────┐
│                     User Request                         │
│  {"action": "label_email", "user_id": "user_123"}       │
└──────────────────────┬──────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────┐
│                  Handler (src/handler.py)                │
│  - Parse input                                           │
│  - Initialize GmailLabelerAgent                          │
│  - Execute action                                        │
└──────────────────────┬──────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────┐
│         GmailLabelerAgent (src/agent/gmail_labeler.py)  │
│  ┌────────────────────────────────────────────────┐     │
│  │  Session Management                            │     │
│  │  - SQLiteSession (./data/gmail_sessions.db)    │     │
│  │  - Per-user conversation context               │     │
│  │  - Automatic history management                │     │
│  └────────────────────────────────────────────────┘     │
│                                                          │
│  ┌────────────────────────────────────────────────┐     │
│  │  OpenAI Agent                                   │     │
│  │  - Model: gpt-4o-mini                          │     │
│  │  - Instructions: Labeling guidelines           │     │
│  │  - Tools: Composio Gmail tools                 │     │
│  └────────────────────────────────────────────────┘     │
└──────────────────────┬──────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────┐
│    AuthAwareComposioTools (src/tools/composio_wrapper.py)│
│  ┌────────────────────────────────────────────────┐     │
│  │  Pre-flight Auth Check                         │     │
│  │  - Check connection on startup                 │     │
│  │  - Initiate OAuth if needed                    │     │
│  │  - Wait for completion (2min timeout)          │     │
│  └────────────────────────────────────────────────┘     │
└──────────────────────┬──────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────┐
│                 Composio Gmail Tools                     │
│  - GMAIL_LIST_LABELS                                    │
│  - GMAIL_CREATE_LABEL                                   │
│  - GMAIL_ADD_LABEL_TO_EMAIL                             │
│  - GMAIL_GET_MESSAGE                                    │
│  - GMAIL_LIST_MESSAGES                                  │
└─────────────────────────────────────────────────────────┘
```

### File Structure

```
gmail-automation/
├── agent.yaml              # Orpheus deployment config
├── requirements.txt        # Pinned Python dependencies
├── .env.example           # Environment variable template
├── .gitignore             # Ignore sensitive data
│
├── src/
│   ├── handler.py         # Orpheus entry point
│   │
│   ├── agent/
│   │   └── gmail_labeler.py    # Main agent with sessions
│   │
│   ├── auth/
│   │   ├── manager.py          # OAuth management
│   │   └── exceptions.py       # Custom exceptions
│   │
│   └── tools/
│       └── composio_wrapper.py # Auth-aware tools
│
├── data/
│   └── gmail_sessions.db  # SQLite session storage (auto-created)
│
├── connect_gmail.py       # Manual auth script (optional)
├── test_local.py          # Local testing
└── README.md
```

## 📖 Usage

### Actions

The agent supports three main actions:

#### 1. List Labels

List all available Gmail labels.

```python
result = handler({
    "action": "list_labels",
    "user_id": "user_123"
})
```

**Response:**
```json
{
    "status": "success",
    "action": "list_labels",
    "result": "Here are the available Gmail labels:\n\n### System Labels\n- INBOX\n- SENT\n..."
}
```

#### 2. Label Email

Analyze and label a specific email.

```python
result = handler({
    "action": "label_email",
    "user_id": "user_123",
    "message_id": "18abc123def",
    "subject": "Q4 Sales Report",
    "content": "Attached is the quarterly sales report..."
})
```

**Response:**
```json
{
    "status": "success",
    "action": "label_email",
    "message_id": "18abc123def",
    "result": "I've analyzed this email and applied the 'Work' label because..."
}
```

#### 3. Process Inbox

Process recent emails and apply labels automatically.

```python
result = handler({
    "action": "process_inbox",
    "user_id": "user_123",
    "max_emails": 5
})
```

**Response:**
```json
{
    "status": "success",
    "action": "process_inbox",
    "result": "I've processed 5 emails:\n1. Email from john@company.com - Applied 'Work' label\n..."
}
```

### Using the Agent (After Deployment)

#### List Labels
```bash
orpheus invoke gmail-automation '{
  "action": "list_labels",
  "user_id": "user_123"
}'
```

#### Label Email
```bash
orpheus invoke gmail-automation '{
  "action": "label_email",
  "user_id": "user_123",
  "message_id": "18abc123def",
  "subject": "Important Meeting",
  "content": "Please join the team meeting tomorrow at 10 AM"
}'
```

#### Process Inbox
```bash
orpheus invoke gmail-automation '{
  "action": "process_inbox",
  "user_id": "user_123",
  "max_emails": 10
}'
```

## 🧠 Memory & Sessions

### How It Works

The agent uses **SQLiteSession** from OpenAI Agents SDK to persist conversation history:

- **Session ID**: Unique per user (`gmail_{user_id}`)
- **Storage**: `./data/gmail_sessions.db`
- **Retention**: Permanent (until manually cleared)
- **Scope**: Per-user isolation

### Benefits

1. **Consistency**: Agent remembers previous labeling decisions
2. **Learning**: Improves labeling accuracy over time
3. **Context**: Understands user preferences
4. **Multi-user**: Separate memory for each user

### Example

```python
# First request
handler({"action": "label_email", "user_id": "alice", ...})
# Agent applies "Work" label to sales report

# Later request (same user)
handler({"action": "label_email", "user_id": "alice", ...})
# Agent remembers to use "Work" for similar emails
```

## 🔐 Authentication

### Automatic OAuth Flow

The agent handles Gmail authentication automatically:

1. **First Run**: Checks for existing connection
2. **If Not Connected**:
   - Displays OAuth URL
   - Waits for user to authenticate
   - Continues automatically once complete
3. **Subsequent Runs**: Uses cached connection (zero overhead)

### Manual Authentication (Optional)

If you prefer to authenticate manually:

```bash
python connect_gmail.py
```

This script:
- Checks for existing connection
- Initiates OAuth if needed
- Saves connection for agent to use

### Connection Status

The agent caches connection status for 5 minutes to reduce API calls.
Force refresh by re-initializing the agent.

## 🛠️ Development

### Local Testing

```bash
# Run basic test
python test_local.py

# Test specific actions (in Python)
from src.handler import handler

# List labels
result = handler({"action": "list_labels", "user_id": "test_user"})

# Label email
result = handler({
    "action": "label_email",
    "user_id": "test_user",
    "message_id": "msg_123",
    "subject": "Test",
    "content": "This is a test email"
})
```

### Environment Setup

```bash
# Create virtual environment
python -m venv venv
source venv/bin/activate  # On Windows: venv\Scripts\activate

# Install dependencies
pip install -r requirements.txt

# Set environment variables
export COMPOSIO_API_KEY=your_key
export OPENAI_API_KEY=your_key
```

### Debugging

Enable debug logging:

```python
import logging
logging.basicConfig(level=logging.DEBUG)
```

## 📊 Session Management

### View Session Data

```bash
# Install SQLite browser or use CLI
sqlite3 ./data/gmail_sessions.db

# List sessions
.tables

# View session data
SELECT * FROM sessions;
```

### Clear Session

```python
from src.agent.gmail_labeler import GmailLabelerAgent
import asyncio

async def clear():
    agent = GmailLabelerAgent(user_id="user_123")
    await agent.initialize()
    await agent.clear_session()

asyncio.run(clear())
```

## 🚨 Troubleshooting

### Authentication Issues

**Problem**: "No active connection found"

**Solution**:
1. Run `python connect_gmail.py` manually
2. Complete OAuth flow in browser
3. Try again

### API Key Errors

**Problem**: "COMPOSIO_API_KEY not set"

**Solution**:
```bash
export COMPOSIO_API_KEY=your_key
export OPENAI_API_KEY=your_key
```

### Session Database Issues

**Problem**: Database locked or corrupted

**Solution**:
```bash
rm ./data/gmail_sessions.db
# Will be recreated on next run
```

### Import Errors

**Problem**: `ModuleNotFoundError`

**Solution**:
```bash
pip install -r requirements.txt
```

## 🔒 Security

- **API Keys**: Never commit `.env` files (in `.gitignore`)
- **Session Data**: Contains conversation history - treat as sensitive
- **OAuth Tokens**: Managed by Composio, never exposed
- **Database**: SQLite file contains session data - backup regularly

## 📝 License

MIT

## 🔗 Resources

- [OpenAI Agents SDK](https://openai.github.io/openai-agents-python/)
- [Composio Documentation](https://docs.composio.dev)
- [Gmail API](https://developers.google.com/gmail/api)
