# Domain Architecture

How a domain-partitioned system is shaped: what a boundary is, what may reference what, and when a new boundary is earned.

## Contents
- When this applies
- What a boundary is
- Dependencies point one way
- Crossing a boundary
- When a new boundary is earned

## When this applies

This assumes the system is partitioned by domain — one folder per domain, feature, or bounded context. A project partitioned by layer, a library with no internal boundaries, or a single-purpose tool takes a different structure and does not take this document.

Nothing here states what goes *inside* a boundary. That divides by stack, not by architecture, and a boundary owns whichever parts it needs.

## What a boundary is

A boundary is a part of the system with its own identity: a domain, a
feature, a bounded context. It owns one folder, and that folder holds
everything the boundary needs and nothing another boundary needs.

## Dependencies point one way

Inside a boundary, declarations come first and implementations depend
on them — never the reverse. A file declaring a contract does not
import the file that satisfies it.

Across boundaries, imports run from the specific to the general: the
layer that composes the system imports a boundary, a boundary imports
shared code, and neither direction reverses.

## Crossing a boundary

- **A boundary is reached only through what it declares public.**
  Everything else inside it is private, whether or not the language
  enforces it.
- **Reaching past the declared surface is a defect**, even where the
  language permits it and even where it works.
- **A cycle between two boundaries means the split is wrong.** Break
  it by inverting one direction — the side that needs the behaviour
  declares the contract and the other implements it — or by merging
  them, or by extracting what both need into a third.

## When a new boundary is earned

A concept earns its own boundary when all three hold:

- It has its own lifecycle and identity.
- Something outside it needs to depend on it.
- It has a surface that can stay stable while its inside changes.

A concern shared by three or more boundaries is extracted. One shared
by two stays with the boundary that owns it and is reached through its
declared surface — unless a boundary may not reach a sibling at all,
in which case it is extracted too.
