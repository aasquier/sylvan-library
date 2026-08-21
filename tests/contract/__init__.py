"""The contract suite: what the served app promises over the wire.

A pytest package rather than loose files so its helpers import by one name
(`contract.harness`, `contract.routes`) from anywhere under `tests/`, and so a
test module here can never shadow one in the parent directory. `README.md`
beside this file is the map; `routes.json` and `golden/` are the data the Go
migration reads too (docs/go-migration/PLAN.md, sections 5 and 8).
"""
