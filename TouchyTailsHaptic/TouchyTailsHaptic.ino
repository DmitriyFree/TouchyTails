#include <Wire.h>
#include <Adafruit_DRV2605.h>

Adafruit_DRV2605 drv;

// =========================
// Configuration
// =========================

#define LRA_RATED_VOLTAGE 1.2f

enum HapticPattern {
  PATTERN_OFF = 0,
  PATTERN_P1_FAST_CLUSTER = 1,
  PATTERN_P3_FAST_NOISE = 3,
};

// =========================
// Haptic Engine
// =========================

class HapticEngine {
public:
  void begin() {
    if (!drv.begin()) {
      Serial.println("DRV2605 not found");
      while (1);
    }

    drv.selectLibrary(6);                  // LRA library
    drv.setMode(DRV2605_MODE_INTTRIG);

    configureSafeLRA(LRA_RATED_VOLTAGE);

    randomSeed(analogRead(0));

    Serial.println("Haptic Engine Ready");
  }

  void setPattern(HapticPattern p) {
    pattern = p;
  }

  void update() {
    unsigned long now = millis();

    switch (pattern) {

      case PATTERN_P1_FAST_CLUSTER:
        updateFastCluster(now);
        break;

      case PATTERN_P3_FAST_NOISE:
        updateFastNoise(now);
        break;

      case PATTERN_OFF:
      default:
        break;
    }
  }

private:
  HapticPattern pattern = PATTERN_OFF;
  unsigned long lastEvent = 0;

  // Cluster state
  int clusterRemaining = 0;
  unsigned long clusterTimer = 0;

  // Noise state
  float phase = 0;

  void strongClick() {
    drv.setWaveform(0, 1);
    drv.setWaveform(1, 0);
    drv.go();
  }

  void configureSafeLRA(float ratedVoltage) {
    uint8_t rated = (ratedVoltage / 5.6f) * 255;
    drv.writeRegister8(0x16, rated);   // Rated voltage
    drv.writeRegister8(0x17, rated);   // Overdrive clamp
    drv.writeRegister8(0x1D, 0x00);    // Disable boost
  }

  void updateFastCluster(unsigned long now) {
    if (clusterRemaining == 0) {
      if (now - lastEvent > random(80, 200)) {
        clusterRemaining = random(4, 10);
      }
    }

    if (clusterRemaining > 0 && now - clusterTimer > random(8, 25)) {
      strongClick();
      clusterTimer = now;
      lastEvent = now;
      clusterRemaining--;
    }
  }

  void updateFastNoise(unsigned long now) {
    phase += 0.25f;

    int interval = 25 + 15 * sin(phase) + random(-10, 10);
    interval = constrain(interval, 6, 60);

    if (now - lastEvent >= interval) {
      strongClick();
      lastEvent = now;
    }
  }
};

// =========================
// Global Instance
// =========================

HapticEngine haptic;

// =========================
// Arduino
// =========================

void setup() {
  Serial.begin(115200);

  haptic.begin();
  haptic.setPattern(PATTERN_P3_FAST_NOISE);

  Serial.println("Send 0 or 1 to switch pattern");
}

void loop() {

  if (Serial.available()) {
    int p = Serial.parseInt();

    if (p == 0) haptic.setPattern(PATTERN_P3_FAST_NOISE);
    if (p == 1) haptic.setPattern(PATTERN_P1_FAST_CLUSTER);

    Serial.print("Pattern set to ");
    Serial.println(p);
  }

  haptic.update();
}
