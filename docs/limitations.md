# Known limitations

## Bitwarden MFA

`bw unlock` with MFA may work for some methods (e.g. TOTP) but is not explicitly tested or supported. Interactive prompts during `bw unlock` that require stdin input beyond a password may hang.

Tracked in [#2](https://github.com/OpalBolt/envoke/issues/2).

## macOS sleep and lock detection

On macOS, envoke uses a **timer-drift heuristic** to detect sleep/wake events: it polls every 10 s and assumes a sleep occurred if a tick fires more than 20 s late. This means:

- Cache cleanup runs **after** wake, not before sleep
- Screen lock events are **not** detected — the `RegisterLock` hook is a no-op on macOS

True before-sleep and on-lock notifications require CGo/IOKit and are not yet implemented.

Tracked in [#10](https://github.com/OpalBolt/envoke/issues/10).

## Windows not officially supported

GoReleaser targets for Windows have been dropped. The code compiles on Windows but is not tested or supported. There is no `/run/user/<uid>` or `/dev/shm` equivalent on Windows — secrets fall back to `%TEMP%`, which is not RAM-backed.
