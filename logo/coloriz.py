first_color = 255 #232 # 16
last_color = 232 # 255 # 45 # 81 #231 # 87 #51
diff = abs(last_color - first_color)

with open("agentx.txt","r",encoding="utf8") as fh:
    lines = fh.readlines()

for line in lines:
    if len(line) <= 1:
        print()
        continue
    u = diff/len(line) 
    
    for i,c in enumerate(line):
        if c in ["\r","\n"]:
            print("\x1b[0m")
            continue
        color=int(first_color - ((i)*u))
        print(f"\x1b[38;5;{color}m{c}", end="")
    print("\x1b[0m",end="")
