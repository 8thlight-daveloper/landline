import base64
import json
import random
import sys

output_path = sys.argv[1]
random.seed(int(sys.argv[2]))
count = int(sys.argv[3])

scalars = ["x", "yes", "No", "null", "~", "007", "0x1F", "1e3", "1.0", "2026-10-15", '"yes"', "'x'", "[]", "{}", "&a x", "*a"]


def scalar():
    return random.choice(scalars)


def document():
    lines = []
    for _ in range(random.randint(1, 6)):
        lines.append("%s: %s" % (random.choice(["a", "b", "c"]), scalar()))
    return "\n".join(lines) + random.choice(["\n", "", "\n\n"])


documents = [document() for _ in range(count)]
json.dump([base64.b64encode(item.encode()).decode() for item in documents], open(output_path, "w"))
print(len(documents), "inputs")
