---
name: server-time
description: Report the current date, time, and timezone of the server cogriaclaw runs on. Use when the user asks what time or date it is on the server, or the host's timezone.
---

# Server Time

The server clock is authoritative — never guess the time, always run the script.

## Steps

1. Run the bundled script to read the clock:
   - Call `run_script` with `path` = `server-time/scripts/now.sh`
2. The script prints a single line like `2026-05-21 00:42:10 BST`.
3. Report it to the user in one short, natural sentence, in their language.

## Notes

- This skill only reads the local clock; it takes no arguments.
- If `run_script` is unavailable (execution disabled), tell the user you can't
  read the server clock right now.
