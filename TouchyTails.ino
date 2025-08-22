#include <ArduinoBLE.h>
#include <Preferences.h>

// ==== CONFIG ====
#define DEVICE_NAME         "TouchyTails"
#define SERVICE_UUID        "0000ab00-0000-1000-8000-00805f9b34fb"
#define CHARACTERISTIC_UUID "0000ab01-0000-1000-8000-00805f9b34fb"
#define CHARACTERISTIC_SIZE 1000

Preferences preferences;

// ==== BLE Elements ====
BLEService Service(SERVICE_UUID);
BLEStringCharacteristic Characteristic(
  CHARACTERISTIC_UUID,
  BLERead | BLEWrite | BLENotify,
  CHARACTERISTIC_SIZE
);
BLEDescriptor CharacteristicDescriptor("2901", "Data");

// ==== SETUP ====

void setup() {
  Serial.begin(115200);
  preferences.begin("data", false);

  initBLE();
  pinMode(0, OUTPUT);//drive motor
  pinMode(8, OUTPUT);//LED motor
  digitalWrite(0,false);
  digitalWrite(8,true);//inverted

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

  // Restore saved data on reconnect
  String savedData = preferences.getString("data", "");
  Characteristic.writeValue(savedData.c_str());

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
void handleData(String data){
  if(data=="on"){
    digitalWrite(0,true);
    digitalWrite(8,false);
  } else {
    digitalWrite(0,false);
    digitalWrite(8,true);
  }
}
void onWrite(BLEDevice central, BLECharacteristic characteristic) {
  int len = characteristic.valueLength();
  const uint8_t* rawData = characteristic.value();

  String received = "";
  for (int i = 0; i < len; i++) {
    received += (char)rawData[i];
  }

  Serial.println("From BLE: " + received);
  Serial1.println(received);

  handleData(received);
}

// ==== MAIN LOOP ====

void loop() {
  BLE.poll();
}


