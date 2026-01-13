"""
Test Memory OOM Agent

Purpose: Test OOM handling, exit code 137 detection, backoff (Issue #5 fix validation)

Test Cases:
- allocate_mb: 200 → SUCCESS (within 256MB limit)
- allocate_mb: 400 → OOM KILLED (exit 137)
- After OOM → Verify 60s backoff delay (Issue #5 fix)
- Trigger 3 OOMs → Worker health → unhealthy → auto-replace

Input format:
{
    "allocate_mb": 200  # Amount of memory to allocate in MB
}
"""

def handler(input_data: dict) -> dict:
    """
    Allocates specified amount of memory to trigger OOM if over limit.

    Args:
        input_data: Dict with allocate_mb

    Returns:
        Dict with status and allocated amount (if succeeds)
    """
    allocate_mb = input_data.get("allocate_mb", 100)

    if allocate_mb <= 0:
        return {
            "error": "allocate_mb must be positive",
            "status": "error"
        }

    print(f"Starting memory allocation: {allocate_mb}MB")

    # Allocate requested memory (1MB chunks)
    data = []
    chunk_size = 1024 * 1024  # 1MB

    try:
        for i in range(allocate_mb):
            # Allocate 1MB chunk (list of bytes)
            chunk = bytearray(chunk_size)
            # Write to ensure it's actually allocated (not lazy)
            chunk[0] = 1
            chunk[len(chunk)-1] = 1
            data.append(chunk)

            if i % 50 == 0 and i > 0:
                print(f"Allocated {i}MB so far...")

        print(f"Successfully allocated {allocate_mb}MB")

        return {
            "status": "completed",
            "allocated_mb": allocate_mb,
            "message": f"Successfully allocated {allocate_mb}MB in {len(data)} chunks"
        }

    except MemoryError:
        # Python MemoryError (before OOM killer)
        return {
            "status": "error",
            "error": "MemoryError before OOM killer",
            "allocated_mb": len(data),
            "message": "Python raised MemoryError before container OOM"
        }
