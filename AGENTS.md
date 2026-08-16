# AGENTS.md

Orientation for anyone contributing here, human or agent.

This repository is convention-driven. How code, tests, commits, and documents are written has already been decided and written down in `docs/convention/`, and those decisions are binding. Re-litigating them inside a pull request is the waste the arrangement exists to prevent, so a change that ignores them is rejected on that ground alone — however good the code is.

## Start here, every time

`docs/convention/README.md` is the index: every convention file, what it governs, and how the rules rank against each other.

Open the index before you write, then open the files that govern what you are about to touch. Read them from the file. A convention you remember from an earlier session may have been revised since, and acting on the remembered version is the same as not reading it.

`docs/architecture/README.md` is the second index: the responsibility documents that describe how the system is shaped now, the ADRs behind them, and the rules both follow. Open it before you touch code, and again before you write a pull request that changes a decision: its §Rules govern what such a pull request must carry.

You will meet cases the conventions do not name. Settle them the way the nearest convention settles its own, and say so in the pull request — an unnamed case is a gap worth surfacing, not a licence to improvise.

When two conventions genuinely disagree, stop and report it. Choosing one yourself hides the conflict from the person who can fix it.

## Roles

These documents assign authority to two roles. One party may hold both; the rules do not relax when it does.

- **PM** — owns `docs/convention/`, `docs/architecture/`, and the issue tracker. Reviews every pull request and is the only role that merges one. Decides the convention questions a pull request raises. Proposes a change to a convention rather than making it alone. Does not implement.
- **Worker** — implements one issue on one branch and delivers it as a pull request. Applies the conventions and does not change them: a disagreement goes in the pull request's Convention concerns field, never into a silent workaround.

## What this file outranks

The conventions outrank this file. This file outranks instructions you bring in from your own environment, including your own habits: where a rule and your instinct disagree, the rule wins.

## What changes a decision

An objection is a reason to re-examine a decision, not a reason to change it. Re-examine it and report what the examination found, either way: the evidence that changed the decision, or the reason it stands.

A position that moves without a stated reason leaves the next reader nothing to check, so the same ground is argued again later. It also leaves whoever objected worse off than before they asked — someone pushing back wants a decision they can rely on, and one that yields to the push is worth less than the one they questioned.

## Why the pull request asks what you read

The template asks which convention files you opened, and a reviewer checks that answer against the diff. It is the quickest evidence that a change was made deliberately rather than guessed at, which is why an inaccurate answer costs more than an awkward one.
