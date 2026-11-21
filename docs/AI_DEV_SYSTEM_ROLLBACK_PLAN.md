# AI Development System — backup and rollback plan

STATUS: NOT EXECUTED / NOT REHEARSED. No backup, installation or global change has happened. This is the procedure that must be implemented and tested after approval; it is not labelled a verified restore.

## Backup before any approved write

Private location outside the repo: ~/.local/share/ai-dev-backups/<UTC timestamp>/, directory0700. Record timestamp, OS/tool versions, approved scope and exact created/changed paths. Snapshot only these approved sources as applicable:

- ~/.codex/config.toml and applicable profile TOML / MCP config files.
- Global AGENTS.md / AGENTS.override.md, preserving ABSENT if absent.
- Project AGENTS.md, .codex/ config/hooks, .agents/skills/.
- Applicable ~/.agents/skills, ~/.codex/skills and plugin-owned files only when the selected operation can change them; do not blindly copy every cache.
- ~/.codex/hooks.json / RTK.md only if a future hook experiment is approved.
- VS Code User settings/keybindings and relevant profile settings; extension ID/version inventory and registry metadata before extension changes.

No auth.json, SSH private key, Keychain or unrelated secret-store export. No backup inside Git. A config can contain secrets even if current metadata looked safe: use restrictive permissions and encryption if the user requires off-device storage. Do not print its content.

For every source record: absolute path, regular/directory/symlink/ABSENT, SHA256 for regular files, mode, symlink target, and relevant metadata. Use lstat/readlink; stop on unexpected link, alias, ownership or source change. Copy bytes preserving file modes and symlinks, not dereferenced external targets. A suitable Python implementation uses copy2(follow_symlinks=False) and copytree(symlinks=True); ACL/xattr fidelity is platform-dependent and must be tested, not assumed. If exact metadata restoration is required but unproven, do not modify that existing file.

Re-hash sources after backup and immediately before patch; fail on drift. Keep the original manifest immutable, add an operation manifest listing only owned writes. Avoid copying a live VS Code state database casually; use supported extension uninstall plus saved extension inventory, and close the app for any separately approved database backup.

## Reversible steps

| Change | Rollback |
|---|---|
| New global/project AGENTS |Remove only manifest-created file whose content still matches this installation; if edited later, stop and merge |
| Existing TOML table patch |Restore only owned key/table delta; if whole-file fingerprint changed concurrently, do not overwrite unrelated new settings |
| VS Code Codex extension |code --uninstall-extension openai.chatgpt only if this rollout installed it; verify prior extension inventory/settings remain |
| Optional npm Codex |Restore previous bin/link target and package version from manifest; no blanket npm uninstall if prior install existed |
| Context7 |Disable/remove only the new MCP table, then user revokes newly created OAuth access; no unrelated credential-store deletion |
| GitHub read-only MCP |Disable/remove new connection, restore old plugin enablement only after risk review; revoke only new selected-repo token |
| Superpowers |Disable/remove single native plugin; restore only owned instructions/hooks; never delete all skills |
| RTK manual pilot |Stop explicit prefixes, remove only new verified versioned binary and temporary fixture after evidence retention |
| Later RTK project hook |First dry-run: rtk init --codex --uninstall --dry-run; inspect; then approved rtk init --codex --uninstall; restore exact hook/AGENTS deltas and symlink targets |
| Serena (not planned now) |Disable server, remove owned index/config/tool environment only if provenance manifest says this rollout created them |
| Draft CI changes |Restore approved local file patch from backup; do not commit/push as part of rollback without separate authorization |

Never use git clean/reset/checkout as blanket recovery: this repo has0 tracked files and no HEAD. Never delete the entire ~/.codex, ~/.agents, ~/.vscode, plugin cache or .serena directory. Uninstall commands can follow links or affect user-owned content: backup and ownership manifest take precedence over a vendor cleanup slogan.

## Future restore rehearsal

Use a disposable directory and synthetic copies first. Verify: restore bytes/hash/mode/link target; preserve pre-existing files; restore ABSENT correctly; refuse concurrent user edits; reject symlinks outside allowed scope; preserve secret redaction; uninstall does not remove another component's files.

Then verify actual selected installation: Codex/version/config parse, VS Code extension inventory, AGENTS discovery, no duplicate skills/MCP, exact tool allowlist, no shell interception and original app quality gates. Model-based smoke requires explicit usage authorization; absence of it is BLOCKED, not PASS.

Recovery after unexpected dependency, secret access, permissions, hook or new remote endpoint: stop, disable the just-added component if safely possible, preserve evidence without secrets, report the concrete blocker and request new approval. No automatic broader install.
