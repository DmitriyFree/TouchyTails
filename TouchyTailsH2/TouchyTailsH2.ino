#include <NimBLEDevice.h>

// ==== CONFIG ====
#define DEVICE_NAME         "TouchyTails"
#define SERVICE_UUID        "0000ab00-0000-1000-8000-00805f9b34fb"
#define CHARACTERISTIC_UUID "0000ab01-0000-1000-8000-00805f9b34fb"

// ==== STATE ====
float currentValue = 0.0;      // current output value [0..1]
unsigned long lastUpdate = 0;  // millis when last update arrived
const unsigned long durationLimit = 500; // ms until output goes to zero

// ==== BLE Elements ====
NimBLEServer* pServer;
NimBLEService* pService;
NimBLECharacteristic* pCharacteristic;
// ==== BLE Event ====
void handleData(String data) {
  float value = data.toFloat();
  if (value <= 0) return; // no output for zero
  value = constrain(value, 0, 1.0); // clamp to [0,1]
  currentValue = value;
  lastUpdate = millis();

  applyOutput(currentValue);
}
// ==== CALLBACKS ====
class CharacteristicCallbacks : public NimBLECharacteristicCallbacks {
  void onWrite(NimBLECharacteristic* c) {
    std::string value = c->getValue();
    if (value.empty()) return;

    String received = String(value.c_str());
    Serial.println("From BLE: " + received);
    handleData(received);
  }
};

// ==== SETUP ====
void setup() {
  Serial.begin(115200);

  initBLE();

  pinMode(0, OUTPUT); // motor
  pinMode(13, OUTPUT); // LED
  pinMode(1, OUTPUT); // buzzer
  digitalWrite(0, LOW);
  digitalWrite(13, LOW); // inverted idle

  // Print useful info
  Serial.print(F("My address: "));
  Serial.println(NimBLEDevice::getAddress().toString().c_str());
  Serial.print(F("Main service: "));
  Serial.println(SERVICE_UUID);
  Serial.print(F("Main characteristic: "));
  Serial.println(CHARACTERISTIC_UUID);
}

void initBLE() {
  NimBLEDevice::init(DEVICE_NAME);

  pServer = NimBLEDevice::createServer();
  pService = pServer->createService(SERVICE_UUID);

  pCharacteristic = pService->createCharacteristic(
    CHARACTERISTIC_UUID,
    NIMBLE_PROPERTY::READ | NIMBLE_PROPERTY::WRITE | NIMBLE_PROPERTY::NOTIFY
  );

  pCharacteristic->setCallbacks(new CharacteristicCallbacks());
  pService->start();

  NimBLEAdvertising* pAdvertising = NimBLEDevice::getAdvertising();
  pAdvertising->addServiceUUID(SERVICE_UUID);
  
  // create a scan response object
  NimBLEAdvertisementData scanResponse;
  scanResponse.setName(DEVICE_NAME);      // set device name for scan response
  pAdvertising->setScanResponseData(scanResponse);
  pAdvertising->start();

  Serial.println("BLE started, advertising...");
}



// ==== OUTPUT ====
void applyOutput(float value) {
  // PWM duty cycle 0–255
  int duty = (int)(value * 255.0);
  analogWrite(0, duty);
  if(value>0)digitalWrite(13, HIGH);
  else digitalWrite(13, LOW);

  // Frequency 0–1000 Hz
  int freq = (int)(value * 1000.0);
  if (freq > 0) {
    tone(1, freq);
  } else {
    noTone(1);
  }
}

// ==== MAIN LOOP ====
void loop() {
  unsigned long now = millis();
  unsigned long elapsed = now - lastUpdate;

  if (elapsed >= durationLimit) {
    currentValue = 0.0;
    applyOutput(currentValue);
  }
  delay(100);
}
