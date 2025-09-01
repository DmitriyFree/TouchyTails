#include <ArduinoBLE.h>
#include <WiFi.h>

// ==== CONFIG ====
#define DEVICE_NAME         "TouchyTails"
#define SERVICE_UUID        "0000ab00-0000-1000-8000-00805f9b34fb"
#define CHARACTERISTIC_UUID "0000ab01-0000-1000-8000-00805f9b34fb"
#define CHARACTERISTIC_SIZE 100

// ==== BLE Elements ====
BLEService Service(SERVICE_UUID);
BLEStringCharacteristic Characteristic(
  CHARACTERISTIC_UUID,
  BLERead | BLEWrite | BLENotify,
  CHARACTERISTIC_SIZE
);
BLEDescriptor CharacteristicDescriptor("2901", "Data");

// ==== STATE ====
float currentValue = 0.0;      // current output value [0..1]
unsigned long lastUpdate = 0;  // millis when last update arrived
const unsigned long durationLimit = 500; // ms until output goes to zero

// ==== SETUP ====

void setup() {
  btStop();
  setCpuFrequencyMhz(80);
  WiFi.mode(WIFI_OFF);
  Serial.begin(115200);
  
  initBLE();
  pinMode(0, OUTPUT); // motor
  pinMode(8, OUTPUT); // LED
  pinMode(1, OUTPUT); // buzzer
  digitalWrite(0, false);
  digitalWrite(8, true); // inverted idle

  // Print useful info
  Serial.print(F("My address: "));
  Serial.println(BLE.address());
  Serial.print(F("Main service: "));
  Serial.println(Service.uuid());
  Serial.print(F("Main characteristic: "));
  Serial.println(Characteristic.uuid());
}

void initBLE() {
  if (!BLE.begin()) {
    Serial.println("Failed to initialize BLE!");
    while (1);
  }

  BLE.setDeviceName(DEVICE_NAME);
  Characteristic.addDescriptor(CharacteristicDescriptor);
  Service.addCharacteristic(Characteristic);
  BLE.addService(Service);

  // Setup handler for writes from central
  Characteristic.setEventHandler(BLEWritten, onWrite);

  // Advertise with name and service
  BLEAdvertisingData scanData;
  scanData.setLocalName(DEVICE_NAME);
  BLE.setAdvertisedService(Service);
  BLE.setScanResponseData(scanData);

  BLE.advertise();
  delay(100); // give time to stabilize
}

// ==== BLE Event ====
void handleData(String data) {
  
  float value = data.toFloat();
  if(value <= 0)return; // no output for zero
  value = constrain(value, 0, 1.0); // clamp to [0,1]
  currentValue = value;  // store current value
  lastUpdate = millis();

  applyOutput(currentValue);
}

void onWrite(BLEDevice central, BLECharacteristic characteristic) {
  int len = characteristic.valueLength();
  const uint8_t* rawData = characteristic.value();

  String received = "";
  for (int i = 0; i < len; i++) {
    received += (char)rawData[i];
  }

  Serial.println("From BLE: " + received);

  handleData(received);
}

// ==== OUTPUT ====
void applyOutput(float value) {

  // PWM duty cycle 0–255
  int duty = (int)(value * 255.0);
  analogWrite(0, duty);
  analogWrite(8, 255-duty);

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
  BLE.poll();

  unsigned long now = millis();
  unsigned long elapsed = now - lastUpdate;

  if (elapsed >= durationLimit) {
    currentValue = 0.0;
    applyOutput(currentValue);
  }
  delay(100);
}
