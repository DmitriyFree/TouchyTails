// TouchyTails - nRF52840 Super Mini (nice!nano compatible), battery-powered port
// Framework: Arduino, Adafruit nRF52 core (Bluefruit52Lib, SoftDevice S140, FreeRTOS)
//
// Power design: fully event-driven. loop() is suspended, there is no polling and no delay().
// Everything runs from BLE callbacks and FreeRTOS software timers, so the CPU sits in
// WFE sleep (a few uA) between BLE connection events.

#include <Arduino.h>
#include <bluefruit.h>
#include <string.h>
#include <stdlib.h>

// ==== CONFIG ====
#define DEVICE_NAME      "TouchyTails"
// Same UUIDs as before (0000ab00-0000-1000-8000-00805f9b34fb / ...ab01...), as 16-bit
// shorthand. Equivalent for hosts, and keeps the advertising packet small.
#define SERVICE_UUID16   0xAB00
#define CHAR_UUID16      0xAB01

// Pins: Arduino pin numbers of the variant, NOT raw GPIO numbers. Check your pinout.
#define MOTOR_PIN        2          // PWM out to motor MOSFET/driver (gate needs a pulldown!)
#define LED_PIN          LED_BUILTIN
#define LED_ON_LEVEL     HIGH       // set LOW if your board's LED is active-low

// Behaviour (unchanged from ESP32 version)
#define DURATION_LIMIT   500        // ms: motor off if no new value arrives
#define WATCHDOG_LIMIT   6000       // ms: drop link if host silent (it pings every 2 s)

// ---- Power knobs ----
#define TX_POWER_DBM         0      // -8..+8; lower saves a little, costs range
// Connection params requested from the host (host may override). Units: 1.25 ms.
// Write latency to the motor is roughly one connection interval, so 20-40 ms is fine.
#define CONN_INTERVAL_MIN    16     // 20 ms
#define CONN_INTERVAL_MAX    32     // 40 ms
#define SLAVE_LATENCY        0      // >0 saves power but adds (latency+1)*interval of write delay
#define SUPERVISION_TIMEOUT  400    // units of 10 ms -> 4 s
// Advertising: fast for a while after boot/disconnect, then slow. Units: 0.625 ms.
#define ADV_FAST_INTERVAL    160    // 100 ms
#define ADV_SLOW_INTERVAL    1600   // 1 s
#define ADV_FAST_TIMEOUT_S   30
#define USE_DCDC             1      // nice!nano-style boards have the DCDC inductor. If BLE
                                    // does not work at all after flashing, set this to 0.
// Optional deep sleep (System OFF, ~uA) after N minutes with no connection.
// Wake by pulling WAKE_PIN to GND (button). 0 = disabled.
#define IDLE_OFF_MINUTES     0
#define WAKE_PIN             3

// ---- Debug / LED ----
#define DEBUG_SERIAL         0      // keep 0 on battery
#define PING_BLINK           0      // 1 = tiny blink on every host ping (costs a few uA)
#define ADV_BLINK_PERIOD_MS  4000   // blink period while waiting for a connection

#if DEBUG_SERIAL
  #define DBG(...) Serial.printf(__VA_ARGS__)
#else
  #define DBG(...)
#endif

// ==== BLE objects ====
BLEService        svc(SERVICE_UUID16);
BLECharacteristic chr(CHAR_UUID16);

// ==== STATE ====
static SoftwareTimer motorTimer, wdogTimer, ledTimer, advBlinkTimer, offTimer;
static SemaphoreHandle_t motorMx;
static volatile uint16_t connHandle = BLE_CONN_HANDLE_INVALID;
static bool pwmRunning = false;

