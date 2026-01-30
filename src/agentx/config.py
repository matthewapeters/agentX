import os
from importlib.resources import files
import toml

DEFAULT_CONFIG = "agentx.toml"


def load_config(config_path=DEFAULT_CONFIG):
    with open(config_path, "r") as f:
        config = toml.loads(f.read())
    return config


def save_config(config, config_path=DEFAULT_CONFIG):
    with open(config_path, "w") as f:
        toml.dumps(config, f)


def get_icon_path(icon_name):
    """
    Retrieve the full path to an SVG icon by name.

    Args:
        icon_name (str): The name of the icon file (e.g., 'smiley.svg').

    Returns:
        str: The full path to the icon file.
    """
    base_path = files("agentx").joinpath("assets/icons/opemmoji-svg-color")
    return os.path.join(base_path, icon_name)
