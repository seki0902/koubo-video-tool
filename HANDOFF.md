# Koubo Video Tool - Handoff

**Updated:** 2026-08-07
**AppID:** 44602067
**API Base:** https://open-api.chanjing.cc/open/v1
**Official docs:** https://doc.chanjing.cc/api/video-synthesis/synthesize-digital-person-video.html
**Working n8n workflow:** D:\口播视频工作流\口播稿改写工作流-2-新版.json

## Root causes fixed

The Chanjing status mapping in the Go scheduler was reversed.

- Official docs and the working n8n workflow define `status=10` as generating and `status=30` as success.
- The Go scheduler treated `20` as success and `30` as failure.
- On `status=30`, it also discarded `status.VideoURL` by writing an empty URL.
- This made every successfully rendered video appear failed locally even though Chanjing returned a valid video URL.

The scheduler now treats `status=30` or `queue_status=completed` as success, treats `4X`/`5X` or `queue_status=failed` as failure, preserves the video URL, and only sets `completed_at` for terminal states.

The recurring `no matching voice ID` failure had a separate cause:

- `C-06d6082c2f39471b8cd28a2f1585b170` is a Web-created custom voice.
- Chanjing support confirmed that OpenAPI calls using this voice must send both `source: 1` and `audio_source: 1`; otherwise Chanjing looks in the OpenAPI voice pool instead of the Web voice pool.
- A direct `create_video` probe with both fields at the request top level completed successfully as task `2085577089969405952`.
- The app now stores `audio_source: 1` as metadata for that voice and adds it to create and retry requests. System voices continue to omit `audio_source`.

## Official request contract

The working endpoint is `POST /open/v1/create_video`, followed by `GET /open/v1/video?id={taskID}`.

The working n8n request establishes the main video request shape:

- nested `person`
- nested `audio.tts`
- `subtitle_config`
- top-level `source: 1` for Chanjing main-site Web custom avatars
- system voice `C-fa39b63eaefa4d3689526a1dfd5a25f3`, which does not need `audio_source`

The workflow does not cover Web custom voice selection, so it cannot be copied mechanically for `C-06d6082c2f39471b8cd28a2f1585b170`. For that voice, the current verified payload uses top-level `source: 1` and top-level `audio_source: 1`. Neither source field is emitted inside `audio.tts` by current code. An older saved task contains nested source fields, but the direct top-level probe above is the canonical evidence for the current implementation.

## Resource findings

The account resource APIs and local assets confirm:

- All 13 local avatar IDs now correspond to the intended Web custom avatars.
- Avatar 6 was stored with a truncated ID. It was corrected from `C-83d49fb02c96470bb3ebdaabcdb20d` to `C-83d49fb02c96470bb3ebdaabcdb20d59`, and its embedded preview image was renamed to match.
- Avatar 7 (`C-a0f9a24aa3b645e4a69f85fa18f85d18`) is bound to voice `C-06d6082c2f39471b8cd28a2f1585b170`.
- The working n8n baseline uses avatar 5 (`C-6a951f1434884368bc4942f7f74885ff`) and voice `C-fa39b63eaefa4d3689526a1dfd5a25f3`.

The PNG files under `chanjing/person id/` are local UI previews only. Chanjing does not receive or compare those images; the API receives `person.id`. Matching a PNG filename to `avatars.json` proves local consistency, not remote pool selection.

Retract the earlier interpretation that the avatars were expired, invalid, unusable, or mismatched with the voice. The server returned a voice-pool lookup error, and the app passed that error through unchanged. The problem was incorrectly diagnosed as an avatar problem before the working workflow, the support answer, and the exact request fields were compared.

## Verified successful tasks

The following local tasks were re-read from Chanjing. All returned `status=30`, `queue_status=completed`, and a non-empty `video_url`, so their local records were repaired to `done`:

- `2085291305550680064` - original avatar 7/custom voice task
- `2085567880201252864` - avatar 7/custom voice, short test
- `2085568645665505280` - n8n avatar 5/system voice, short test
- `2085568953808384000` - n8n avatar 5/system voice, normal-length test
- `2085577089969405952` - avatar 1/Web custom voice with top-level `source: 1` and `audio_source: 1`
- `2085588128118640640` - normal-length task submitted through the UI with avatar 1/Web custom voice; the scheduler advanced it from 10% to 75% to `done` automatically

Remote history contained zero `/video_lip_sync` tasks and 20 `/create_video` tasks. Do not switch the app to `/video_lip_sync/create`.

## Other fixes retained

- Task history is retained locally for 30 days. Terminal records older than 30 days are pruned, while pending/generating tasks are always kept.
- The UI and API support deleting one terminal task record or clearing all terminal history. These actions only modify local `data/tasks.json`; they do not delete Chanjing videos.
- Task retry endpoint and frontend retry action.
- Runtime Chanjing credential refresh after settings save.
- Explicit config decryption errors.
- Blank Chanjing credential validation.
- Browser-local encrypted credential recovery.
- Tutorial script execution fix.
- Scheduler, handler, store, skill, and Chanjing regression tests.

## Current runtime state

- The fixed service is running at `http://localhost:8899` as process ID `28192`.
- `data/config.json` contains a non-empty AppID and an encrypted `ENC:` SecretKey.
- Local API verification returned 6 tasks, all `done`, and the latest task has a video URL.
- `/api/avatars` returns the corrected avatar 6 ID.
- Browser verification showed all six task cards with success icons and preview actions. The newest UI-created task is `2085588128118640640`.
- The only browser console error was an unrelated missing `/favicon.ico`.

## Validation completed

```powershell
go test ./...
go build -o koubo-tool.exe .
```

All Go tests passed. The binary was rebuilt after the status fix.

## Next steps

1. When debugging future Chanjing failures, compare the emitted payload with `D:\口播视频工作流\口播稿改写工作流-2-新版.json` first, then apply resource-specific fields confirmed by Chanjing support.

The temporary Go toolchain and Playwright snapshots created during diagnosis were removed after the final build and test run.
