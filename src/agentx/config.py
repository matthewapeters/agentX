import toml

DEFAULT_CONFIG = "agentx.toml"
def load_config(config_path=DEFAULT_CONFIG):
    with open(config_path, 'r') as f:
        config = toml.loads(f.read())
    return config
def save_config(config, config_path=DEFAULT_CONFIG):
    with open(config_path, 'w') as f:
        toml.dumps(config, f)