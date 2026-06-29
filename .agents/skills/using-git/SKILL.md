---
name: using-git
description: Guidelines for using Git version control, including branch policies and restrictions.
---

# Using Git

## Branch Policy

- **Always run `git switch work` before committing.** All agent commits must go on the `work` branch.
- **Never commit to the `main` branch.** Only the user will commit to `main`.

## Workflow

1. Before making any commits, ensure you are on the `work` branch:
   ```bash
   git switch work
   ```
2. Be sure you only add **source code files** that you created or changed.  Never add `_tmp` files, binaries, logs, or other test output.
3. Stage and commit your changes as needed.
4. Never merge into or push to `main`.

## RCS

The user may occasionally use RCS (Revision Control System) for their own purposes. **The agent should never use RCS commands or interact with RCS files** (e.g., `ci`, `co`, `rcs`, or `*,v` files).
