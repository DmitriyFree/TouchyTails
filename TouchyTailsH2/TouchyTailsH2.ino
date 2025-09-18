#include <NimBLEDevice.h>
#include <Adafruit_NeoPixel.h>

// ==== CONFIG ====
// BLE
#define DEVICE_NAME         "TouchyTails"
#define SERVICE_UUID        "0000ab00-0000-1000-8000-00805f9b34fb"
#define CHARACTERISTIC_UUID "0000ab01-0000-1000-8000-00805f9b34fb"

// Hardware pins
#define MOTOR_PIN    2
#define LED_PIN      13   // onboard indicator LED (inverted idle)
#define BUZZER_PIN   3
#define RGB_PIN      8    // WS2812 / NeoPixel
#define NUM_LEDS     1

// Timing (ms)
const unsigned long DURATION_LIMIT = 1000;   // stop output if no updates
const unsigned long WATCHDOG_LIMIT = 10000;  // reboot if no messages

// ==== STATE ====
Adafruit_NeoPixel strip(NUM_LEDS, RGB_PIN, NEO_GRB + NEO_KHZ800);

float currentValue = 0.0;      // current output value [0..1]
unsigned long lastUpdate = 0;  // last valid BLE update

// ==== BLE Elements ====
NimBLEServer* pServer;
NimBLEService* pService;
NimBLECharacteristic* pCharacteristic;

// ==== HELPERS ====
void applyOutput(float value) {
  // PWM duty cycle 0–255
  int duty = (int)(value * 255.0);
  analogWrite(MOTOR_PIN, duty);

  // Frequency 0–1000 Hz
  int freq = (int)(value * 1000.0);
  if (freq > 0) {
    tone(BUZZER_PIN, freq);
  } else {
    noTone(BUZZER_PIN);
  }
}

void resetRGB() {
  strip.begin();
  strip.setBrightness(50);
  strip.clear();
  strip.show();
}

// ==== BLE Event ====
void handleData(const String& data) {
  float value = data.toFloat();
  if (value <= 0) return; // ignore zeros
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
    handleData(received);
  }
} chrCallbacks;

class ServerCallbacks : public NimBLEServerCallbacks {
  void onDisconnect(NimBLEServer* s, NimBLEConnInfo& connInfo, int reason) override {
    Serial.println("BLE disconnected! Forcing reboot...");
    ESP.restart();
  }
} srvCallbacks;

// ==== BLE INIT ====
void initBLE() {
  NimBLEDevice::init(DEVICE_NAME);

  pServer = NimBLEDevice::createServer();
  pServer->setCallbacks(&srvCallbacks);

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
}

// ==== SETUP ====
void setup() {
  Serial.begin(115200);

  initBLE();
  resetRGB();

  pinMode(MOTOR_PIN, OUTPUT);
  pinMode(LED_PIN, OUTPUT);
  pinMode(BUZZER_PIN, OUTPUT);

  digitalWrite(MOTOR_PIN, LOW);
  digitalWrite(LED_PIN, LOW); // inverted idle

  Serial.print("My address: ");
  Serial.println(NimBLEDevice::getAddress().toString().c_str());
}

// ==== LOOP ====
void loop() {
  unsigned long now = millis();

  // 1) auto-zero if inactive
  if (currentValue != 0 && (now - lastUpdate) >= DURATION_LIMIT) {
    currentValue = 0.0;
    applyOutput(currentValue);
  }

  // 2) watchdog reboot
  if ((now - lastUpdate) >= WATCHDOG_LIMIT) {
    Serial.println("No BLE data for too long! Rebooting...");
    ESP.restart();
  }

  // --- Light sleep until next tick ---
  esp_sleep_enable_timer_wakeup(1000 * 1000); // 1s
  esp_light_sleep_start();
}
