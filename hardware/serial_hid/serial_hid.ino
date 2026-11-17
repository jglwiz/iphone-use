// Serial control with selectable USB or BLE HID output; shared action engine.
// Based on ESP32 iPhone Agent, Copyright (c) 2026 Henry Yuan, MIT (root LICENSE).
#include <Arduino.h>
#include <atomic>
#include <esp_mac.h>
#include <esp_system.h>
#include <NimBLEDevice.h>
#include <NimBLEHIDDevice.h>
#include <USB.h>
#include <USBHIDKeyboard.h>
#include <USBHIDConsumerControl.h>
#include <USBHIDMouse.h>
#include <tusb.h>
#include "cJSON.h"
#define ABS_POINTER 1
#define HID_TRACE 0
#include "../shared_hid/pace.h"

// Exactly one output for each command. No radio command characteristics exist.
bool useBLE = false, reportFailed = false, clearing = false, linkFault = false;
bool advertisingConfigured=false, advertisingStarted=false;
std::atomic<bool> connected{false}, encrypted{false}, suspended{false};
std::atomic<unsigned> generation{0}, subscriptions{0};
std::atomic<uint16_t> peerHandle{0xffff};
std::atomic<int> disconnectReason{-1};
std::atomic<unsigned> authEvents{0};
std::atomic<bool> securityStarted{false}, lastAuthEncrypted{false}, lastAuthBonded{false};
unsigned commandGeneration = 0, selectedGeneration = 0;
uint32_t usbCalls = 0, bleReports = 0;
String lastBLE[13];
String bootId, deviceId;
NimBLEHIDDevice *bleHid;
NimBLECharacteristic *inputs[13] = {};
USBHID usbSender;
USBHIDKeyboard usbKeyboard;
USBHIDConsumerControl usbConsumer;
constexpr unsigned REQUIRED = (1<<1)|(1<<4)|(1<<10)|(1<<11)|(1<<12);

bool bleReady() { return connected && encrypted && !suspended && (subscriptions.load() & REQUIRED) == REQUIRED; }
bool outputReady() { return useBLE ? bleReady() && !linkFault : tud_mounted() && !tud_suspended(); }
bool sendHIDReport(uint8_t id, const void *data, size_t n) {
  if (reportFailed && !clearing) return false;
  bool ok = false;
  if (useBLE) {
    if (bleReady() && (clearing || (!linkFault && generation == commandGeneration)) && id < 13 && inputs[id]) {
      inputs[id]->setValue((const uint8_t *)data,n);
      ok = inputs[id]->notify();
      if (ok) {
        ++bleReports; lastBLE[id]="";
        for(size_t i=0;i<n;i++) { char b[3]; snprintf(b,3,"%02x",((const uint8_t*)data)[i]); lastBLE[id]+=b; }
      }
    }
  } else if (tud_mounted() && !tud_suspended()) {
    ++usbCalls;
    ok = usbSender.SendReport(id,data,n);
  }
  if (!ok) reportFailed = true;
  return ok;
}
#include "../shared_hid/abs_mouse.h"
AbsMouse mouse(sendHIDReport);

// Keep native USB keyboard behavior. BLE implements the same keyboard calls.
class KeyboardOutput {
  uint8_t keys[8] = {};
public:
  bool held = false;
  size_t press(uint8_t k) {
    held = true;
    if (!useBLE) { ++usbCalls; auto n=usbKeyboard.press(k); if (!n) reportFailed=true; return n; }
    uint8_t usage = 0, mods = keys[0];
    if (k >= 0x88) usage = k-0x88;
    else if (k >= 0x80) mods |= 1 << (k-0x80);
    else { usage = KeyboardLayout_en_US[k]; if (!usage) { reportFailed=true; return 0; }
      if (usage & 0x80) { mods |= 2; usage &= 0x7f; } }
    if (mods != keys[0]) {
      keys[0] = mods;
      if (!sendHIDReport(1,keys,8)) return 0;
      delay(200); // Establish modifiers before a character on iOS.
    }
    if (usage) {
      bool added=false;
      for (int i=2;i<8;i++) if (keys[i] == usage) { added=true; break; }
      if (!added) for (int i=2;i<8;i++) if (!keys[i]) { keys[i]=usage; added=true; break; }
      if (!added) { reportFailed=true; return 0; }
    }
    return sendHIDReport(1,keys,8);
  }
  void releaseAll() {
    held=false;
    if (!useBLE) { ++usbCalls; usbKeyboard.releaseAll(); return; }
    if (keys[0]) { memset(keys+2,0,6); sendHIDReport(1,keys,8); delay(60); }
    memset(keys,0,8); sendHIDReport(1,keys,8);
  }
  void print(const String &text) {
    if (!useBLE) { ++usbCalls; usbKeyboard.print(text); return; }
    for (unsigned i=0;i<text.length() && !reportFailed;i++) { press(text[i]); delay(60); releaseAll(); delay(80); }
  }
} keyboard;
class ConsumerOutput {
public:
  bool held=false;
  void press(uint16_t key) {
    held=true;
    if (!useBLE) { ++usbCalls; if (!usbConsumer.press(key)) reportFailed=true; }
    else { uint8_t b[]={uint8_t(key),uint8_t(key>>8)}; sendHIDReport(4,b,2); }
  }
  void release() { held=false; if (!useBLE) { ++usbCalls; if (!usbConsumer.release()) reportFailed=true; }
    else { uint8_t b[2]={}; sendHIDReport(4,b,2); } }
} consumer;
const int STEP=10, ABS_STEP=245, ABS_SETTLE_MS=60, STEP_MS=8, HOME_REPORTS=25;
const uint16_t AC_HOME=0x0223;
bool hidReady() {
  if (!useBLE && tud_suspended()) {
    tud_remote_wakeup();
    for (int i=0;i<50 && tud_suspended();i++) delay(10);
  }
  return outputReady();
}
#include "../shared_hid/actions.h"

