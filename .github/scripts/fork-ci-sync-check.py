#!/usr/bin/env python3
#
# Licensed to the Apache Software Foundation (ASF) under one or more
# contributor license agreements.  See the NOTICE file distributed with
# this work for additional information regarding copyright ownership.
# The ASF licenses this file to You under the Apache License, Version 2.0
# (the "License"); you may not use this file except in compliance with
# the License.  You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

"""Guard that keeps ``.github/workflows/fork-ci.yml`` at least as strict as the
upstream workflows it mirrors.

Background
----------
``fork-ci.yml`` exists so a fork branch can be verified *before* a pull request
against ``apache/devlake`` is opened (the upstream workflows only trigger on
``push`` to ``main`` and on ``pull_request`` with base ``main``/``release-*``).
That only works as long as the mirror actually reproduces the upstream jobs.
Every time an upstream workflow gains a step, bumps an action, changes a pinned
tool version or a service image, the mirror silently becomes *weaker*: fork-ci
goes green, the upstream PR goes red.

What is checked
---------------
1. **Coverage** - every upstream workflow that can be triggered by a pull
   request is either mirrored by a fork-ci job or explicitly waived below.
2. **Step superset** - the normalised step list of the upstream job must be an
   ordered subsequence of the fork-ci job's step list.  Extra fork-only steps
   are therefore allowed (stricter), missing or *modified* upstream steps are
   not (weaker).  ``name``/``id``/``if`` are ignored, ``uses``/``run``/``with``/
   ``env`` are compared verbatim (whitespace-normalised).
3. **Services** - service container images and their environment must match, so
   a ``mysql:``/``postgres:`` bump upstream cannot drift away unnoticed.

Deliberate, documented deviations are declared in ``WAIVERS`` - anything else
fails the job.
"""

from __future__ import annotations

import sys
from pathlib import Path
from typing import Any

try:
    import yaml
except ImportError:  # pragma: no cover - the workflow installs it
    sys.exit("PyYAML is required: pip install pyyaml")

WORKFLOWS = Path(__file__).resolve().parents[1] / "workflows"
FORK_CI = WORKFLOWS / "fork-ci.yml"

# fork-ci job id -> (upstream workflow file, upstream job id)
MIRRORS: dict[str, tuple[str, str]] = {
    "golangci-lint": ("golangci-lint.yml", "golangci"),
    "migration-script-lint": ("migration-script-lint.yml", "migration-script-lint"),
    "unit-test": ("test.yml", "test"),
    "e2e-mysql": ("test-e2e.yml", "e2e-mysql"),
    "e2e-postgres": ("test-e2e.yml", "e2e-postgres"),
    "config-ui": ("config-ui.yml", "lint"),
    "asf-header-check": ("asf-header-check.yml", "check-ASF-header"),
    "grafana-dashboards-check": ("grafana-dashboards-check.yml", "check-grafana-dashboards"),
    "notice-year-check": ("NOTICE-year-check.yml", "notice-year-check"),
    "commit-msg": ("commit-msg.yml", "commit-msg"),
    "yaml-lint": ("yaml-lint.yml", "yaml-lint"),
}

# Upstream workflows that are reachable from a pull_request event but must not
# be mirrored, with the reason why.
NOT_MIRRORED: dict[str, str] = {
    "auto-cherry-pick.yml": "only acts on already merged PRs (github.event.pull_request.merged)",
}

# Fork-only workflows - never compared against upstream.
FORK_ONLY = {"fork-ci.yml", "dependency-watchlist.yml"}

# Documented, intentional deviations. Key: fork-ci job id.
WAIVERS: dict[str, str] = {
    # The fork builds its own lake-builder image from
    # devops/docker/lake-builder/Dockerfile because the published
    # mericodev/lake-builder:latest is stale and only upstream can republish it.
    "container-image": "fork builds lake-builder from the repo Dockerfile",
}


def load(path: Path) -> dict[str, Any]:
    with path.open(encoding="utf-8") as handle:
        return yaml.safe_load(handle) or {}


def triggers(workflow: dict[str, Any]) -> dict[str, Any]:
    """Return the ``on:`` block. PyYAML parses the bare key ``on`` as True."""
    for key in ("on", True):
        if key in workflow:
            value = workflow[key]
            if isinstance(value, str):
                return {value: None}
            if isinstance(value, list):
                return {item: None for item in value}
            return value or {}
    return {}


def normalise(step: dict[str, Any]) -> dict[str, Any]:
    """Reduce a step to what actually influences the outcome."""

    def flat(value: Any) -> Any:
        if isinstance(value, str):
            return " ".join(value.split())
        if isinstance(value, dict):
            return {k: flat(v) for k, v in value.items()}
        if isinstance(value, list):
            return [flat(v) for v in value]
        return value

    out: dict[str, Any] = {}
    for key in ("uses", "run", "with", "env", "shell", "working-directory"):
        if key in step and step[key] is not None:
            out[key] = flat(step[key])
    return out


