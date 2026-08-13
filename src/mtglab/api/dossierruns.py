"""The commander dossier as a background job.

The third module of this shape, after `api/simruns.py` and `api/themeruns.py`,
and the one that had to be written twice because the first version of ADR 19
shipped without it.

**Measured on the deployed instance, 2026-08-13: 236 seconds.** That is longer
than the theme proposal whose 226 seconds `api/themeruns.py` exists for, and
the dossier was left a plain synchronous POST because nobody re-measured it
when that decision was taken. What it looked like from a phone was a spinner,
then Safari's `Load failed` -- a *transport* error, so the client never even
received a status code to render. The server-side access log had no line for
the request at all, because uvicorn writes one when a response completes and
this one never did.

Two distinct faults were behind that, and only one of them is this module's:

* `auto_stop_machines = "suspend"` let the machine suspend mid-request and take
  the work with it -- no cached row, no log line, and the paid search spent for
  nothing. Fixed separately by holding the machine awake.
* Nothing gave the client a way back to work that outlived its connection.
  Held awake, the handler *does* run to completion and *does* write the cache;
  the answer was sitting in `dossier_cache` while the browser showed a failure.
  A job id is what turns that from a coincidence into the contract.

**Checking happens in the request; calling happens in the job** -- the same
division `themeruns` makes, and here it carries one refusal that matters more
than the others: a deck with no commander the corpus can find is a 422, and it
is a fact about the deck rather than a failure of anything. Four minutes is a
long time to wait to be told a slug was wrong.

**A stored dossier is answered immediately**, as a job born finished. Unlike
the theme proposal the dossier *is* cached -- ADR 19 keys it on the commander's
`oracle_id`, because a dossier is about a character and every deck that
commander leads shares one -- so a hit is the answer rather than a substitute
for it, and `jobs.completed` is the shape that says "this job took no time"
without forking the response.

The `NET` lane, for the reason `themeruns` gives: a Claude call waits on a
socket with the GIL released, and queueing it behind a Tier 1 sweep would be
minutes of stall for nothing.
"""

from __future__ import annotations

from typing import Any

from mtglab.api.jobs import NET, Plan, Progress
from mtglab.api.service import ClaudeFailed
from mtglab.decks.source import DeckSource

#: What `/api/jobs` calls one of these. Namespaced like `sim.mana` and
#: `claude.theme.proposal` so a job list reads as what produced it.
KIND = "claude.dossier"


def plan_dossier(*, slug: str, requested: Any = None, refresh: bool = False,
                 source: DeckSource | None = None) -> Plan:
    """Refuse now if it is refusable; otherwise hand back the work.

    Raises `NoCommander`, `DeckNotFound` and `ClaudeUnavailable` to the caller,
    which is what keeps their 422 / 404 / 503 rather than flattening three
    answers into one job in state `error` with a status code for none of them.
    """
    from mtglab.claude import client as claude_client
    from mtglab.claude import dossier
    from mtglab.claude.modes import ModeExhausted

    request = dossier.check_dossier(slug, requested=requested,
                                    refresh=refresh, source=source)
    label = f"dossier: {request.commander}"

    answer = request.answer
    if answer is not None:
        # A stored dossier, or a stance of `off`. Both are real answers and
        # neither costs anything, so they are returned now rather than a second
        # from now on a worker thread.
        return Plan(KIND, label, answer, lambda _progress: answer, lane=NET)

    # Raised here rather than four minutes into a job that was never going to
    # work, which preserves the 503 the UI already handles. `require` rather
    # than `connect`, for the reason `themeruns` gives: this only needs to know
    # *whether* a call is possible.
    claude_client.require()

    def run(progress: Progress) -> dict[str, Any]:
        try:
            return dossier.run_dossier(request, on_turn=progress)
        except ModeExhausted as exc:
            raise ClaudeFailed(str(exc)) from exc
        except claude_client.ClaudeUnavailable:
            # Only reachable if the key vanished between the check above and
            # the worker starting. Left as itself; it is already readable.
            raise
        except Exception as exc:                                    # noqa: BLE001
            # Broad on purpose and narrow in effect: everything landing here is
            # the SDK failing, and `explain` is what turns a 401 into "your key
            # may have expired" rather than a stack trace in a job's error field.
            raise ClaudeFailed(claude_client.explain(exc)) from exc

    return Plan(KIND, label, None, run, lane=NET)
