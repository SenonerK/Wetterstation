package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/oxtoacart/bpool"
)

// Wetter holds weather data
type Wetter struct {
	Time        time.Time
	Humidity    int
	Temperature float64
	Brightness  int
	Battery     float64
}

type StatsPage struct {
	Filter string
	Data   []Wetter
}

func (d StatsPage) CalcJSObject() string {
	labels := ""
	temps := ""
	humids := ""
	brighs := ""
	batteries := ""

	for _, x := range d.Data {
		labels += fmt.Sprintf("\"%s\",", x.Time.Format("_2 Jan 15:04"))
		temps += fmt.Sprintf("%f,", x.Temperature)
		humids += fmt.Sprintf("%d,", x.Humidity)
		brighs += fmt.Sprintf("%d,", x.Brightness)
		batteries += fmt.Sprintf("%f,", x.Battery)
	}

	labels = labels[:len(labels)-1]
	temps = temps[:len(temps)-1]
	humids = humids[:len(humids)-1]
	brighs = brighs[:len(brighs)-1]
	batteries = batteries[:len(batteries)-1]

	return fmt.Sprintf(`{"labels": [%s], "datasets": [
		{"label": "Temperatur", "borderColor": "red", "fill": false, "data": [%s]},
		{"label": "Feuchtigkeit", "borderColor": "green", "fill": false, "hidden": true, "data": [%s]},
		{"label": "Helligkeit", "borderColor": "blue", "fill": false, "hidden": true, "data": [%s]},
		{"label": "Batterie", "borderColor": "black", "fill": false, "hidden": true, "data": [%s]}
		]}`, labels, temps, humids, brighs, batteries)
}

func (d Wetter) TimeOffset() time.Duration {
	_, timezone := time.Now().Zone()
	return time.Since(d.Time.Add(-time.Duration(timezone) * time.Second))
}

func (d Wetter) WeatherIcon() string {
	res := "cloudy"

	if (d.Time.Hour() > 17 || d.Time.Hour() < 6) && d.Brightness < 20 {
		res = "night"
	} else if d.Brightness > 88 {
		res = "day"
	}

	if d.Humidity >= 95 {
		if d.Temperature < 0 {
			res = "snowy-5"
		} else {
			res = "rainy-5"
		}
	}

	if d.Brightness > 85 && d.Humidity >= 92 {
		if d.Temperature < 0 {
			res = "snowy-3"
		} else {
			res = "rainy-3"
		}
	}

	return res + ".svg"
}

const CONFIG_PATH = "/etc/wetter/config.json"

var templates map[string]*template.Template
var bufpool *bpool.BufferPool

var config map[string]interface{}

var db *sql.DB

func loadConfig() {
	jsn, err := os.Open(CONFIG_PATH)
	if err != nil {
		panic(err.Error())
	}
	defer jsn.Close()

	bytes, err := ioutil.ReadAll(jsn)
	if err != nil {
		panic(err.Error())
	}

	err = json.Unmarshal(bytes, &config)
	if err != nil {
		panic(err.Error())
	}

	dbconfig, ok := config["db"].(map[string]interface{})
	if !ok {
		panic("From config format")
	}

	db, err = sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s:3306)/%s", dbconfig["user"], dbconfig["password"], dbconfig["host"], dbconfig["database"]))
	if err != nil {
		panic(err.Error())
	}
}

func writeConfig() error {
	bytes, err := json.Marshal(config)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(CONFIG_PATH, bytes, 664)
	if err != nil {
		return err
	}

	return exec.Command("sudo", "systemctl", "restart", "wetter-client.service").Run()
}

