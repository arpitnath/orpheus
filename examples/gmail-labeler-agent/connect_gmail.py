#!/usr/bin/env python3
"""
Connect Gmail to Composio programmatically.
Checks for existing connection first, only creates new if needed.
"""

import os
import sys

if not os.environ.get("COMPOSIO_API_KEY"):
    print("Error: COMPOSIO_API_KEY not set")
    print("Run: export COMPOSIO_API_KEY=your-key")
    sys.exit(1)

from composio import Composio
from composio_langchain import LangchainProvider

# Initialize Composio
composio = Composio(
    api_key=os.environ.get("COMPOSIO_API_KEY"),
    provider=LangchainProvider()
)

print("=" * 50)
print("Gmail Connection Setup")
print("=" * 50)

# Your Gmail auth config ID
GMAIL_AUTH_CONFIG_ID = "ac_zQq26WDiUZYI"
USER_ID = "arpit-gmail-user"

# Step 1: Check for existing connections
print(f"\n1. Checking for existing Gmail connections...")
print(f"   User ID: {USER_ID}")

existing_connections = composio.connected_accounts.list(
    user_ids=[USER_ID],
    toolkit_slugs=["GMAIL"],
)

active_connection = None
for conn in existing_connections.items:
    print(f"   Found: {conn.id} (Status: {conn.status})")
    if conn.status.upper() == "ACTIVE":
        active_connection = conn
        break

if active_connection:
    print(f"\n✅ Active Gmail connection already exists!")
    print(f"   Connected Account ID: {active_connection.id}")
    print(f"   User ID: {active_connection.user_id}")
    print(f"\nYou can now run: python test_local.py")
    sys.exit(0)

# Step 2: No active connection, initiate new one
print("\n2. No active connection found. Initiating OAuth...")

connection_request = composio.connected_accounts.initiate(
    user_id=USER_ID,
    auth_config_id=GMAIL_AUTH_CONFIG_ID,
    allow_multiple=True,  # Allow retry scenarios
)

print(f"\n🔗 Please authorize Gmail by visiting this URL:\n")
print(f"   {connection_request.redirect_url}")
print(f"\n⏳ Waiting for authorization (5 min timeout)...")

# Wait for user to complete OAuth
try:
    connected_account = connection_request.wait_for_connection(timeout=300)
    print(f"\n✅ Gmail connected successfully!")
    print(f"   Connected Account ID: {connected_account.id}")
    print(f"   User ID: {connected_account.user_id}")
    print(f"\nNow run: python test_local.py")
except Exception as e:
    print(f"\n❌ Connection failed or timed out: {e}")
    print("Try again or connect via: https://platform.composio.dev")
