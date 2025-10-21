#include <NimBLEDevice.h>
#include <Adafruit_NeoPixel.h>

// ==== CONFIG ====
// BLE
#define DEVICE_NAME         "TouchyTails"
#define SERVICE_UUID        "0000ab00-0000-1000-8000-00805f9b34fb"
#define CHARACTERISTIC_UUID "0000ab01-0000-1000-8000-00805f9b34fb"

// Hardware pins
#define MOTOR_PIN    2
#define LED_PIN      13   // onboard blue status LED
#define BUZZER_PIN   3
#define RGB_PIN      8    // WS2812 / NeoPixel
#define NUM_LEDS     1

// ==== BLE Elements ====
NimBLEServer* pServer;
NimBLEService* pService;
NimBLECharacteristic* pCharacteristic;

// Timing (ms)
const unsigned long DURATION_LIMIT = 500;
const unsigned long WATCHDOG_LIMIT = 5000;

// ==== STATE ====
Adafruit_NeoPixel strip(NUM_LEDS, RGB_PIN, NEO_GRB + NEO_KHZ800);

float currentValue = 0.0;
unsigned long lastUpdate = 0;

// ==== LED STATE ====
enum ConnState { DISCONNECTED, CONNECTED };
ConnState connState = DISCONNECTED;

bool pingFlashActive = false;
unsigned long lastPingFlash = 0;

bool hapticActiveFlag = false;
int hapticIntensity = 0;
unsigned long lastHapticToggle = 0;
bool hapticFlicker = false;

uint16_t rainbowOffset = 0;
unsigned long lastRainbowUpdate = 0;

// ==== TIMING ====
#define PING_FLASH_DURATION       80    // ms
#define RAINBOW_INTERVAL          30    // ms
#define HAPTIC_FLICKER_INTERVAL   30    // ms
#define BREATH_INTERVAL           15    // ms (controls breathing update rate)
#define BREATH_PERIOD             3000  // ms for one full breath cycle

// ==== FUNCTIONS ====
void setStatusConnected() {
  connState = CONNECTED;
}

void setStatusDisconnected() {
  connState = DISCONNECTED;
}

void pingFlash() {
  pingFlashActive = true;
  lastPingFlash = millis();
}

void hapticActive(int intensity) {
  hapticActiveFlag = true;
  hapticIntensity = constrain(intensity, 0, 255);
}

void hapticStop() {
  hapticActiveFlag = false;
}

// rainbow generator
uint32_t Wheel(byte WheelPos) {
  WheelPos = 255 - WheelPos;
  if (WheelPos < 85) {
    return strip.Color(255 - WheelPos * 3, 0, WheelPos * 3);
  }
  if (WheelPos < 170) {
    WheelPos -= 85;
    return strip.Color(0, WheelPos * 3, 255 - WheelPos * 3);
  }
  WheelPos -= 170;
  return strip.Color(WheelPos * 3, 255 - WheelPos * 3, 0);
}

// ==== HELPERS ====
void applyOutput(float value) {
  int duty = (int)(value * 255.0);
  analogWrite(MOTOR_PIN, duty);

  int freq = (int)(value * 1000.0);
  if (freq > 0) tone(BUZZER_PIN, freq);
  else noTone(BUZZER_PIN);

  if (value > 0) hapticActive((int)(value * 200));
  else hapticStop();
}

void resetRGB() {
  strip.begin();
  strip.clear();
  strip.show();
}

// ==== BLE Event ====
void handleData(const String& data) {
  float value = data.toFloat();
  if (value < 0) return;
  currentValue = constrain(value, 0, 1.0);
  lastUpdate = millis();
  applyOutput(currentValue);
}

// ==== CALLBACKS ====
class CharacteristicCallbacks : public NimBLECharacteristicCallbacks {
  void onWrite(NimBLECharacteristic* c, NimBLEConnInfo& connInfo) override {
    std::string value = c->getValue();
    if (value.empty()) return;
    String received(value.c_str());
    Serial.println("From BLE: " + received);

    if (received == "ping") {
      pingFlash();
      return;
    }
    handleData(received);
  }
} chrCallbacks;

class ServerCallbacks : public NimBLEServerCallbacks {
  void onConnect(NimBLEServer* pServer, NimBLEConnInfo& connInfo) override {
    setStatusConnected();
  }
  void onDisconnect(NimBLEServer* pServer, NimBLEConnInfo& connInfo, int reason) override {
    setStatusDisconnected();
    //reboot
    Serial.println("Disconnected! Rebooting...");
    ESP.restart();
  }
};

