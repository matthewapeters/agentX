"""ToolSpec dataclass for representing tool specifications."""

from __future__ import annotations

from dataclasses import dataclass
from typing import Dict, Optional


@dataclass
class ToolSpec:
    """
    Docstring for ToolSpec
    """

    name: str
    description: Optional[str]
    docstring: Optional[str]
    parameters_schema: Dict
    returns: Optional[Dict]
    qualified_name: str
    is_method: bool = False
    class_name: Optional[str] = None
