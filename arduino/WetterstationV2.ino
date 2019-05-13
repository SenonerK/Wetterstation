#include <SPI.h>
#include <RF24.h>
#include <EEPROM.h>
#include "DHT.h"

// Bitte ändern! wenn eine andere Version des DHT Sensors verwendet wird
DHT dht(2, DHT11);

// Radio controller
RF24 nrf(9, 10);

void setup()
{
  dht.begin();

  nrf.begin();
  // Maximale Power
  nrf.setPALevel(RF24_PA_MAX);
  nrf.setChannel(0x76);
  nrf.openWritingPipe(0xF0F0F0F0E1LL);
  const uint64_t pipe = 0xE8E8F0F0E1LL;
  nrf.openReadingPipe(1, pipe);
  nrf.enableDynamicPayloads();
  nrf.startListening();
}

void loop()
{
  // Wenn irgendetwas gesendet wird
  if (nrf.available())
  {
    char msg[32] = {0};
    nrf.read(msg, sizeof(msg));
    nrf.stopListening();
    SendStateData();
    nrf.startListening();
  }
}

void SendFloat(char *key, float v)
{
  char text[32] = {0};
  char tmp[6] = {0};
  // Mindestens eine insgesamte länge von 3 zeichen und zwei Kommastellen
  dtostrf(v, 3, 2, tmp);
  sprintf(text, "%s: %s", key, tmp);
  nrf.write(&text, sizeof(text));
}

void SendInt(char *key, int v)
{
  char text[32] = {0};
  sprintf(text, "%s: %d", key, v);
  nrf.write(&text, sizeof(text));
}

void SendStateData()
{
  float humidity = dht.readHumidity();
  float temperatur = dht.readTemperature();
  int brightness = map(analogRead(A0), 0, 1023, 0, 100);
  // den eingangswert in Volt convertieren
  float battery = (float)analogRead(A1) * (5.00 / 1023.00) * 2;

  if (isnan(humidity) || isnan(temperatur))
  {
    char text[32] = {0};
    sprintf(text, "ERROR");
    nrf.write(&text, sizeof(text));
    return;
  }

  SendFloat("humid", humidity);
  SendFloat("temp", temperatur);
  SendInt("bright", brightness);
  SendFloat("battery", battery);
}
