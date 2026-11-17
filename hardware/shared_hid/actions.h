// Shared action engine for USB and BLE HID outputs.
// Requires mouse, keyboard, consumer, pacing constants and hidReady().
#pragma once
void movePaced(long dx, long dy, int step = STEP) {
  pace_t p;
  pace_start(&p, dx, dy, step);
  int sx, sy;
  while (pace_next(&p, &sx, &sy)) {
    mouse.move(sx, sy);
    delay(STEP_MS);
  }
}

uint8_t keyCode(const String &k) {
  if (k == "enter") return KEY_RETURN;
  if (k == "esc") return KEY_ESC;
  if (k == "tab") return KEY_TAB;
  if (k == "backspace") return KEY_BACKSPACE;
  if (k == "space") return ' ';
  if (k == "up") return KEY_UP_ARROW;
  if (k == "down") return KEY_DOWN_ARROW;
  if (k == "left") return KEY_LEFT_ARROW;
  if (k == "right") return KEY_RIGHT_ARROW;
  if (k == "cmd") return KEY_LEFT_GUI;
  if (k == "ctrl") return KEY_LEFT_CTRL;
  if (k == "alt") return KEY_LEFT_ALT;
  if (k == "shift") return KEY_LEFT_SHIFT;
  return k.length() == 1 ? k[0] : 0;
}

bool pressCombo(String combo) {
  if (combo == "eject") {
    // iOS toggles the on-screen keyboard once per Consumer Control Eject press.
    consumer.press(CONSUMER_CONTROL_EJECT);
    delay(40);
    consumer.release();
    return true;
  }
  combo += '+';
  for (int i; (i = combo.indexOf('+')) >= 0; combo = combo.substring(i + 1)) {
    uint8_t c = keyCode(combo.substring(0, i));
    if (!c) { keyboard.releaseAll(); return false; }
    keyboard.press(c);
  }
  delay(30);
  keyboard.releaseAll();
  return true;
}

// A locked or sleeping phone suspends USB; sending HID reports then can stall. Ask it to wake up instead,
// and report "asleep" right away if it does not, so the board never hangs.


// `press_for MS`: the button goes up by itself after MS (a long press, or a press then moves: a drag), so a lost
// `release` never leaves it held. 0: nothing pending.
static uint32_t holdUntil;
const long HOLD_MAX_MS = 60000;

