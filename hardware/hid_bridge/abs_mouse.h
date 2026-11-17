// Absolute mouse (ABS_POINTER builds), with independent position, button and scroll reports.
// The default mouse layout is adapted from our Windows BLE HID project:
// Copyright (c) 2026 jglwiz. MIT licensed (see LICENSE).
// Keep position separate from clicks: press/release must not resend cached X/Y.
#pragma once
#include "USBHID.h"

// Retain the upstream combined layout only for controlled compatibility comparisons.
#ifndef HID_BLE_LAYOUT
#define HID_BLE_LAYOUT 1
#endif

class AbsMouse : public USBHIDDevice {
public:
  static const uint16_t FULL = 32767;
  // Relative moves (`move`, the keep-awake wiggle) in screen units per mouse unit, about what a relative mouse does.
  static const int MOVE_SCALE = 50;
  // A relative move longer than the screen ends at its edge anyway; bounding it keeps a wild `move` (a paced loop
  // of 8 ms reports) far under the 8 s watchdog.
  static const long MAX_MOVE = FULL / MOVE_SCALE + 1;

  using Output = bool (*)(uint8_t, const void *, size_t);
  AbsMouse(Output output = nullptr) : output_(output) { hid.addDevice(this, sizeof DESCRIPTOR); }
  void begin() { hid.begin(); }

  void moveTo(long x, long y) {
    x_ = constrain(x, 0, FULL);
    y_ = constrain(y, 0, FULL);
    send(0);
  }
  // Paced, for a drag: a jump is a flick to iOS, and the list flies. At a relative mouse's speed it scrolls as far
  // as it is dragged.
  // Bounded for the 8 s watchdog whatever the caller sends: the target is clamped to the screen, the step has a
  // floor (MIN_STEP: at most FULL / MIN_STEP + 1 = 513 reports, ~4.1 s at 8 ms) and the reports a cap (MAX_REPORTS:
  // a longer glide takes bigger steps). The default step (245) crosses the screen in 134 reports, ~1.1 s.
  static const int MIN_STEP = 64;
  static const long MAX_REPORTS = 250;  // ~2 s at 8 ms
  void glideTo(long x, long y, int step, int ms) {
    step = max(step, MIN_STEP);
    x = constrain(x, 0, FULL);
    y = constrain(y, 0, FULL);
    long x0 = x_, y0 = y_;
    long n = min(max(labs(x - x0), labs(y - y0)) / step + 1, MAX_REPORTS);
    for (long i = 1; i <= n; i++) {
      moveTo(x0 + (x - x0) * i / n, y0 + (y - y0) * i / n);
      delay(ms);
    }
  }
  void move(int dx, int dy, int8_t wheel = 0, int8_t = 0) {
    if (dx || dy) moveTo(x_ + dx * MOVE_SCALE, y_ + dy * MOVE_SCALE);
    if (wheel) send(wheel);
  }
  void press(uint8_t b) { buttons_ |= b; sendButtons(); }
  void release(uint8_t b) { buttons_ &= ~b; sendButtons(); }
  bool held() const { return buttons_; }

#if HID_TRACE
  // Diagnostic ring: record in RAM; print only on demand, outside click timing.
  void traceDump() {
    uint32_t first = traceCount_ > 16 ? traceCount_ - 16 : 0;
    for (uint32_t i = first; i < traceCount_; ++i) {
      const Trace &t = trace_[i % 16];
      Serial.printf("hidtrace seq=%lu start_us=%lu end_us=%lu buttons=%u x=%u y=%u wheel=%d ok=%u report=%u len=%u\n",
                    (unsigned long)i, (unsigned long)t.start, (unsigned long)t.end,
                    t.buttons, t.x, t.y, t.wheel, t.ok, t.report, t.length);
    }
    Serial.printf("hidtrace count=%lu held=%u\n", (unsigned long)traceCount_, buttons_);
  }
#endif

