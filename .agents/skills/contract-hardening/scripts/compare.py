import base64
import collections
import json
import subprocess
import sys

corpus_path, first_program, second_program = sys.argv[1:4]
corpus = json.load(open(corpus_path))


def run(program):
    done = subprocess.run([program], input=json.dumps(corpus).encode(), capture_output=True, check=True)
    return json.loads(done.stdout)


def problems(output):
    lines = output.split("\n")
    return [line.split("|")[0] + "@" + line.split("|")[2] for line in lines[1:] if line.count("|") >= 2], lines[0]


first, second = run(first_program), run(second_program)
differing = [i for i in range(len(corpus)) if first[i] != second[i]]
print(len(corpus), "inputs,", len(differing), "differ")
groups = collections.defaultdict(list)
for i in differing:
    first_problems, first_plan = problems(first[i])
    second_problems, second_plan = problems(second[i])
    only_first = sorted(set(first_problems) - set(second_problems))
    only_second = sorted(set(second_problems) - set(first_problems))
    kind = "same problems, different value" if first_problems == second_problems else (
        "same problems, different order" if sorted(first_problems) == sorted(second_problems) else "different problems")
    key = (kind, tuple(p.split("@")[0] for p in only_first), tuple(p.split("@")[0] for p in only_second))
    groups[key].append(i)
for key, indexes in sorted(groups.items(), key=lambda item: -len(item[1])):
    smallest = min(indexes, key=lambda i: len(base64.b64decode(corpus[i])))
    print("=" * 60)
    print(len(indexes), "inputs:", key[0], "| first only:", key[1], "| second only:", key[2])
    print(repr(base64.b64decode(corpus[smallest]).decode("utf-8", "replace")[:400]))
    print(" first: ", first[smallest][:400].replace("\n", " / "))
    print(" second:", second[smallest][:400].replace("\n", " / "))
