#!/usr/bin/env python3
"""An exec transform: rewrites the persona's message the way a user typing in
a hurry would, to test that the agent copes. It reads the transform request,
{"message", "persona", "scenario", "exchange"}, as JSON on stdin and writes
{"message": "..."} on stdout. The rewrite depends only on the message, so a
run is repeatable."""
import json
import sys
import zlib

req = json.load(sys.stdin)
text = req["message"].strip()

# Start lowercase and drop the final full stop.
if text:
    text = text[0].lower() + text[1:]
if text.endswith("."):
    text = text[:-1]

# Swap two adjacent letters in one longer word, picked from the message itself.
words = text.split(" ")
long_words = [i for i, w in enumerate(words) if len(w) > 4 and w.isalpha()]
if long_words:
    i = long_words[zlib.crc32(text.encode()) % len(long_words)]
    w = words[i]
    j = len(w) // 2
    words[i] = w[: j - 1] + w[j] + w[j - 1] + w[j + 1 :]
text = " ".join(words)

json.dump({"message": text}, sys.stdout)