// ==== BLE INIT ====
void initBLE() {
  NimBLEDevice::init(DEVICE_NAME);

  pServer = NimBLEDevice::createServer();
  pServer->setCallbacks(new ServerCallbacks());

  pService = pServer->createService(SERVICE_UUID);
  pCharacteristic = pService->createCharacteristic(
    CHARACTERISTIC_UUID,
    NIMBLE_PROPERTY::READ | NIMBLE_PROPERTY::WRITE |
    NIMBLE_PROPERTY::WRITE_NR | NIMBLE_PROPERTY::NOTIFY
  );
  pCharacteristic->setCallbacks(&chrCallbacks);
  pService->start();

  NimBLEAdvertising* pAdvertising = NimBLEDevice::getAdvertising();
  NimBLEAdvertisementData advertisement;
  advertisement.setName(DEVICE_NAME);
  advertisement.addServiceUUID(SERVICE_UUID);
  pAdvertising->setAdvertisementData(advertisement);

  NimBLEAdvertisementData scanResponse;
  scanResponse.setName(DEVICE_NAME);
  pAdvertising->setScanResponseData(scanResponse);

  pAdvertising->start();
  Serial.println("BLE started, advertising...");
  Serial.print("My address: ");
  Serial.println(NimBLEDevice::getAddress().toString().c_str());
}

void initPins(){
  pinMode(MOTOR_PIN, OUTPUT);
  pinMode(LED_PIN, OUTPUT);
  pinMode(BUZZER_PIN, OUTPUT);

  digitalWrite(MOTOR_PIN, LOW);
  analogWrite(LED_PIN, 0);
}

// ==== LED UPDATE ====
void updateLEDs() {
  unsigned long now = millis();

  // --- Blue status LED ---
  static unsigned long lastBreathUpdate = 0;
  static float breathPhase = 0.0;

  if (connState == DISCONNECTED) {
    // Breathing effect between 10% and 30%
    if (now - lastBreathUpdate > BREATH_INTERVAL) {
      lastBreathUpdate = now;
      breathPhase += (2.0 * PI / BREATH_PERIOD) * BREATH_INTERVAL;
      if (breathPhase > 2 * PI) breathPhase -= 2 * PI;
      float breath = (sin(breathPhase) + 1.0) / 2.0; // 0..1
      int brightness = 25 + (int)(breath * 26); // range ~10%–30%
      analogWrite(LED_PIN, brightness);
    }
  } else {
    // Connected: LED off, unless ping flash
    if (pingFlashActive) {
      if (now - lastPingFlash < PING_FLASH_DURATION) {
        analogWrite(LED_PIN, 200); // bright flash
      } else {
        pingFlashActive = false;
        analogWrite(LED_PIN, 0);
      }
    } else {
      analogWrite(LED_PIN, 0);
    }
  }

  // --- RGB LED ---
  if (connState == DISCONNECTED) {
    strip.clear();
    strip.show();
    return;
  }

  // Connected: normal rainbow + haptic flicker
  uint32_t color = 0;
  if (now - lastRainbowUpdate > RAINBOW_INTERVAL) {
    rainbowOffset++;
    lastRainbowUpdate = now;
  }
  color = Wheel(rainbowOffset & 255);

  if (hapticActiveFlag) {
    if (now - lastHapticToggle > HAPTIC_FLICKER_INTERVAL) {
      hapticFlicker = !hapticFlicker;
      lastHapticToggle = now;
    }
    if (hapticFlicker) {
      int r = (int)((color >> 16) & 0xFF);
      int g = (int)((color >> 8) & 0xFF);
      int b = (int)(color & 0xFF);
      r = min(255, r + hapticIntensity);
      g = min(255, g + hapticIntensity);
      b = min(255, b + hapticIntensity);
      color = strip.Color(r, g, b);
    }
  }

  strip.setPixelColor(0, color);
  strip.show();
}

// ==== SETUP ====
void setup() {
  Serial.begin(115200);
  initPins();
  initBLE();
  resetRGB();
}

// ==== LOOP ====
void loop() {
  unsigned long now = millis();

  if (currentValue != 0 && (now - lastUpdate) >= DURATION_LIMIT) {
    currentValue = 0.0;
    applyOutput(currentValue);
  }

  if (lastUpdate && ((now - lastUpdate) >= WATCHDOG_LIMIT)) {
    Serial.println("No BLE data for too long! Rebooting...");
    ESP.restart();
  }

  updateLEDs();
  delay(5);
}