def render(step: dict[str, Any]) -> str:
    return yaml.safe_dump(step, sort_keys=True, default_flow_style=False).strip()


def is_subsequence(expected: list[dict], actual: list[dict]) -> dict | None:
    """Return the first expected element that could not be matched, in order."""
    cursor = 0
    for item in expected:
        while cursor < len(actual) and actual[cursor] != item:
            cursor += 1
        if cursor == len(actual):
            return item
        cursor += 1
    return None


def main() -> int:
    problems: list[str] = []

    fork = load(FORK_CI)
    fork_jobs = fork.get("jobs", {})

    # ---------------------------------------------------------------- coverage
    mirrored_files = {file for file, _ in MIRRORS.values()}
    for path in sorted(WORKFLOWS.glob("*.yml")):
        if path.name in FORK_ONLY or path.name in mirrored_files:
            continue
        on = triggers(load(path))
        if "pull_request" not in on and "pull_request_target" not in on:
            continue
        if path.name in NOT_MIRRORED:
            continue
        problems.append(
            f"{path.name} runs on pull_request but has no fork-ci mirror. "
            f"Add it to MIRRORS (preferred) or, with a reason, to NOT_MIRRORED."
        )

    # ------------------------------------------------------------ job mirroring
    for fork_job_id, (upstream_file, upstream_job_id) in MIRRORS.items():
        where = f"fork-ci.yml:{fork_job_id} vs {upstream_file}:{upstream_job_id}"

        if fork_job_id not in fork_jobs:
            problems.append(f"{where}: job '{fork_job_id}' is missing from fork-ci.yml")
            continue

        upstream_path = WORKFLOWS / upstream_file
        if not upstream_path.exists():
            problems.append(
                f"{where}: upstream workflow '{upstream_file}' no longer exists - "
                f"drop the mirror or fix MIRRORS"
            )
            continue

        upstream_jobs = load(upstream_path).get("jobs", {})
        if upstream_job_id not in upstream_jobs:
            problems.append(
                f"{where}: upstream job '{upstream_job_id}' no longer exists - fix MIRRORS"
            )
            continue

        upstream_job = upstream_jobs[upstream_job_id]
        fork_job = fork_jobs[fork_job_id]

        expected = [normalise(step) for step in upstream_job.get("steps", [])]
        actual = [normalise(step) for step in fork_job.get("steps", [])]

        missing = is_subsequence(expected, actual)
        if missing is not None:
            problems.append(
                f"{where}: the mirror is weaker than upstream - this step is "
                f"missing or was modified:\n"
                f"{render(missing)}\n"
                f"  fork-ci steps in order:\n"
                + "\n".join(f"    - {render(s).splitlines()[0]}" for s in actual)
            )

        # runs-on must not be weaker either (e.g. ubuntu-24.04 vs ubuntu-latest).
        # Expressions such as '${{ matrix.os }}' cannot be resolved statically.
        if upstream_job.get("runs-on") and fork_job.get("runs-on"):
            up_runs_on = upstream_job["runs-on"]
            if (
                isinstance(up_runs_on, str)
                and "${{" not in up_runs_on
                and up_runs_on != fork_job["runs-on"]
            ):
                problems.append(
                    f"{where}: runs-on differs - upstream '{up_runs_on}', "
                    f"fork '{fork_job['runs-on']}'"
                )

        # service containers (db images) must be identical
        up_services = upstream_job.get("services", {}) or {}
        fork_services = fork_job.get("services", {}) or {}
        for name, spec in up_services.items():
            if name not in fork_services:
                problems.append(f"{where}: service '{name}' is missing in fork-ci")
                continue
            for key in ("image", "env", "ports", "options"):
                if spec.get(key) != fork_services[name].get(key):
                    problems.append(
                        f"{where}: service '{name}.{key}' differs - "
                        f"upstream {spec.get(key)!r}, fork {fork_services[name].get(key)!r}"
                    )

    if problems:
        print("fork-ci.yml has drifted away from the upstream workflows:\n")
        for problem in problems:
            print(f"  * {problem}\n")
        print(
            "Align fork-ci.yml with the upstream workflow (use the stricter side) "
            "and update the coverage map in its header comment."
        )
        return 1

    print(
        f"fork-ci.yml mirrors {len(MIRRORS)} upstream jobs; "
        f"no upstream pull_request workflow is unmirrored."
    )
    for waived, reason in WAIVERS.items():
        print(f"  waived: {waived} - {reason}")
    return 0


if __name__ == "__main__":
    sys.exit(main())

