# Wetterstation

## Raspberry Pi
- Eine MySQL Datenbank
- Einstellungen in `/etc/wetter/config.json` speichern
### Beispiel
```json
{
   "db":{
      "database":"wetter",
      "host":"localhost",
      "password":"Kennwort0",
      "user":"wetter"
   },
   "intervalSec":300
}
```
- Ordner server: Der Webserver ist in Golang geschrieben. Kompilieren mit: `GOARCH=arm GOARM=5 GOOS=linux go build main.go`


- Ordner client: Der Client ist ein Python script. Es versucht in festgelegten intervalls Daten vom Arduino anzufrage und in die datenbank zu speichern

- Ordner lcd: Ein Python script der jede 10 Sekunden die Datenbank abfragt und informationen auf ein LCD Display anzeigt

- Scripts und binaries in systemd registrieren (`/lib/systemd/system/wetter-client.service`)
### Beispiel
```ini
[Unit]
Description=Client for fetching weather data
After=multi-user.target

[Service]
Type=simple
ExecStart=/usr/bin/python /home/pi/projekt/client/main.py
Restart=always

[Install]
WantedBy=multi-user.target
```

## Arduino
Der Arduino code wartet auf Anfragen vom Raspberry Pi, liest Wetterdaten ein und sendet sie dann zurück

## Milestones
- Unterstützung von mehreren Wetterstation (da der NRF24 chip 6 pipes hat)
- Bessere LCD UI mit kontroll-buttons
- Den code für client in golang umschreiben (dazu müss man auch die NRF24 lib schreiben)
- Den Übertragungsprotokoll zwischen arduino und rpi verbessern
- Security -> login, auf SQL und Command -injection testen
