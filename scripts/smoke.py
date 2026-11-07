#!/usr/bin/env python3
"""Verify a running DEMO_MODE server; uses only the Python standard library."""
import json
import os
import time
import urllib.error
import urllib.request

BASE = os.environ.get("BASE_URL", "http://127.0.0.1:8080")
TOKEN = os.environ["ADMIN_TOKEN"]

def request(path, payload=None, authenticated=True):
    headers = {"Content-Type": "application/json"}
    if authenticated:
        headers["Authorization"] = "Bearer " + TOKEN
    req = urllib.request.Request(BASE + path, headers=headers,
        data=None if payload is None else json.dumps(payload).encode())
    with urllib.request.urlopen(req, timeout=5) as response:
        return response.status, json.load(response)

assert request("/health")[1]["status"] == "ok"
try:
    request("/api/v1/sources", authenticated=False)
    raise AssertionError("anonymous request succeeded")
except urllib.error.HTTPError as error:
    assert error.code == 401
source = next(s for s in request("/api/v1/sources")[1] if s["name"] == "example")
ids = None
for iteration in range(2):
    status, job = request("/api/v1/jobs", {"source_id": source["id"]})
    assert status == 202
    deadline = time.monotonic() + 45
    while True:
        job = request("/api/v1/jobs/" + job["id"])[1]
        if job["status"] != "running":
            break
        assert time.monotonic() < deadline, "job did not finish"
        time.sleep(0.2)
    assert job["status"] == "succeeded", job
    assert job["collected_count"] == 2, job
    contents = request("/api/v1/contents?source_id=" + source["id"])[1]["items"]
    assert len(contents) == 2, contents
    current_ids = {item["id"] for item in contents}
    if ids is not None:
        assert ids == current_ids, "upsert changed IDs"
    ids = current_ids
filtered = request("/api/v1/contents?category=academic&limit=1")[1]
assert len(filtered["items"]) == 1
assert request("/api/v1/contents/" + filtered["items"][0]["id"])[1]["title"]
assert len(request("/api/v1/jobs")[1]["items"]) >= 2
print("PASS: health, authentication, crawl, pagination, persistence, upsert, filters, job history")