  uint16_t _onGetDescriptor(uint8_t *buffer) override {
    memcpy(buffer, DESCRIPTOR, sizeof DESCRIPTOR);
    return sizeof DESCRIPTOR;
  }

private:
  static const uint8_t REPORT_ID = 10;  // USBHID.h takes 1-6
  static constexpr uint8_t DESCRIPTOR[] = {
#if HID_BLE_LAYOUT
    // Match our BLE mouse map; remap IDs to avoid USB consumer-control ID 4.
    // Scroll-only collection: padding, zero relative X/Y, wheel.
    0x05,0x01, 0x09,0x02, 0xA1,0x01, 0x85,12,
    0x09,0x01, 0xA1,0x00,
    0x75,0x08, 0x95,0x01, 0x81,0x01,
    0x05,0x01, 0x09,0x30, 0x09,0x31, 0x09,0x38,
    0x15,0x81, 0x25,0x7F, 0x75,0x08, 0x95,0x03, 0x81,0x06,
    0xC0,0xC0,
    // Absolute position and the sole button state, in independent reports.
    0x05,0x01, 0x09,0x02, 0xA1,0x01, 0x85,REPORT_ID,
    0x09,0x01, 0xA1,0x00,
    0x75,0x08, 0x95,0x01, 0x81,0x01,
    0x05,0x01, 0x09,0x30, 0x09,0x31,
    0x15,0x00, 0x26,0xFF,0x7F, 0x75,0x10, 0x95,0x02, 0x81,0x02,
    0x85,11,
    0x05,0x09, 0x19,0x01, 0x29,0x03, 0x15,0x00, 0x25,0x01,
    0x75,0x01, 0x95,0x03, 0x81,0x02,
    0x75,0x05, 0x95,0x01, 0x81,0x01,
    0xC0,0xC0,
#else
    0x05, 0x01, 0x09, 0x02, 0xA1, 0x01,  // Generic Desktop, Mouse, Application
    0x85, REPORT_ID,
    0x09, 0x01, 0xA1, 0x00,              //   Pointer, Physical
    0x05, 0x09, 0x19, 0x01, 0x29, 0x03,  //     Buttons 1-3
    0x15, 0x00, 0x25, 0x01, 0x75, 0x01, 0x95, 0x03, 0x81, 0x02,
    0x95, 0x05, 0x81, 0x03,              //     padding
    0x05, 0x01, 0x09, 0x30, 0x09, 0x31,  //     X, Y
    0x15, 0x00, 0x26, 0xFF, 0x7F, 0x35, 0x00, 0x46, 0xFF, 0x7F, 0x75, 0x10, 0x95, 0x02, 0x81, 0x02,
    0x09, 0x38, 0x15, 0x81, 0x25, 0x7F, 0x35, 0x00, 0x45, 0x00, 0x75, 0x08, 0x95, 0x01, 0x81, 0x06,  // Wheel
    0xC0, 0xC0,
#endif
  };

  void send(int8_t wheel) {
#if HID_BLE_LAYOUT
    if (wheel) {
      uint8_t r[] = {0, 0, 0, (uint8_t)wheel};
      transmit(12, r, sizeof r, wheel);
    } else {
      uint8_t r[] = {0, (uint8_t)x_, (uint8_t)(x_ >> 8), (uint8_t)y_, (uint8_t)(y_ >> 8)};
      transmit(REPORT_ID, r, sizeof r, 0);
    }
#else
    uint8_t r[6] = {buttons_, (uint8_t)x_, (uint8_t)(x_ >> 8), (uint8_t)y_, (uint8_t)(y_ >> 8), (uint8_t)wheel};
    transmit(REPORT_ID, r, sizeof r, wheel);
#endif
  }

  void sendButtons() {
#if HID_BLE_LAYOUT
    uint8_t r[] = {buttons_};
    transmit(11, r, sizeof r, 0);
#else
    send(0);
#endif
  }

  void transmit(uint8_t report, const void *data, size_t length, int8_t wheel) {
#if HID_TRACE
    uint32_t start = micros();
    bool ok = output_ ? output_(report, data, length) : hid.SendReport(report, data, length);
    trace_[traceCount_++ % 16] = {start, micros(), x_, y_, buttons_, wheel, ok, report, (uint8_t)length};
#else
    if (output_) output_(report, data, length);
    else hid.SendReport(report, data, length);
#endif
  }

#if HID_TRACE
  struct Trace { uint32_t start, end; uint16_t x, y; uint8_t buttons; int8_t wheel; bool ok; uint8_t report, length; };
  Trace trace_[16] = {};
  uint32_t traceCount_ = 0;
#endif

  USBHID hid;
  Output output_;
  uint8_t buttons_ = 0;
  uint16_t x_ = 0, y_ = 0;
};
