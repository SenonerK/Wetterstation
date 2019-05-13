import lcddriver
import mysql.connector
import json
from time import *

lcd = lcddriver.lcd()
lcd.lcd_clear()

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
except:
    print("Error connecting to Database!")
    exit(1)

while True:
	try:
	   db.execute("SELECT * FROM wetter ORDER BY time DESC LIMIT 1;")
	   res = db.fetchone()
	   lcd.lcd_display_string("Feuchtigkeit: {}%".format(res[2]), 1)
	   lcd.lcd_display_string("  Temperatur: {}{}C".format(res[3], chr(223)), 2)
	   lcd.lcd_display_string("    Batterie: {}V".format(res[5]), 3)
	   lcd.lcd_display_string("  Helligkeit: {}%".format(res[4]), 4)
	except mysql.connector.Error as error:
	   print("Failed get data: {}".format(error))

	sleep(10)
