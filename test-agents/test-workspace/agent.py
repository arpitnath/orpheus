"""
Test Workspace Agent

Purpose: Test workspace persistence across multiple requests

Test Cases:
- action: "write" → Create files in workspace
- action: "read" → Read files from workspace
- action: "list" → List all files in workspace
- action: "append" → Append to existing file

Input format:
{
    "action": "write",      # write, read, list, append
    "filename": "test.txt", # optional, defaults to data.txt
    "content": "hello"      # for write/append actions
}
"""

import os
import json
from pathlib import Path

# Workspace directory - check environment variable or use /workspace
# Orpheus mounts workspace at /workspace (separate from /agent code directory)
WORKSPACE_DIR = Path(os.environ.get("ORPHEUS_WORKSPACE", "/workspace"))

def handler(input_data: dict) -> dict:
    """
    Handle workspace operations: write, read, list, append.

    Args:
        input_data: Dict with action, filename (optional), content (optional)

    Returns:
        Dict with status and result
    """
    action = input_data.get("action", "list")
    filename = input_data.get("filename", "data.txt")
    content = input_data.get("content", "")

    filepath = WORKSPACE_DIR / filename

    try:
        if action == "write":
            # Write content to file
            filepath.write_text(content)
            return {
                "status": "success",
                "action": "write",
                "filename": filename,
                "content": content,
                "message": f"Wrote {len(content)} characters to {filename}"
            }

        elif action == "read":
            # Read content from file
            if not filepath.exists():
                return {
                    "status": "error",
                    "action": "read",
                    "filename": filename,
                    "error": f"File {filename} does not exist"
                }

            file_content = filepath.read_text()
            return {
                "status": "success",
                "action": "read",
                "filename": filename,
                "content": file_content,
                "size": len(file_content)
            }

        elif action == "append":
            # Append to existing file
            with open(filepath, "a") as f:
                f.write(content)

            return {
                "status": "success",
                "action": "append",
                "filename": filename,
                "appended": content,
                "message": f"Appended {len(content)} characters to {filename}"
            }

        elif action == "list":
            # List all files in workspace
            files = []
            for item in WORKSPACE_DIR.iterdir():
                if item.is_file():
                    stat = item.stat()
                    files.append({
                        "name": item.name,
                        "size": stat.st_size,
                        "modified": stat.st_mtime
                    })

            return {
                "status": "success",
                "action": "list",
                "files": files,
                "count": len(files),
                "workspace": str(WORKSPACE_DIR)
            }

        else:
            return {
                "status": "error",
                "error": f"Unknown action: {action}",
                "valid_actions": ["write", "read", "list", "append"]
            }

    except Exception as e:
        return {
            "status": "error",
            "action": action,
            "error": str(e),
            "message": "Operation failed"
        }