// ==== LED (status blinks) ====
// A pattern is a list of durations in ms: ON, OFF, ON, OFF, ... (LED ends OFF).
static const uint16_t SEQ_BOOT[]      = {80, 120, 80, 120, 80};  // 3 blinks: powered up
static const uint16_t SEQ_CONNECTED[] = {40, 120, 40};           // 2 blinks: link established
static const uint16_t SEQ_ADV[]       = {15};                    // 1 short blink: waiting for host
static const uint16_t SEQ_WATCHDOG[]  = {400};                   // 1 long blink: dropped silent host
static const uint16_t SEQ_PING[]      = {8};

static const uint16_t* ledSeq = nullptr;
static uint8_t ledLen = 0, ledIdx = 0;

static inline void setLed(bool on) {
  digitalWrite(LED_PIN, on ? LED_ON_LEVEL : !LED_ON_LEVEL);
}

static void ledStepCb(TimerHandle_t) {
  if (ledIdx >= ledLen) { setLed(false); return; }
  setLed((ledIdx & 1) == 0);               // even steps = ON, odd = OFF
  ledTimer.setPeriod(ledSeq[ledIdx++]);    // re-arms the one-shot timer for this step
}

static void ledPlay(const uint16_t* seq, uint8_t len) {
  ledSeq = seq; ledLen = len; ledIdx = 0;
  ledTimer.setPeriod(1);                   // kick: first step runs in the timer task
}
#define LED_PLAY(s) ledPlay(s, sizeof(s) / sizeof(s[0]))

static void advBlinkCb(TimerHandle_t) { LED_PLAY(SEQ_ADV); }

// ==== MOTOR ====
// PWM peripheral is only enabled while the motor is actually running.
static void motorSet(float value) {
  int duty = (int)(value * 255.0f);
  xSemaphoreTake(motorMx, portMAX_DELAY);
  if (duty <= 0) {
    if (pwmRunning) { HwPWM0.stop(); pwmRunning = false; }
    digitalWrite(MOTOR_PIN, LOW);
  } else {
    if (!pwmRunning) { HwPWM0.begin(); pwmRunning = true; }
    HwPWM0.writePin(MOTOR_PIN, duty);
  }
  xSemaphoreGive(motorMx);
}

static void motorTimeoutCb(TimerHandle_t) { motorSet(0.0f); }

// ==== OPTIONAL DEEP SLEEP ====
#if IDLE_OFF_MINUTES > 0
static void offCb(TimerHandle_t) {
  motorSet(0.0f);
  setLed(false);
  nrf_gpio_cfg_sense_input(g_ADigitalPinMap[WAKE_PIN], NRF_GPIO_PIN_PULLUP, NRF_GPIO_PIN_SENSE_LOW);
  sd_power_system_off();                   // wake = full reset via WAKE_PIN low
}
#endif

// ==== BLE callbacks ====
static void wdogCb(TimerHandle_t) {
  DBG("Link silent, dropping connection\n");
  LED_PLAY(SEQ_WATCHDOG);
  if (connHandle != BLE_CONN_HANDLE_INVALID) Bluefruit.disconnect(connHandle);
}

static void onWrite(uint16_t, BLECharacteristic*, uint8_t* data, uint16_t len) {
  if (len == 0) return;
  char buf[24];
  if (len > sizeof(buf) - 1) len = sizeof(buf) - 1;
  memcpy(buf, data, len);
  buf[len] = 0;

  wdogTimer.reset();                       // any write, including ping, proves host is alive

  if (strcmp(buf, "ping") == 0) {
#if PING_BLINK
    LED_PLAY(SEQ_PING);
#endif
    return;
  }

  float v = strtof(buf, nullptr);
  if (v < 0) return;
  if (v > 1.0f) v = 1.0f;
  motorSet(v);
  if (v > 0) motorTimer.reset(); else motorTimer.stop();
}

static void connectCb(uint16_t handle) {
  connHandle = handle;
  advBlinkTimer.stop();
#if IDLE_OFF_MINUTES > 0
  offTimer.stop();
#endif
  wdogTimer.reset();
  LED_PLAY(SEQ_CONNECTED);
  DBG("Connected\n");
}

