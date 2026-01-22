#!/usr/bin/env python3
"""
Local test script for Gmail Labeler Agent.

Tests the agent locally before deploying to Orpheus.
"""

import os
import sys

# Import handler from agent.py
from agent import handler


def test_list_labels():
    """Test listing Gmail labels"""
    print("=" * 60)
    print("Test: List Gmail Labels")
    print("=" * 60)

    # Check environment variables
    if not os.environ.get("COMPOSIO_API_KEY"):
        print("❌ Error: COMPOSIO_API_KEY not set")
        print("Run: export COMPOSIO_API_KEY=your-key")
        return False

    if not os.environ.get("OPENAI_API_KEY"):
        print("❌ Error: OPENAI_API_KEY not set")
        print("Run: export OPENAI_API_KEY=sk-proj-...")
        return False

    # Call handler
    result = handler({
        "action": "list_labels",
        "user_id": "arpit-gmail-user"
    })

    # Display result
    print(f"\nStatus: {result.get('status')}")
    if result.get('status') == 'success':
        print(f"\nResult:\n{result.get('result', 'No result')}")
        return True
    else:
        print(f"\nError: {result.get('error', result.get('message', 'Unknown error'))}")
        return False


def test_label_email():
    """Test labeling a specific email"""
    print("\n" + "=" * 60)
    print("Test: Label Email")
    print("=" * 60)

    result = handler({
        "action": "label_email",
        "user_id": "arpit-gmail-user",
        "message_id": "test_msg_123",
        "subject": "Q4 Sales Report",
        "content": "Attached is the quarterly sales report for review. Please review the revenue metrics and provide feedback by EOD."
    })

    print(f"\nStatus: {result.get('status')}")
    if result.get('status') == 'success':
        print(f"\nResult:\n{result.get('result', 'No result')}")
        return True
    else:
        print(f"\nError: {result.get('error', result.get('message', 'Unknown error'))}")
        return False


def test_process_inbox():
    """Test processing inbox"""
    print("\n" + "=" * 60)
    print("Test: Process Inbox")
    print("=" * 60)

    result = handler({
        "action": "process_inbox",
        "user_id": "arpit-gmail-user",
        "max_emails": 3
    })

    print(f"\nStatus: {result.get('status')}")
    if result.get('status') == 'success':
        print(f"\nResult:\n{result.get('result', 'No result')}")
        return True
    else:
        print(f"\nError: {result.get('error', result.get('message', 'Unknown error'))}")
        return False


def main():
    """Run all tests"""
    print("\n🤖 Gmail Labeler Agent - Local Testing\n")

    # Test 1: List labels (basic test)
    success1 = test_list_labels()

    if not success1:
        print("\n❌ Basic test failed. Fix authentication before continuing.")
        return 1

    # Optional: Test other actions
    print("\n" + "=" * 60)
    print("Additional tests available:")
    print("  - test_label_email()")
    print("  - test_process_inbox()")
    print("=" * 60)

    print("\n✅ Basic test passed! Agent is working.")
    print("\nNext steps:")
    print("  1. Test other actions if needed")
    print("  2. Deploy to Orpheus: orpheus deploy .")
    print("  3. Test via API endpoints")

    return 0


if __name__ == "__main__":
    sys.exit(main())
