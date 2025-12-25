"""Archive utilities for agent image packaging."""

import hashlib
import tarfile
from pathlib import Path
from typing import Optional


def create_tar(directory: Path, output_path: Optional[Path] = None) -> Path:
    """Create tar.gz archive of directory.

    Args:
        directory: Directory to archive
        output_path: Optional output path (defaults to /tmp/{name}.tar.gz)

    Returns:
        Path to created tar file
    """
    if not directory.exists() or not directory.is_dir():
        raise ValueError(f"Directory not found: {directory}")

    # Default output path
    if output_path is None:
        output_path = Path(f"/tmp/{directory.name}.tar.gz")

    # Create compressed tar
    with tarfile.open(output_path, "w:gz") as tar:
        # Add directory with relative paths (arcname removes parent path)
        tar.add(directory, arcname=directory.name, recursive=True)

    return output_path


def calculate_checksum(file_path: Path) -> str:
    """Calculate SHA256 checksum of file.

    Args:
        file_path: File to checksum

    Returns:
        Hex-encoded SHA256 hash
    """
    if not file_path.exists():
        raise ValueError(f"File not found: {file_path}")

    sha256 = hashlib.sha256()

    # Read in chunks to handle large files
    with open(file_path, 'rb') as f:
        for chunk in iter(lambda: f.read(8192), b''):
            sha256.update(chunk)

    return sha256.hexdigest()


def validate_tar(file_path: Path) -> bool:
    """Validate that file is a valid tar.gz archive.

    Args:
        file_path: Tar file to validate

    Returns:
        True if valid tar.gz
    """
    if not file_path.exists():
        return False

    try:
        with tarfile.open(file_path, "r:gz") as tar:
            # Try to list members (will fail if corrupt)
            tar.getmembers()
        return True
    except Exception:
        return False


def get_tar_size_mb(file_path: Path) -> int:
    """Get tar file size in megabytes.

    Args:
        file_path: Tar file

    Returns:
        Size in MB
    """
    if not file_path.exists():
        return 0

    size_bytes = file_path.stat().st_size
    return size_bytes // (1024 * 1024)
