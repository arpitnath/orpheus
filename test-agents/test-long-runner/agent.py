"""
Test Long-Runner Agent

Purpose: Test timeout management, activity monitoring, idle detection

Test Cases:
- Short run (60s): Completes successfully
- Medium run (300s): Completes with periodic activity
- Long run (700s): Times out at 600s max
- Idle test: Times out after 120s of inactivity

Input format:
{
    "duration_sec": 60,        # Total duration to run
    "activity_interval": 10    # How often to emit progress
}
"""

import time

def handler(input_data: dict) -> dict:
    """
    Runs for specified duration, emitting progress periodically.

    Args:
        input_data: Dict with duration_sec and activity_interval

    Returns:
        Dict with status, duration, and checkpoint count
    """
    duration_sec = input_data.get("duration_sec", 60)
    activity_interval = input_data.get("activity_interval", 10)

    if duration_sec <= 0 or activity_interval <= 0:
        return {
            "error": "duration_sec and activity_interval must be positive",
            "status": "error"
        }

    results = []
    elapsed = 0

    print(f"Starting long-runner: {duration_sec}s total, {activity_interval}s intervals")

    while elapsed < duration_sec:
        # Sleep for activity interval
        time.sleep(activity_interval)
        elapsed += activity_interval

        # Emit progress (keeps activity monitor happy, prevents idle timeout)
        checkpoint = f"checkpoint_{elapsed}"
        print(f"Progress: {elapsed}/{duration_sec}s - {checkpoint}")
        results.append(checkpoint)

    print(f"Completed after {elapsed}s with {len(results)} checkpoints")

    return {
        "status": "completed",
        "duration_sec": elapsed,
        "checkpoints": len(results),
        "checkpoint_list": results[:10]  # First 10 only
    }