static void disconnectCb(uint16_t, uint8_t reason) {
  connHandle = BLE_CONN_HANDLE_INVALID;
  wdogTimer.stop();
  motorTimer.stop();
  motorSet(0.0f);                          // never leave the motor running
  advBlinkTimer.reset();
#if IDLE_OFF_MINUTES > 0
  offTimer.reset();
#endif
  DBG("Disconnected (reason 0x%02X), advertising again\n", reason);
  // Advertising restarts automatically (restartOnDisconnect)
}

// ==== SETUP ====
void setup() {
#if DEBUG_SERIAL
  Serial.begin(115200);
#endif

  pinMode(LED_PIN, OUTPUT);
  setLed(false);
  pinMode(MOTOR_PIN, OUTPUT);
  digitalWrite(MOTOR_PIN, LOW);

  motorMx = xSemaphoreCreateMutex();
  HwPWM0.addPin(MOTOR_PIN);
  HwPWM0.setResolution(8);
  HwPWM0.setClockDiv(PWM_PRESCALER_PRESCALER_DIV_64);  // 16 MHz / 64 / 256 = ~1 kHz

  motorTimer.begin(DURATION_LIMIT, motorTimeoutCb, NULL, false);
  wdogTimer.begin(WATCHDOG_LIMIT, wdogCb, NULL, false);
  ledTimer.begin(1000, ledStepCb, NULL, false);
  advBlinkTimer.begin(ADV_BLINK_PERIOD_MS, advBlinkCb, NULL, true);
#if IDLE_OFF_MINUTES > 0
  offTimer.begin(IDLE_OFF_MINUTES * 60000UL, offCb, NULL, false);
#endif

  Bluefruit.autoConnLed(false);            // otherwise the core blinks the LED on its own
  Bluefruit.begin(1, 0);                   // 1 peripheral link, no central
  Bluefruit.setTxPower(TX_POWER_DBM);
  Bluefruit.setName(DEVICE_NAME);
#if USE_DCDC
  sd_power_dcdc_mode_set(NRF_POWER_DCDC_ENABLE);
#endif

  Bluefruit.Periph.setConnectCallback(connectCb);
  Bluefruit.Periph.setDisconnectCallback(disconnectCb);
  Bluefruit.Periph.setConnInterval(CONN_INTERVAL_MIN, CONN_INTERVAL_MAX);
  Bluefruit.Periph.setConnSlaveLatency(SLAVE_LATENCY);
  Bluefruit.Periph.setConnSupervisionTimeout(SUPERVISION_TIMEOUT);

  svc.begin();
  chr.setProperties(CHR_PROPS_READ | CHR_PROPS_WRITE | CHR_PROPS_WRITE_WO_RESP | CHR_PROPS_NOTIFY);
  chr.setPermission(SECMODE_OPEN, SECMODE_OPEN);
  chr.setMaxLen(20);
  chr.setWriteCallback(onWrite);
  chr.begin();

  Bluefruit.Advertising.addFlags(BLE_GAP_ADV_FLAGS_LE_ONLY_GENERAL_DISC_MODE);
  Bluefruit.Advertising.addService(svc);
  Bluefruit.Advertising.addName();
  Bluefruit.Advertising.restartOnDisconnect(true);
  Bluefruit.Advertising.setInterval(ADV_FAST_INTERVAL, ADV_SLOW_INTERVAL);
  Bluefruit.Advertising.setFastTimeout(ADV_FAST_TIMEOUT_S);
  Bluefruit.Advertising.start(0);          // 0 = advertise forever (fast, then slow)

  LED_PLAY(SEQ_BOOT);
  advBlinkTimer.reset();
#if IDLE_OFF_MINUTES > 0
  offTimer.reset();
#endif

  suspendLoop();                           // nothing to poll: let the idle task sleep the CPU
}

void loop() {}