bool releaseInputs() {
  clearing=true;
  mouse.release(0xff); keyboard.releaseAll(); consumer.release(); holdUntil=0;
  clearing=false;
  return !reportFailed;
}
class PeerCallbacks : public NimBLEServerCallbacks {
  void onConnect(NimBLEServer *s, NimBLEConnInfo &info) override {
    if (connected) { s->disconnect(info.getConnHandle()); return; }
    peerHandle=info.getConnHandle();
    ++generation; connected=true; encrypted=info.isEncrypted(); suspended=false;
    s->updateConnParams(info.getConnHandle(),12,24,0,400);
    securityStarted=NimBLEDevice::startSecurity(info.getConnHandle());
  }
  void onDisconnect(NimBLEServer *, NimBLEConnInfo &info, int reason) override {
    if (info.getConnHandle()!=peerHandle) return;
    peerHandle=0xffff;
    disconnectReason=reason;
    connected=false; encrypted=false; subscriptions=0; suspended=false; ++generation;
  }
  void onAuthenticationComplete(NimBLEConnInfo &info) override {
    if(info.getConnHandle()==peerHandle) {
      encrypted=info.isEncrypted(); ++authEvents;
      lastAuthEncrypted=info.isEncrypted(); lastAuthBonded=info.isBonded();
    }
  }
};
class SubscriptionCallbacks : public NimBLECharacteristicCallbacks {
  void onSubscribe(NimBLECharacteristic *ch, NimBLEConnInfo &info, uint16_t flags) override {
    if(info.getConnHandle()!=peerHandle) return;
    for (unsigned i=0;i<13;i++) if (inputs[i] == ch) {
      if (flags&1) subscriptions.fetch_or(1u<<i); else subscriptions.fetch_and(~(1u<<i));
    }
  }
};
class SuspendCallbacks : public NimBLECharacteristicCallbacks {
  void onWrite(NimBLECharacteristic *ch, NimBLEConnInfo &) override {
    auto value=ch->getValue(); if (value.size()) suspended=value[0]==0;
  }
};
String statusJSON() {
  cJSON *j=cJSON_CreateObject();
  cJSON_AddStringToObject(j,"fw","0.3.0-serial");
  cJSON_AddStringToObject(j,"name","Handsy Serial HID");
  cJSON_AddStringToObject(j,"boot_id",bootId.c_str());
  cJSON_AddStringToObject(j,"device_id",deviceId.c_str());
  cJSON_AddStringToObject(j,"hid_output",useBLE?"ble":"usb");
  cJSON_AddStringToObject(j,"usb",tud_mounted()?(tud_suspended()?"asleep":"ok"):"off");
  cJSON_AddStringToObject(j,"owner","none");
  cJSON_AddNumberToObject(j,"abs",1); cJSON_AddNumberToObject(j,"up",millis());
  cJSON_AddBoolToObject(j,"output_select",true); cJSON_AddBoolToObject(j,"ready",outputReady());
  cJSON_AddBoolToObject(j,"ble_connected",connected); cJSON_AddBoolToObject(j,"ble_encrypted",encrypted);
  cJSON_AddBoolToObject(j,"ble_ready",bleReady()); cJSON_AddNumberToObject(j,"ble_subscriptions",subscriptions);
  auto *adv=NimBLEDevice::getAdvertising();
  cJSON_AddBoolToObject(j,"advertising_configured",advertisingConfigured);
  cJSON_AddBoolToObject(j,"advertising_started",advertisingStarted);
  cJSON_AddBoolToObject(j,"advertising",adv->isAdvertising());
  cJSON_AddStringToObject(j,"ble_address",NimBLEDevice::getAddress().toString().c_str());
  cJSON_AddStringToObject(j,"advertisement",adv->getAdvertisementData().toString().c_str());
  // NimBLE 2.3.8 getScanData() incorrectly returns advertising data; do not report it as on-air scan data.
  cJSON_AddStringToObject(j,"scan_response_name_configured","Handsy Serial HID");
  cJSON_AddNumberToObject(j,"connection_generation",generation);
  cJSON_AddNumberToObject(j,"disconnect_reason",disconnectReason);
  cJSON_AddNumberToObject(j,"auth_events",authEvents);
  cJSON_AddBoolToObject(j,"security_started",securityStarted);
  cJSON_AddBoolToObject(j,"last_auth_encrypted",lastAuthEncrypted);
  cJSON_AddBoolToObject(j,"last_auth_bonded",lastAuthBonded);
  cJSON_AddNumberToObject(j,"bond_count",NimBLEDevice::getNumBonds());
  cJSON_AddNumberToObject(j,"pnp_length",bleHid->getPnp()->getValue().size());
  auto protocol=bleHid->getProtocolMode()->getValue();
  cJSON_AddNumberToObject(j,"ble_protocol",protocol.size()?protocol[0]:-1);
  cJSON_AddBoolToObject(j,"fault",linkFault); cJSON_AddNumberToObject(j,"held",mouse.held()||keyboard.held||consumer.held);
  cJSON_AddNumberToObject(j,"usb_calls",usbCalls); cJSON_AddNumberToObject(j,"ble_reports",bleReports);
  cJSON_AddStringToObject(j,"last_ble_keyboard",lastBLE[1].c_str());
  cJSON_AddStringToObject(j,"last_ble_position",lastBLE[10].c_str());
  cJSON_AddStringToObject(j,"last_ble_buttons",lastBLE[11].c_str());
  cJSON_AddStringToObject(j,"last_ble_scroll",lastBLE[12].c_str());
  char *s=cJSON_PrintUnformatted(j); String out(s?s:"{}"); cJSON_free(s); cJSON_Delete(j); return out;
}
String dispatch(const String &command) {
  if (command=="status") return statusJSON();
  if(command=="advertise off") return NimBLEDevice::getAdvertising()->stop()?"ok":"err stop advertising";
  if(command=="advertise on") return NimBLEDevice::startAdvertising()?"ok":"err start advertising";
  if (command=="diag") {
    auto *ch=bleHid->getHidService()->getCharacteristic(NimBLEUUID((uint16_t)0x2a4b));
    auto value=ch->getValue(); String hex;
    for(size_t i=0;i<value.size();i++) { char b[3]; snprintf(b,3,"%02x",value[i]); hex+=b; }
    return "ok "+hex;
  }
  reportFailed=false;
  commandGeneration=generation;
  if (command=="output usb" || command=="output ble") {
    if (mouse.held() || keyboard.held || consumer.held || holdUntil) return "err release before switching output";
    // A disconnected old output cannot block recovery through the other output.
    releaseInputs();
    useBLE=command.endsWith("ble"); selectedGeneration=generation; linkFault=false; reportFailed=false;
    if (outputReady()) releaseInputs();
    return reportFailed?"err selected output release failed":"ok";
  }
  if (command=="reset") { releaseInputs(); if (!reportFailed) linkFault=false; return reportFailed?"err release unconfirmed":"ok"; }
  String verb=command.substring(0,command.indexOf(' ')<0?command.length():command.indexOf(' '));
  if (verb=="home" || verb=="move" || verb=="flick") return "err relative control disabled";
  if (useBLE && selectedGeneration!=generation) linkFault=true;
  if (useBLE && linkFault) return "err BLE connection changed; inspect then explicitly reset or select output";
  if (useBLE && command!="ping" && command!="pace" && !bleReady()) return "err BLE HID not ready (connection/encryption/subscriptions)";
  // Bound a synchronous command below the PC's ten-second deadline.
  if (useBLE && command.startsWith("type ") && command.length()>21) return "err BLE text chunk limit 16";
  String reply=run(command);
  if (useBLE && (generation!=commandGeneration || reportFailed)) {
    linkFault=true; releaseInputs(); return "err BLE report failed or connection changed; outcome unknown; not replayed";
  }
  if (reportFailed) return "err HID report failed; outcome unknown; not replayed";
  return reply;
}
void setup() {
  Serial0.begin(115200);
  char id[17]; snprintf(id,sizeof(id),"%08lx%08lx",(unsigned long)esp_random(),(unsigned long)esp_random()); bootId=id;
  uint8_t mac[6]; esp_read_mac(mac,ESP_MAC_BT);
  for (auto b:mac) { char hex[3]; snprintf(hex,3,"%02x",b); deviceId+=hex; }
  mouse.begin(); usbKeyboard.begin(); usbConsumer.begin(); USB.productName("Handsy Serial HID"); USB.begin();
  NimBLEDevice::init("Handsy Serial HID");
  // Stable dedicated HID identity; never reuse the old command-service identity.
  uint8_t address[]={mac[5],mac[4],mac[3],mac[2],mac[1],uint8_t((mac[0]^0x35)|0xc0)};
  if (!NimBLEDevice::setOwnAddr(address) || !NimBLEDevice::setOwnAddrType(BLE_OWN_ADDR_RANDOM)) { Serial0.println("fatal BLE identity"); return; }
  NimBLEDevice::setSecurityIOCap(BLE_HS_IO_NO_INPUT_OUTPUT);
  NimBLEDevice::setSecurityAuth(true,false,true);
  auto *server=NimBLEDevice::createServer(); server->setCallbacks(new PeerCallbacks()); server->advertiseOnDisconnect(true);
  bleHid=new NimBLEHIDDevice(server); bleHid->setManufacturer("Handsy"); bleHid->setHidInfo(0,1);
  // HOGP requires a populated seven-byte PnP ID, not an empty characteristic.
  bleHid->setPnp(1,0xffff,1,1); // Development identity; no borrowed commercial keyboard ID.
  // Use the descriptors registered by the existing USB devices, unchanged.
  uint8_t map[512]; unsigned n=usbKeyboard._onGetDescriptor(map);
  n+=mouse._onGetDescriptor(map+n); n+=usbConsumer._onGetDescriptor(map+n);
  bleHid->setReportMap(map,n);
  for (unsigned id : {1u,4u,10u,11u,12u}) {
    inputs[id]=bleHid->getInputReport(id); inputs[id]->setCallbacks(new SubscriptionCallbacks());
    uint8_t zero[8]={}; unsigned length=id==1?8:id==4?2:id==10?5:id==11?1:4;
    inputs[id]->setValue(zero,length);
  }
  uint8_t zero=0; bleHid->getOutputReport(1)->setValue(&zero,1);
  bleHid->getHidControl()->setCallbacks(new SuspendCallbacks());
  bleHid->startServices(); server->start(); bleHid->setBatteryLevel(100);
  auto *adv=NimBLEDevice::getAdvertising();
  advertisingConfigured=adv->setAppearance(HID_KEYBOARD);
  advertisingConfigured=adv->addServiceUUID(bleHid->getHidService()->getUUID()) && advertisingConfigured;
  advertisingConfigured=adv->setName("Handsy Serial HID") && advertisingConfigured;
  adv->enableScanResponse(true);
  // Both packets identify this HID device. Keep connection hints out of scan response.
  advertisingConfigured=adv->setName("Handsy Serial HID") && advertisingConfigured;
  advertisingStarted=advertisingConfigured && NimBLEDevice::startAdvertising();
}
void loop() {
  if (useBLE && selectedGeneration!=generation) {
    selectedGeneration=generation;
    linkFault=true; reportFailed=false; releaseInputs();
  }
  if (holdUntil && (int32_t)(millis()-holdUntil)>=0) {
    reportFailed=false; clearing=true; mouse.release(0xff); clearing=false; holdUntil=0;
    if (reportFailed) linkFault=true;
  }
  static String line; static bool overflow=false;
  for (unsigned budget=0;budget<256 && Serial0.available();budget++) {
    char ch=Serial0.read();
    if (ch=='\n') {
      if (!overflow) {
        int sp=line.indexOf(' '); String id=sp<0?"":line.substring(0,sp);
        bool numeric=id.length(); for (unsigned i=0;i<id.length();i++) numeric &= isDigit(id[i]);
        if (numeric) Serial0.println(id+" "+dispatch(line.substring(sp+1)));
      }
      line=""; overflow=false;
    } else if (ch!='\r' && !overflow) { if (!ch || line.length()>=512) { overflow=true; line=""; } else line+=ch; }
  }
  delay(1);
}
