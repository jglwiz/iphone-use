---
name: esp32agent
description: See and drive a real iPhone through an ESP32 Agent board (USB or BLE HID mouse/keyboard + AirPlay screen mirroring, reached over serial). Use when asked to look at, tap, type on, or operate the iPhone, or to test an iOS app on the device.
---

# esp32agent

`esp32agent` talks over serial to an ESP32 Agent board connected to the iPhone by USB or BLE HID. The board is the phone's USB or BLE HID mouse and
keyboard; this computer reaches it over a persistent serial port. Screens come from an independent HTTP AirPlay service. Every command prints
JSON: `{"status":"ok","data":...}` or `{"status":"error","error":"..."}`. The MCP server (`esp32agent mcp`)
offers the same as tools: screenshot, tap, longpress, swipe, scroll, type_text, press_key, press_button, device_info,
plus history, intent_list, intent_show, intent_save, memory_recall and memory_add.

## Setup (once)
- Build this serial branch from source. Upstream npm/installers/upgrade do not contain this implementation.
- Set `ESP32AGENT_PORT` to the verified port, e.g. COM24; `devices` only enumerates ports.
- Set `ESP32AGENT_FRAME_URL` to the existing AirPlay HTTP `/frame` endpoint (default http://127.0.0.1:8892/frame).
- Keep the receiver running; stop a verified temporary serial bridge before starting this daemon.
- Set `ESP32AGENT_HID_OUTPUT=usb` (default) or `ble`; stop the daemon before changing configuration. `device info` reports `control_transport=serial` and the actual output. USB requires `usb=ok`; BLE requires `ble_ready`, `ble_connected`, `ble_encrypted`, `ready` true and no fault. No PC Bluetooth pairing.
- Pair the iPhone through AssistiveTouch → Devices → Bluetooth Devices; USB can guide pairing. Only manage the intended iPhone Agent HID device (older firmware or cached pairings may show Handsy Serial HID or Handsy-BLE-Lab). BLE failure never falls back to USB.
- `calibrate` only reports native absolute mode; it does not generate verified calibration.

## The loop
1. `esp32agent screenshot` (writes a PNG in the current directory and prints its `filePath`; read the image).
   Read the capture metadata: an old still frame may be returned as observation_only with its original timestamp. It is not post-action evidence or permission to replay input.
2. Decide one action from what you see.
3. Act, with coordinates in pixels of that screenshot (full size: don't pass `--scale`/`--max-size` if you
   take coordinates from it):
   - `esp32agent io tap X,Y` (`--aim` checks absolute pointer response and blocks the click on a miss; it does not correct relatively; `--count 2` double-taps, e.g. to zoom a map or select a word)
   - `esp32agent io longpress X,Y --duration 1500`
   - `esp32agent io swipe X1,Y1,X2,Y2` (pointer drag, not guaranteed page scrolling)
   - `esp32agent io scroll 1` (content down), `io scroll -1` (up); integer -4..4, zero no-op
   - `esp32agent io text 'hello'`, `esp32agent io keys enter`, `esp32agent io keys cmd+a`
   - `esp32agent io button HOME` (Home Screen)
4. Screenshot again and check the result before the next step. Never chain several taps blind.

Phone audio is not provided by the serial/HTTP image path. Do not start a Bluetooth or Wi-Fi control fallback.

## Typing
- The board types printable ASCII only, one key per character.
- Chinese is typed as pinyin: `io text '拿铁'` types `natie` (the reply says `typed_as`). With the iPhone's Chinese (pinyin)
  keyboard active, commit the first candidate with `io keys space`, or screenshot and tap the right candidate.
  Many search boxes accept the plain pinyin as well.
- The board is a hardware keyboard, so iOS may hide the on-screen keyboard; the text field still takes the keys.
- Tap the text field first so it has focus; `io keys cmd+a` then `io keys backspace` clears it.

## Save what worked
After a task succeeded, save it so the next run is faster:
1. `esp32agent history show latest` (MCP: `history` with action show): the run's steps in 0-1000 screen units,
   numbered, each with the screenshot it was done on. Every action and screenshot is recorded by itself; a new run
   starts after 10 minutes idle, or with `esp32agent history new` before a new task.
2. Pick the steps that did the job (skip detours, misses and retries).
3. `esp32agent intent save --name 'Alarm 7:00' --prompt 'Set an alarm for 7:00 AM every day' --hint 'Type the time
   instead of scrolling the wheel' --from-run RUN --steps 2,3,5` (MCP: `intent_save` with name, prompt, hints,
   from_run, steps). One short, clear instruction as the prompt and at most 6 concrete hints. Only replayable moves
   can be steps: tap, swipe, type, key, home. The same name replaces the old intent.

## Run a saved intent
1. `esp32agent intent list`, then `esp32agent intent show NAME` (MCP: `intent_list`, `intent_show`): the prompt,
   hints and recorded steps, each with its reference screen (a JPEG path).
2. Follow the steps as references, not blindly: coordinates are 0-1000 units of the screen, so convert them to
   pixels of your screenshot (x * width / 1000, y * height / 1000).
3. Take a screenshot after each step and check it looks like the next step's reference screen.
4. On a mismatch (another screen, a popup, a changed layout) drop the recorded steps and go on with the normal loop,
   using the prompt and hints.

## Memory
- Before a task, `esp32agent memory search 'TASK'` (MCP: `memory_recall`): the user's preferences and how their apps
  work, the same entries the iOS app uses.
- When the user says to remember something, `esp32agent memory add 'TEXT' --kind preference` (MCP: `memory_add`).
  Only the user's own words; never passwords, codes or numbers off the screen.

## Errors and limits
- An uncertain timeout/disconnect is never replayed. Inspect the phone and saved output; recover the daemon explicitly.
- An old screenshot may be a still screen: inspect capture metadata. `observation_only` preserves the original frame time and is not post-action evidence. Source failure, no frame, and an explicitly reported mirroring disconnect are separate errors; a cached image alone leaves mirroring state unknown. Do not restart UxPlay automatically.
- USB not ready: check phone unlock and the native USB cable. Port names are not permanent identities.
- BLE not ready: check the intended phone pairing, encryption and report subscriptions; never borrow USB readiness. After a connection-generation fault, inspect the phone and explicitly restart the daemon; input is not replayed.
- Another controller owns the board: do not inject input concurrently.
- A firmware `ok` means submission, not a verified UI effect. Inspect the next screenshot.
- `swipe` moves held objects but is not proven to scroll every page; use `scroll` for content.
- Full-screen edges, orientation changes, long-running reliability and the reference Backend's full guard/calibration workflow are not certified by this migration.
- Only HOME as a button. No app list, UI tree, direct file access or webview APIs; work from screenshots.
- H.264 capture and phone audio are unavailable from an HTTP JPEG/PNG source; MJPEG is supported.
