"""The research mode as a background job.

The fourth module of this shape, after `api/simruns.py`, `api/themeruns.py` and
`api/dossierruns.py` -- and **the first one that was a job from its first
commit** rather than after a deployed failure taught it to be.

That is the whole point of this docstring. ADR 20's lesson is *a duration
measured for one surface is a question to ask of every sibling surface*, and it
has now cost three separate incidents: the theme proposal at 226 seconds, the
theme conversation turn whose docstring said "it is a few seconds" and was
4.3-133.8, and the dossier at 236 seconds, which presented deployed as a
spinner and then Safari's `Load failed` -- a *transport* error, so no status
code reached the client and no access-log line was written either, because
uvicorn writes one when a response completes and that one never did. Research
searches more than the dossier does. It was never a candidate for a synchronous
POST, and nobody had to measure it to know that.

**Checking happens in the request; calling happens in the job.** Two things can
refuse without touching the network -- a question that is empty or long enough
to be a pasted decklist (422), and no key or SDK (503) -- and each is a
distinct answer the UI acts on differently. Carried into a worker they would
both arrive as a job in state `error`, which is one string for two cases and a
status code for neither.

**Nothing is cached, and that is argued rather than skipped** (ADR 26). ADR 18
caches a simulation because it is reproducible; ADR 19 caches a dossier because
its subject is a character who outlives any conversation. Research's subject is
the part of Magic that moves -- what people think this month, what was spoiled
last week -- so a cache here would serve last month's answer under this month's
question. `generated_at` rides in the payload as the honest substitute.

**It is deduplicated in flight anyway**, which is the distinction
`api/dossierruns.py` had to learn the hard way: the cache covers "somebody
asked before" and `Plan.key` covers "somebody is asking right now", and only
the second one applies here. Two identical question strings from one account
inside the minutes a search takes are one question asked twice. This is the
opposite call from `themeruns.plan_ask`'s `key=None`, and the reason is that
the question text *is* the whole input -- there is no client-held transcript
making two identical-looking requests into two different conversations.

The `NET` lane, for the reason `themeruns` gives: a Claude call waits on a
socket with the GIL released, and queueing it behind a Tier 1 sweep would be
minutes of stall for nothing.

Note what this planner does not take: a `slug`, an `owner`, or a `DeckSource`.
ADR 26's first decision reaches all the way out to the signature.
"""

from __future__ import annotations

from typing import Any

from mtglab.api.jobs import NET, Plan, Progress
from mtglab.api.service import ClaudeFailed

#: What `/api/jobs` calls one of these. Namespaced like `sim.mana` and
#: `claude.dossier` so a job list reads as what produced it.
KIND = "claude.research"

#: How much of the question goes in the job's label. A job list is a list of
#: one-liners, and the full 2000 characters would make it a wall.
LABEL_CHARS = 60


def plan_research(*, question: Any, requested: Any = None) -> Plan:
    """Refuse now if it is refusable; otherwise hand back the work.

    Raises `QuestionRejected` and `ClaudeUnavailable` to the caller, which is
    what keeps their 422 and 503 rather than flattening two answers into one
    job in state `error` with a status code for neither.
    """
    from mtglab.claude import client as claude_client
    from mtglab.claude import research
    from mtglab.claude.modes import ModeExhausted

    request = research.check_research(question, requested=requested)

    short = request.question[:LABEL_CHARS]
    if len(request.question) > LABEL_CHARS:
        short = short.rstrip() + "..."
    label = f"research: {short}"

    answer = request.answer
    if answer is not None:
        # The stance is `off`. A real position and a real answer, and it costs
        # nothing -- so it is returned now rather than a second from now on a
        # worker thread. `jobs.completed` is the shape that says "this job took
        # no time" without forking the client's response.
        return Plan(KIND, label, answer, lambda _progress: answer, lane=NET)

    # Raised here rather than minutes into a job that was never going to work,
    # which preserves the 503 the UI already handles. `require` rather than
    # `connect`, for the reason `themeruns` gives: this only needs to know
    # *whether* a call is possible, and connecting would leave an HTTP client
    # nobody closes behind on every request.
    claude_client.require()

    def run(progress: Progress) -> dict[str, Any]:
        try:
            return research.run_research(request, on_turn=progress)
        except ModeExhausted as exc:
            raise ClaudeFailed(str(exc)) from exc
        except claude_client.ClaudeUnavailable:
            # Only reachable if the key vanished between the check above and
            # the worker starting. Left as itself; it is already readable.
            raise
        except Exception as exc:
            # Broad on purpose and narrow in effect: everything landing here is
            # the SDK failing, and `explain` is what turns a 401 into "your key
            # may have expired" rather than a stack trace in a job's error
            # field.
            raise ClaudeFailed(claude_client.explain(exc)) from exc

    # Keyed on a hash of the normalised question, so a double click is one
    # search. `jobs.submit` matches per **owner** as well as per key, which
    # matters more here than it did for the dossier: a question is somebody's
    # own words, and two accounts that happened to type the same sentence
    # should not be handed each other's job (ADR 5's shape, one layer down).
    return Plan(KIND, label, None, run, lane=NET, key=request.key)
