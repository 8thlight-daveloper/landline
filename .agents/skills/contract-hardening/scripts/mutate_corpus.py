import base64
import json
import random
import re
import sys

seed_paths = sys.argv[3:]
output_path = sys.argv[1]
vocabulary = [line.rstrip("\n") for line in open(sys.argv[2], encoding="utf-8")]
random.seed(7)


def mutations(text):
    lines = text.split("\n")
    found = []
    for index in range(len(lines)):
        found.append("\n".join(lines[:index] + lines[index + 1:]))
        found.append("\n".join(lines[: index + 1] + [lines[index]] + lines[index + 1:]))
    for index, line in enumerate(lines):
        keyed = re.match(r"^(\s*(?:- )?[^:#\n]+:\s+)(.+)$", line)
        if keyed:
            for token in random.sample(vocabulary, min(12, len(vocabulary))):
                found.append("\n".join(lines[:index] + [keyed.group(1) + token] + lines[index + 1:]))
        elif re.match(r"^\s*- \S", line):
            prefix = re.match(r"^(\s*- )", line).group(1)
            for token in random.sample(vocabulary, min(4, len(vocabulary))):
                found.append("\n".join(lines[:index] + [prefix + token] + lines[index + 1:]))
    return found


corpus = []
for path in seed_paths:
    data = open(path, "rb").read()
    corpus.append(data)
    made = [m.encode() for m in mutations(data.decode("utf-8", "replace"))]
    random.shuffle(made)
    corpus += made[:150]
unique = list(dict.fromkeys(corpus))
json.dump([base64.b64encode(item).decode() for item in unique], open(output_path, "w"))
print(len(unique), "inputs")
