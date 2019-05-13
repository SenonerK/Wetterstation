import RPi.GPIO as GPIO
from lib_nrf24 import NRF24
import time
import spidev
import mysql.connector
import re
from datetime import datetime
import json

# Konfiguration laden
with open("/etc/wetter/config.json", 'r') as cfgfile:
    try:
	    config = json.load(cfgfile)
    except Exception as e:
	    print("Error loading configuration")
	    exit(1)

try:
    dbconn = mysql.connector.connect(
      host=config['db']['host'],
      user=config['db']['user'],
      passwd=config['db']['password'],
      database=config['db']['database']
    )
    db = dbconn.cursor()

    # DB schema erstellen
    db.execute("CREATE TABLE IF NOT EXISTS wetter (id INT AUTO_INCREMENT PRIMARY KEY, time DATETIME, humidity INT, temperature DOUBLE, brightness INT, battery DOUBLE)")
except:
    print("Error connecting to Database!")
    exit(1)

GPIO.setmode(GPIO.BCM)

# Sende und Empfangs Adressen sind vordefiniert
pipes = [[0xE8, 0xE8, 0xF0, 0xF0, 0xE1], [0xF0, 0xF0, 0xF0, 0xF0, 0xE1]]

radio = NRF24(GPIO, spidev.SpiDev())
radio.begin(0, 17)

radio.setPayloadSize(32)
radio.setChannel(0x76)
radio.setDataRate(NRF24.BR_1MBPS)
radio.setPALevel(NRF24.PA_MIN)

radio.setAutoAck(True)
radio.enableDynamicPayloads()
radio.enableAckPayload()

radio.openWritingPipe(pipes[0])
radio.openReadingPipe(1, pipes[1])
radio.printDetails()

while True:
    radio.write('\0')
    print("Sent request")
    radio.startListening()

    success = False
    humidity = 0.0
    temperature = 0.0
    brightness = 0
    battery = 0.0

    # 4 variablen werden übermittlet
    for i in range(4):
        # Wenn für 500ms keine antwort gesendet wird
        start = time.time()
        while not radio.available(0):
            time.sleep(1/100)
            if time.time() - start > 0.5:
                print("Timed out.")
                break

        msg = []
        # Narchicht einlesen
        radio.read(msg, radio.getDynamicPayloadSize())

        # CharArray in string umwandeln
        result = ""
        for c in msg:
            if c >= 32 and c <= 126:
            result+=chr(c)

        if result == "ERROR":
            print("Error reading sensors")
            break
        
        print("Received: {}".format(result))

        # Key-Value extrahieren
        sfield = re.search('(.*): .*', result)
        sdata = re.search('.*: (.*)', result)

        if sfield and sdata:
            field = sfield.group(1)
            data = sdata.group(1)

            # Basierend auf dem Key. Value in eine Variable einlesen

            success = True
            if field == "humid":
                humidity = float(data)
            elif field == "temp":
                temperature = float(data)
            elif field == "bright":
                brightness = int(data)
            elif field == "battery":
                battery = float(data)
            else:
                success = False

        if success:
            try:
                db.execute("INSERT INTO wetter (time, humidity, temperature, brightness, battery) VALUES (now(), %s, %s, %s, %s)",
                            (humidity, temperature, brightness, battery))
                dbconn.commit()
            except mysql.connector.Error as error:
                connection.rollback()
                print("Failed to insert: {}".format(error))
            finally:
                radio.stopListening()
                time.sleep(config['intervalSec']-10)
        else:
            radio.stopListening()
            time.sleep(10)
