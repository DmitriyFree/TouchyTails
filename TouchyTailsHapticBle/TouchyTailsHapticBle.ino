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
const unsigned long WATCHDOG_LIMIT = 3000;

// =========================
// GLOBALS
// =========================

Adafruit_DRV2605 drv;
Adafruit_NeoPixel strip(NUM_LEDS, RGB_PIN, NEO_GRB + NEO_KHZ800);

NimBLEServer* pServer;
NimBLEService* pService;
NimBLECharacteristic* pCharacteristic;

float currentValue = 0.0;
unsigned long lastUpdate = 0;
bool playing = false;
unsigned long playStart = 0;

enum ConnState { DISCONNECTED, CONNECTED };
ConnState connState = DISCONNECTED;

// =========================
// HAPTIC ENGINE
// =========================

class HapticEngine {
public:
  void begin() {
    if (!drv.begin()) {
      Serial.println("DRV2605 not found");
      while (1);
    }

    drv.selectLibrary(6);
    drv.setMode(DRV2605_MODE_INTTRIG);

    uint8_t rated = (LRA_RATED_VOLTAGE / 5.6f) * 255;
    drv.writeRegister8(0x16, rated);
    drv.writeRegister8(0x17, rated);
    drv.writeRegister8(0x1D, 0x00); // disable boost

    randomSeed(analogRead(0));
  }

  void setEntropy(float e) {
    entropy = constrain(e, 0.0f, 1.0f);
    active = (entropy > 0.01f);
  }

  void update() {
    if (!active) return;
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

    String received(value.c_str());
    Serial.println("From BLE: " + received);

    if (received == "ping") return;

    handleData(received);
  }
} chrCallbacks;

class ServerCallbacks : public NimBLEServerCallbacks {
  void onConnect(NimBLEServer*, NimBLEConnInfo&) override {
    connState = CONNECTED;
  }
  void onDisconnect(NimBLEServer*, NimBLEConnInfo&, int) override {
    connState = DISCONNECTED;
    ESP.restart();
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

  if (lastUpdate && ((now - lastUpdate) >= WATCHDOG_LIMIT)) {
    ESP.restart();
  }

  haptic.update();
  //updateLEDs();

  delay(2);
  yield();
}
