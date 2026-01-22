"""
Gmail Labeler Agent - Automatically labels Gmail messages using Composio + LangChain.

This agent can:
- List Gmail labels
- Create new labels
- Add labels to emails
- Process emails and auto-label based on content
"""

import os
from typing import Any, Dict

from composio import Composio
from composio_langchain import LangchainProvider

from langchain.agents import create_agent
from langchain_openai import ChatOpenAI


# Prompt for labeling emails
LABEL_EMAIL_PROMPT = """
You are an email labeling assistant. Your job is to analyze emails and apply appropriate labels.

Given the following email:
- Message ID: {message_id}
- Subject: {subject}
- Content: {content}

Analyze the email content and:
1. First, list the available Gmail labels using GMAIL_LIST_LABELS
2. Determine the most appropriate label based on the email content:
   - If it's about work/business, use or create a "Work" label
   - If it's about personal matters, use or create a "Personal" label
   - If it's a newsletter/marketing, use or create a "Newsletter" label
   - If it's urgent/important, use or create an "Urgent" label
   - If it's a receipt/order, use or create a "Receipts" label
3. If the appropriate label doesn't exist, create it using GMAIL_CREATE_LABEL
4. Apply the label to the email using GMAIL_ADD_LABEL_TO_EMAIL

Provide a brief explanation of why you chose that label.
"""

# Prompt for listing labels
LIST_LABELS_PROMPT = """
List all available Gmail labels for this account using GMAIL_LIST_LABELS.
Format the output nicely showing label names and IDs.
"""


def create_gmail_agent(composio_client: Composio, user_id: str):
    """
    Create a Gmail labeling agent with Composio tools.
    """
    # Get Gmail tools from Composio
    tools = composio_client.tools.get(
        user_id=user_id,
        tools=[
            "GMAIL_LIST_LABELS",
            "GMAIL_ADD_LABEL_TO_EMAIL",
            "GMAIL_CREATE_LABEL",
            "GMAIL_FETCH_MESSAGE_BY_MESSAGE_ID",
            "GMAIL_FETCH_EMAILS",
        ],
    )

    # Initialize OpenAI chat model
    llm = ChatOpenAI(model="gpt-4o-mini", temperature=0)

    # Create the agent using LangChain 1.2+ API
    agent = create_agent(
        model=llm,
        tools=tools,
        system_prompt="You are a helpful Gmail assistant that can manage email labels.",
        name="GmailLabeler"
    )

    return agent


def handler(input_data: Dict[str, Any]) -> Dict[str, Any]:
    """
    Orpheus handler for Gmail automation agent.

    Input format:
    {
        "action": "label_email" | "list_labels" | "process_inbox",
        "user_id": "default",  # Composio user ID
        "message_id": "...",   # For label_email action
        "subject": "...",      # For label_email action
        "content": "..."       # For label_email action
    }
    """
    try:
        action = input_data.get("action", "list_labels")
        user_id = input_data.get("user_id", "default")

        # Get API key from environment
        composio_api_key = os.environ.get("COMPOSIO_API_KEY")
        if not composio_api_key:
            return {
                "status": "error",
                "message": "COMPOSIO_API_KEY not set in environment"
            }

        # Initialize Composio client with LangChain provider
        composio_client = Composio(
            api_key=composio_api_key,
            provider=LangchainProvider()
        )

        # Create the agent
        agent = create_gmail_agent(composio_client=composio_client, user_id=user_id)

        # Execute based on action
        if action == "list_labels":
            result = agent.invoke({"messages": [("user", LIST_LABELS_PROMPT)]})
            # Extract the final message content
            output = result.get("messages", [])[-1].content if result.get("messages") else str(result)
            return {
                "status": "success",
                "action": "list_labels",
                "result": output
            }

        elif action == "label_email":
            message_id = input_data.get("message_id")
            subject = input_data.get("subject", "No subject")
            content = input_data.get("content", "No content")

            if not message_id:
                return {
                    "status": "error",
                    "message": "message_id is required for label_email action"
                }

            prompt = LABEL_EMAIL_PROMPT.format(
                message_id=message_id,
                subject=subject,
                content=content
            )

            result = agent.invoke({"messages": [("user", prompt)]})
            output = result.get("messages", [])[-1].content if result.get("messages") else str(result)
            return {
                "status": "success",
                "action": "label_email",
                "message_id": message_id,
                "result": output
            }

        elif action == "process_inbox":
            # Get max_emails from input (default 10)
            max_emails = input_data.get("max_emails", 10)

            # Two-step workflow: get IDs first, then fetch selectively
            process_prompt = f"""
            STEP 1: Get Message IDs (Ultra-Lightweight)
            Use GMAIL_FETCH_EMAILS with these parameters:
            - max_results: {max_emails}
            - ids_only: True (returns ONLY message IDs, no content at all)
            This will give you a list of message IDs.

            STEP 2: Fetch Basic Info (Subject + Sender)
            For the first 5 message IDs from Step 1, use GMAIL_FETCH_MESSAGE_BY_MESSAGE_ID with format="metadata"
            This gives you subject and sender without full email body.

            STEP 3: Select Important Emails
            Based on subjects and senders, pick 2-3 emails that likely need custom labeling.
            (Skip obvious spam/newsletters that can use system categories)

            STEP 4: Fetch Full Content (Only for Selected)
            For the 2-3 selected emails, use GMAIL_FETCH_MESSAGE_BY_MESSAGE_ID with format="full"
            to get the complete email body.

            STEP 5: Suggest Labels
            For each email you fully analyzed:
            - Suggest the most appropriate label (Work, Personal, Newsletter, Urgent, Receipts)
            - Explain your reasoning

            Provide a clear summary:
            - Total message IDs retrieved: {max_emails}
            - Emails with basic info checked: 5
            - Emails fully analyzed: 2-3
            - Suggested label for each analyzed email

            DO NOT actually apply labels - just provide recommendations.
            """
            result = agent.invoke({"messages": [("user", process_prompt)]})
            output = result.get("messages", [])[-1].content if result.get("messages") else str(result)
            return {
                "status": "success",
                "action": "process_inbox",
                "result": output
            }

        else:
            return {
                "status": "error",
                "message": f"Unknown action: {action}. Valid actions: list_labels, label_email, process_inbox"
            }

    except Exception as e:
        return {
            "status": "error",
            "error": str(e),
            "agent": "gmail-labeler"
        }