func loadTemplates() {
	if templates == nil {
		templates = make(map[string]*template.Template)
	}

	layoutFiles, err := filepath.Glob("templates/layout/*.html")
	if err != nil {
		log.Fatal(err)
	}

	includeFiles, err := filepath.Glob("templates/*.html")
	if err != nil {
		log.Fatal(err)
	}

	mainTemplate := template.New("main")
	mainTemplate, err = mainTemplate.Parse(`{{define "main" }} {{ template "base" . }} {{ end }}`)
	if err != nil {
		log.Fatal(err)
	}

	for _, file := range includeFiles {
		fileName := filepath.Base(file)
		files := append(layoutFiles, file)
		templates[fileName], err = mainTemplate.Clone()
		if err != nil {
			log.Fatal(err)
		}
		templates[fileName] = template.Must(templates[fileName].ParseFiles(files...))
	}

	log.Println("Templates loaded")

	bufpool = bpool.NewBufferPool(64)
}

func renderTemplate(w http.ResponseWriter, name string, data interface{}) {
	tmpl, ok := templates[name]
	if !ok {
		http.Error(w, fmt.Sprintf("The template %s does not exist.", name),
			http.StatusNotFound)
	}

	buf := bufpool.Get()
	defer bufpool.Put(buf)

	err := tmpl.Execute(buf, data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	buf.WriteTo(w)
}

func index(w http.ResponseWriter, r *http.Request) {
	var data Wetter
	var tm mysql.NullTime
	err := db.QueryRow("SELECT time,humidity,temperature,brightness,battery FROM wetter ORDER BY time DESC LIMIT 1").Scan(&tm, &data.Humidity, &data.Temperature, &data.Brightness, &data.Battery)

	if err != nil || !tm.Valid {
		if err == sql.ErrNoRows {
			renderTemplate(w, "index.html", nil)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data.Time = tm.Time

	renderTemplate(w, "index.html", data)
}

func chart(w http.ResponseWriter, r *http.Request) {
	page := StatsPage{
		Filter: r.URL.Query().Get("timespan"),
	}

	if page.Filter == "" {
		page.Filter = "day"
	}

	var fromTime time.Time
	switch page.Filter {
	case "week":
		fromTime = time.Now().AddDate(0, 0, -7)
	case "all":
		fromTime = time.Unix(0, 0)
	case "day":
		fromTime = time.Now().AddDate(0, 0, -1)
	}

	result, err := db.Query("SELECT time,humidity,temperature,brightness,battery FROM wetter WHERE time BETWEEN ? AND now() ORDER BY time", fromTime.Format("2006-01-02 15:04:05"))

	if err != nil {
		if err == sql.ErrNoRows {
			renderTemplate(w, "chart.html", nil)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for result.Next() {
		var t Wetter
		var tm mysql.NullTime
		err = result.Scan(&tm, &t.Humidity, &t.Temperature, &t.Brightness, &t.Battery)
		if err != nil || !tm.Valid {
			continue
		}
		t.Time = tm.Time
		page.Data = append(page.Data, t)
	}

	renderTemplate(w, "chart.html", page)
}

func settings(w http.ResponseWriter, r *http.Request) {
	renderTemplate(w, "settings.html", nil)
}

func intervall(w http.ResponseWriter, r *http.Request) {

	err := r.ParseForm()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	inter, _ := strconv.Atoi(r.Form.Get("inter"))
	config["intervalSec"] = inter

	if err := writeConfig(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", 300)
}

func database(w http.ResponseWriter, r *http.Request) {

	err := r.ParseForm()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ddata := make(map[string]string)
	ddata["host"] = r.Form.Get("host")
	ddata["password"] = r.Form.Get("password")
	ddata["user"] = r.Form.Get("user")
	ddata["database"] = r.Form.Get("database")

	config["db"] = ddata

	if err := writeConfig(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", 300)
}

func main() {
	// Konfiguration laden und DB verbindung aufbauen
	loadConfig()
	// DB verbindug schließen wenn diese function zuende ist
	defer db.Close()

	// Websiten templates ins RAM laden
	loadTemplates()

	// Asset dateien bilder/css
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./assets/"))))
	http.HandleFunc("/", index)
	http.HandleFunc("/chart", chart)
	http.HandleFunc("/settings", settings)
	http.HandleFunc("/settings/intervall", intervall)
	http.HandleFunc("/settings/db", database)

	log.Println("Starting webserver")
	log.Fatal(http.ListenAndServe(":80", nil))
}
