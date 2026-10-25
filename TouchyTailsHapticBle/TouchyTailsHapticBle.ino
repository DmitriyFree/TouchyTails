#include <NimBLEDevice.h>
#include <Adafruit_NeoPixel.h>
#include <Wire.h>
#include <Adafruit_DRV2605.h>

// ==== CONFIG ====

// BLE
#define DEVICE_NAME         "TouchyTails"
#define SERVICE_UUID        "0000ab00-0000-1000-8000-00805f9b34fb"
#define CHARACTERISTIC_UUID "0000ab01-0000-1000-8000-00805f9b34fb"

// Hardware
#define LED_PIN      13
#define RGB_PIN      10
#define NUM_LEDS     1

#define LRA_RATED_VOLTAGE 1.2f

// Timing
const unsigned long DURATION_LIMIT = 500;
// Host pings every 2 s. If nothing (data or ping) arrives for this long,
// drop the link so the host reconnects. Never reboot on idle.
const unsigned long LINK_TIMEOUT = 6000;

// =========================
// GLOBALS
// =========================

Adafruit_DRV2605 drv;
Adafruit_NeoPixel strip(NUM_LEDS, RGB_PIN, NEO_GRB + NEO_KHZ800);

NimBLEServer* pServer;
NimBLEService* pService;
NimBLECharacteristic* pCharacteristic;

volatile float currentValue = 0.0;
volatile unsigned long lastUpdate = 0;   // last haptic value
volatile unsigned long lastRx = 0;       // last write of any kind (incl. ping)
volatile uint16_t connHandle = 0;
bool playing = false;
unsigned long playStart = 0;

enum ConnState { DISCONNECTED, CONNECTED };
volatile ConnState connState = DISCONNECTED;

// =========================
// HAPTIC ENGINE
// =========================

class HapticEngine {
public:
  void begin() {
    if (!drv.begin()) {
      // Keep BLE alive so the problem is visible from the host side
      Serial.println("DRV2605 not found, haptics disabled");
      ok = false;
      return;
    }
    ok = true;

    drv.selectLibrary(6);
    drv.setMode(DRV2605_MODE_INTTRIG);

    uint8_t rated = (LRA_RATED_VOLTAGE / 5.6f) * 255;
    drv.writeRegister8(0x16, rated);
    drv.writeRegister8(0x17, rated);
    drv.writeRegister8(0x1D, 0x00); // disable boost

    randomSeed(analogRead(0));
  }

  bool ok = false;

  void setEntropy(float e) {
    entropy = constrain(e, 0.0f, 1.0f);
    active = (entropy > 0.01f);
  }

  void update() {
    if (!ok || !active) return;
    if (playing) {
      if (millis() - playStart > 20) {  // waveform length safety
        playing = false;
      } else {
        return;
      }
    }


    unsigned long now = millis();

    // ===== P3-STYLE FAST NOISE WITH ENTROPY =====

    phase += 0.25f + entropy * 0.5f;

    int baseInterval = 40 - (entropy * 30);          // denser with entropy
    int noiseRange  = 10 + (entropy * 40);           // more chaos

    int interval = baseInterval +
                   15 * sin(phase) +
                   random(-noiseRange, noiseRange);

    interval = constrain(interval, 18, 120);

    if (now - lastEvent >= interval) {
      strongClick();
      lastEvent = now;
    }
  }

  bool isActive() const { return active; }

private:
  float entropy = 0.0f;
  bool active = false;
  unsigned long lastEvent = 0;
  float phase = 0;

  void strongClick() {
    if (playing) return;

    drv.setWaveform(0, 1);
    drv.setWaveform(1, 0);
    drv.go();

    playing = true;
    playStart = millis();
  }

};

HapticEngine haptic;

// =========================
// BLE CALLBACKS
// =========================

void handleData(const String& data) {
  float value = data.toFloat();
  if (value < 0) return;

  currentValue = constrain(value, 0, 1.0);
  lastUpdate = millis();

  haptic.setEntropy(currentValue);
}

class CharacteristicCallbacks : public NimBLECharacteristicCallbacks {
  void onWrite(NimBLECharacteristic* c, NimBLEConnInfo&) override {
    std::string value = c->getValue();
    if (value.empty()) return;

    lastRx = millis();
    String received(value.c_str());

    if (received == "ping") return;

    handleData(received);
  }
} chrCallbacks;

class ServerCallbacks : public NimBLEServerCallbacks {
  void onConnect(NimBLEServer*, NimBLEConnInfo& info) override {
    connHandle = info.getConnHandle();
    lastRx = millis();
    connState = CONNECTED;
    Serial.println("BLE connected");
  }
  void onDisconnect(NimBLEServer*, NimBLEConnInfo&, int reason) override {
    connState = DISCONNECTED;
    currentValue = 0;
    lastUpdate = 0;
    Serial.printf("BLE disconnected, reason %d\n", reason);
    NimBLEDevice::startAdvertising();  // accept the next connection, no reboot
  }
};

// =========================
// BLE INIT
// =========================

void initBLE() {
  NimBLEDevice::init(DEVICE_NAME);

  pServer = NimBLEDevice::createServer();
  pServer->setCallbacks(new ServerCallbacks());

  pService = pServer->createService(SERVICE_UUID);
  pCharacteristic = pService->createCharacteristic(
    CHARACTERISTIC_UUID,
    NIMBLE_PROPERTY::READ |
    NIMBLE_PROPERTY::WRITE |
    NIMBLE_PROPERTY::WRITE_NR |
    NIMBLE_PROPERTY::NOTIFY
  );

  pCharacteristic->setCallbacks(&chrCallbacks);
  pService->start();

  NimBLEAdvertising* adv = NimBLEDevice::getAdvertising();
  NimBLEAdvertisementData advertisement;
  advertisement.setName(DEVICE_NAME);
  advertisement.addServiceUUID(SERVICE_UUID);
  adv->setAdvertisementData(advertisement);
  adv->start();

  Serial.println("BLE started");
}

// =========================
// LED (UNCHANGED LOGIC CORE)
// =========================

void updateLEDs() {
  if (connState == DISCONNECTED) {
    analogWrite(LED_PIN, 30);
    strip.clear();
    strip.show();
    return;
  }

  strip.setPixelColor(0, strip.Color(0, 0, 50));
  strip.show();
}

// =========================
// SETUP
// =========================

void setup() {
  Serial.begin(115200);
  Serial.println("point 1");
  //pinMode(LED_PIN, OUTPUT);

  //strip.begin();
  //strip.show();

  Wire.begin();
  haptic.begin();
  

  initBLE();
  Serial.println("ready");
}

// =========================
// LOOP
// =========================

void loop() {
  unsigned long now = millis();

  if (currentValue != 0 && (now - lastUpdate) >= DURATION_LIMIT) {
    currentValue = 0.0;
    haptic.setEntropy(0);
  }

  if (currentValue == 0 && haptic.isActive()) {
    haptic.setEntropy(0);  // value was cleared from the BLE callback
  }

  if (connState == CONNECTED && (now - lastRx) >= LINK_TIMEOUT) {
    Serial.println("Link silent, dropping connection");
    lastRx = now;
    pServer->disconnect(connHandle);
  }

  haptic.update();
  //updateLEDs();

  delay(2);
  yield();
}
