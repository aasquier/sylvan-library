"""The theme proposal as a background job.

The conversational half of the theme interview (ADR 20) is a few seconds and
stays a plain POST. The proposal is not: it was **measured at 226 seconds**,
reading a dozen-odd pages and resolving every legend it names against the
corpus. A four-minute synchronous request survives a laptop and does not
survive a hosted proxy -- Fly's router, and most others, give up long before
that -- so it runs the way `api/simruns.py` already runs Tier 1: submitted,
polled, and read back off the job.

**Checking happens in the request; calling happens in the job.** That division
is the whole of this module, and it is the same one `plan_mana` makes for the
same reason. Three things can refuse a proposal without touching the network --
a malformed transcript (422), a floor not yet reached (409), and no key or SDK
(503) -- and each is a distinct answer the UI acts on differently. Carried into
a worker they would all arrive as a *job* in state `error`, which is one string
for three cases and a status code for none of them. So `theme.check_proposal`
runs in the request and only what needs Anthropic is queued.

**A stance of `off` is answered immediately**, as a job born finished, which is
the shape `jobs.completed` already exists for: the honest claim is not "this is
not a job" but "this job took no time".

**Nothing is cached, and that is deliberate.** ADR 18 caches a simulation
because it is reproducible -- the same compiled deck and the same seed are the
same numbers tomorrow, so a hit is the answer rather than a substitute for it.
A proposal is not that. Two runs over one transcript legitimately differ, and
the dossier is cached because its subject is a *character* that outlives any
conversation; a transcript exists for ten minutes and then is gone. Caching
here would mean the one moment somebody wants a different answer -- clicking
again on an unchanged conversation -- is precisely the moment they cannot have
one. What the client keeps instead is the job *id*, so a reload reattaches to
the run already in flight rather than paying for a second one.

The job goes in the `NET` lane (`api/jobs.py`). A Claude call waits on a socket
with the GIL released; queueing it behind a Tier 1 sweep, or a sweep behind it,
would be minutes of stall for nothing.
"""

from __future__ import annotations

from typing import Any

from mtglab.api.jobs import NET, Plan, Progress
from mtglab.api.service import ClaudeFailed

#: What `/api/jobs` calls one of these. Namespaced like `sim.mana` so a job
#: list reads as what produced it rather than as an opaque id.
KIND = "claude.theme.proposal"


def plan_proposal(*, transcript: Any = None, slots: Any = None,
                  requested: Any = None, budget: float | None = None,
                  avoid: str = "") -> Plan:
    """Refuse now if it is refusable; otherwise hand back the work.

    Raises `TranscriptRejected`, `NotReady` and `ClaudeUnavailable` to the
    caller, which is what keeps their 422 / 409 / 503 rather than collapsing
    them into a job error. Everything after that point is a network call and
    belongs in the worker.
    """
    from mtglab.claude import client as claude_client
    from mtglab.claude import theme
    from mtglab.claude.modes import ModeExhausted

    request = theme.check_proposal(transcript, slots, requested=requested,
                                   budget=budget, avoid=avoid)

    label = (f"theme: colours and commanders, from "
             f"{len(request.grounded)} thing"
             f"{'' if len(request.grounded) == 1 else 's'} known")

    if not request.needs_call:
        # Stance `off`: a real position, answered without spending anything.
        # `theme.run_proposal` returns the report saying so, and it returns it
        # now rather than a second from now on a worker thread.
        answer = theme.run_proposal(request)
        return Plan(KIND, label, answer, lambda _progress: answer, lane=NET)

    # Raised here rather than four minutes into a job that was never going to
    # work, which preserves the 503 the UI already handles. `require` rather
    # than `connect`: this only needs to know *whether* a call is possible, and
    # connecting would leave an HTTP client nobody closes behind on every
    # request to answer the same question.
    claude_client.require()

    def run(progress: Progress) -> dict[str, Any]:
        try:
            return theme.run_proposal(request, on_turn=progress)
        except ModeExhausted as exc:
            raise ClaudeFailed(str(exc)) from exc
        except claude_client.ClaudeUnavailable:
            # Only reachable if the key vanished between the check above and
            # the worker starting. Left as itself; it is already readable.
            raise
        except Exception as exc:                                # noqa: BLE001
            # Broad on purpose and narrow in effect, the same treatment
            # `service.claude_interview` gives it: everything landing here is
            # the SDK failing, and `explain` is what turns a 401 into "your key
            # may have expired" instead of a stack trace in a job's error field.
            raise ClaudeFailed(claude_client.explain(exc)) from exc

    return Plan(KIND, label, None, run, lane=NET)
