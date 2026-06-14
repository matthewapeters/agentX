class Color:
    PURPLE = '\033[95m'
    CYAN = '\033[96m'
    DARKCYAN = '\033[36m'
    BLUE = '\033[94m'
    GREEN = '\033[92m'
    YELLOW = '\033[93m'
    RED = '\033[91m'
    BOLD = '\033[1m'
    UNDERLINE = '\033[4m'
    RESET = '\033[0m'

colors=[Color.PURPLE, Color.CYAN, Color.BLUE, Color.GREEN, Color.YELLOW, Color.RED]

with open("agentx.logo","r",encoding="utf8") as fh:
    lines = fh.readlines()

for line in lines:
    for i,c in enumerate(line):
        print(f"{colors[5-int(i/6)]}{c}", end="")

