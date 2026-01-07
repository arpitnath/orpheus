"""
File tools for Deep Research Agent.

Provides report generation and file management in the workspace.
"""

import os
import json
from datetime import datetime
from agents import function_tool

# Workspace directory for reports
REPORTS_DIR = "./reports"


@function_tool
def write_report(title: str, content: str) -> str:
    """
    Write a research report to the workspace for future reference.

    Args:
        title: Report title (will be used in filename)
        content: Full report content in markdown format

    Returns:
        Path to the saved report file
    """
    try:
        os.makedirs(REPORTS_DIR, exist_ok=True)

        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        safe_title = "".join(c if c.isalnum() or c in " -_" else "_" for c in title)[:50]
        safe_title = safe_title.replace(" ", "_")
        filename = f"{timestamp}_{safe_title}.md"
        filepath = os.path.join(REPORTS_DIR, filename)

        # Add metadata header
        report_content = f"""# {title}

**Generated**: {datetime.now().strftime("%Y-%m-%d %H:%M:%S")}
**Agent**: Deep Research Agent

---

{content}
"""

        with open(filepath, "w") as f:
            f.write(report_content)

        return json.dumps({
            "status": "success",
            "message": f"Report saved successfully",
            "filepath": filepath,
            "filename": filename
        })
    except Exception as e:
        return json.dumps({"status": "error", "message": str(e)})


@function_tool
def list_past_reports(topic_filter: str = "") -> str:
    """
    List all past research reports, optionally filtered by topic.

    Args:
        topic_filter: Optional keyword to filter reports by (searches filename)

    Returns:
        List of available report files
    """
    try:
        if not os.path.exists(REPORTS_DIR):
            return json.dumps({"status": "success", "reports": [], "message": "No reports directory yet"})

        reports = []
        for filename in sorted(os.listdir(REPORTS_DIR), reverse=True):
            if filename.endswith(".md"):
                if not topic_filter or topic_filter.lower() in filename.lower():
                    filepath = os.path.join(REPORTS_DIR, filename)
                    stat = os.stat(filepath)
                    reports.append({
                        "filename": filename,
                        "size_kb": round(stat.st_size / 1024, 2),
                        "created": datetime.fromtimestamp(stat.st_ctime).isoformat()
                    })

        return json.dumps({
            "status": "success",
            "reports": reports[:20],  # Limit to 20 most recent
            "total": len(reports)
        })
    except Exception as e:
        return json.dumps({"status": "error", "reports": [], "message": str(e)})


@function_tool
def read_report(filename: str) -> str:
    """
    Read the content of a past research report.

    Args:
        filename: The filename of the report to read

    Returns:
        The full content of the report
    """
    try:
        filepath = os.path.join(REPORTS_DIR, filename)
        if not os.path.exists(filepath):
            return json.dumps({"status": "error", "message": f"Report not found: {filename}"})

        with open(filepath, "r") as f:
            content = f.read()

        return json.dumps({
            "status": "success",
            "filename": filename,
            "content": content[:10000]  # Limit to 10k chars
        })
    except Exception as e:
        return json.dumps({"status": "error", "message": str(e)})