String run(const String &line) {
  int sp = line.indexOf(' ');
  String cmd = sp < 0 ? line : line.substring(0, sp);
  String arg = sp < 0 ? "" : line.substring(sp + 1);
  long a = 0, b = 0, c = STEP;
  sscanf(arg.c_str(), "%ld %ld %ld", &a, &b, &c);

  if (cmd == "ping") return "ok";
#if ABS_POINTER && HID_TRACE
  if (cmd == "hidtrace") { mouse.traceDump(); return "ok"; }
#endif
#if ABS_POINTER
  if (cmd == "move" || cmd == "flick") {  // emulated on the absolute mouse; never further than across the screen
    a = constrain(a, -AbsMouse::MAX_MOVE, AbsMouse::MAX_MOVE);
    b = constrain(b, -AbsMouse::MAX_MOVE, AbsMouse::MAX_MOVE);
  }
#endif
  if (cmd == "pace") return "ok " + String(PACE_VERSION);  // how moves are paced: the app recalibrates on a change
  if (!hidReady()) return "err HID output not ready (unlock the phone)";
  if (cmd == "reset") {
    if (arg.length()) return "err usage: reset";
    // Release held inputs in place so the user keeps the current page and text.
    mouse.release(0xff);
    keyboard.releaseAll();
    consumer.release();
    holdUntil = 0;
    return "ok";
  }
  if (cmd == "home") {  // slam the pointer into the top-left corner: left first, then up along the left edge,
    // so it never rides the top edge from the right (that opened Control Center, 2026-11-06)
    for (int i = 0; i < HOME_REPORTS; i++) { mouse.move(-127, 0); delay(2); }
    for (int i = 0; i < HOME_REPORTS; i++) { mouse.move(0, -127); delay(2); }
    return "ok";
  }
  if (cmd == "move") { movePaced(a, b, constrain(c, 1, 40)); return "ok"; }  // c: speed per report (calibration)
#if ABS_POINTER
  if (cmd == "abs") {  // absolute position, 0-32767 across the screen; [step]: per report while dragging
    if (mouse.held()) {
      // Lists need the glide (a jump is a flick, and they fly); system gestures need a flick: a glided home swipe
      // took ~1 s and opened the app switcher instead (2026-11-07). The app sends a big step for those.
      long step = sscanf(arg.c_str(), "%*ld %*ld %ld", &c) == 1 ? constrain(c, AbsMouse::MIN_STEP, AbsMouse::FULL) : ABS_STEP;
      mouse.glideTo(a, b, step, STEP_MS);
    } else {
      mouse.moveTo(a, b);
      delay(ABS_SETTLE_MS);
    }
    return "ok";
  }
#endif
  if (cmd == "press") { mouse.press(MOUSE_LEFT); holdUntil = 0; return "ok"; }
  if (cmd == "press_for") {  // cy41 #103: a held long press (a 30 s hold used to come up after 5 s)
    if (a < 1 || a > HOLD_MAX_MS) return "err usage: press_for MS (1-60000)";
    mouse.press(MOUSE_LEFT);
    holdUntil = millis() + a;
    if (!holdUntil) holdUntil = 1;
    return "ok";
  }
  if (cmd == "release") { mouse.release(MOUSE_LEFT); holdUntil = 0; return "ok"; }
  if (cmd == "flick") {  // press, move, release with no pause between: a lingering stroke from the home bar
    // opened the app switcher instead of the Home Screen (iddzz 2026-11-12)
    mouse.press(MOUSE_LEFT);
    movePaced(a, b, constrain(c, 1, 40));
    mouse.release(MOUSE_LEFT);
    holdUntil = 0;
    return "ok";
  }
  if (cmd == "click") { mouse.press(MOUSE_LEFT); delay(60); mouse.release(MOUSE_LEFT); holdUntil = 0; return "ok"; }
  if (cmd == "scroll") { mouse.move(0, 0, a, b); return "ok"; }
  if (cmd == "scroll_by") {
    // Positive means content moves down; the unit is a bounded gesture, not pixels.
    long amount;
    char extra;
    if (sscanf(arg.c_str(), "%ld %c", &amount, &extra) != 1 || amount < -4 || amount > 4)
      return "err usage: scroll_by AMOUNT (-4..4)";
    if (!amount) return "ok";
    const int wheel = amount > 0 ? 8 : -8;
    const int reports = abs((int)amount) * 8;
    for (int i = 0; i < reports; ++i) {
      if (i) { delay(100); if (!hidReady()) return "err HID output not ready (scroll may be partial)"; }
      mouse.move(0, 0, wheel, 0);
    }
    return "ok";
  }
  if (cmd == "scroll_burst") {
    // A single wheel report reached Safari but did not scroll on the tested iPhone. Keep the reports
    // close together on the board instead of depending on transport round-trip time.
    long v, h, count;
    char extra;
    if (sscanf(arg.c_str(), "%ld %ld %ld %c", &v, &h, &count, &extra) != 3 ||
        v < -127 || v > 127 || h < -127 || h > 127 || (!v && !h) || count < 1 || count > 20)
      return "err usage: scroll_burst V H COUNT (axes -127..127, count 1..20)";
#if ABS_POINTER
    if (h) return "err horizontal scroll unsupported by absolute mouse";
#endif
    for (long i = 0; i < count; ++i) {
      if (i) { delay(100); if (!hidReady()) return "err HID output not ready (scroll may be partial)"; }
      mouse.move(0, 0, v, h);
    }
    return "ok";
  }
  if (cmd == "type") { keyboard.print(arg); return "ok"; }
  if (cmd == "key") return pressCombo(arg) ? "ok" : "err unknown key";
  if (cmd == "homescreen") { consumer.press(AC_HOME); delay(40); consumer.release(); return "ok"; }
  return "err unknown command";
}
