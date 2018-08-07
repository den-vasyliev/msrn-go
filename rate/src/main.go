package main

import (
	"bytes"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/armon/go-metrics"
	"github.com/armon/go-metrics/prometheus"
	"github.com/fatih/structs"
	"github.com/go-redis/redis"
	_ "github.com/go-sql-driver/mysql"
	"github.com/gorilla/mux"
	_ "github.com/mattn/go-sqlite3"
	"github.com/mitchellh/mapstructure"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// AppName app
//var AppName = "name"
var AppName = os.Getenv("APP_NAME")

// Version app
var Version = "version"

// BuildInfo app
var BuildInfo = "commit"

// Revision app
var Revision = fmt.Sprintf("%s version: %s+%s", AppName, Version, BuildInfo)

// AppPort app
var AppPort = os.Getenv("APP_PORT")

// AppDb Server
var AppDb = os.Getenv("APP_DB_SERVER")

// AppRedis Server
var AppRedis = os.Getenv("APP_REDIS_SERVER") + ":" + os.Getenv("APP_REDIS_PORT")

type msisdn struct {
	Msisdn string `json:"msisdn"`
}

func main() {
	log.Print(Revision)
	sink, _ := prometheus.NewPrometheusSink()
	metrics.NewGlobal(metrics.DefaultConfig("getRate"), sink)

	router := mux.NewRouter().StrictSlash(true)
	router.HandleFunc("/version", versionHandler)
	router.HandleFunc("/healthz", healthzHandler)
	router.HandleFunc("/readinez", readinessHandler)
	router.Handle("/metrics", promhttp.Handler())
	router.HandleFunc("/", rateHandler)

	log.Fatal(http.ListenAndServe(":"+AppPort, router))
}

func versionHandler(w http.ResponseWriter, r *http.Request) {
	var b []byte
	b = append([]byte(""), Revision...)
	w.Write(b)
}

func healthzHandler(w http.ResponseWriter, r *http.Request) {

	w.Write([]byte("Healthz: alive!"))
}

func readinessHandler(w http.ResponseWriter, r *http.Request) {

	client := redis.NewClient(&redis.Options{
		Addr:     AppRedis,
		Password: "", // no password set
		DB:       0,  // use default DB
	})
	probe, err := client.Set("readiness_probe", 0, 0).Result()
	log.Print(probe)
	if err != nil {
		http.Error(w, "Cache Not Ready", http.StatusServiceUnavailable)
	}

	db, err := sql.Open("sqlite3", AppDb)
	if err != nil {
		http.Error(w, "DB Not Ready", http.StatusServiceUnavailable)
	}
	defer db.Close()

	err = db.Ping()
	if err != nil {
		http.Error(w, "DB Not Reachable", http.StatusServiceUnavailable)
	}

	//w.Write([]byte("200"))

}

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	var b []byte
	b = append([]byte(""), Revision...)
	w.Write(b)
}

func rest(url string, jsonStr string) []byte {
	req, err := http.NewRequest("POST", url, bytes.NewBuffer([]byte(jsonStr)))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: time.Second * 5}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	log.Print("response Status:", resp.Status)
	log.Print("response Headers:", resp.Header)
	body, _ := ioutil.ReadAll(resp.Body)
	return body
}

func readiness(url string) string {
	c := &http.Client{
		Timeout: 2 * time.Second,
	}
	resp, err := c.Get(url)
	if err != nil {
		log.Print(err)
		return resp.Status
	}
	defer resp.Body.Close()

	log.Print("response Status:", resp.Status)
	return resp.Status
}

func rateHandler(w http.ResponseWriter, r *http.Request) {
	metrics.IncrCounter([]string{"requestCounter"}, 1)
	var m msisdn
	switch r.Method {
	case "GET":
		log.Printf("Get GET Request!")
		w.Write([]byte("Please use POST"))

	case "POST":
		b, _ := ioutil.ReadAll(io.LimitReader(r.Body, 1048576))
		if err := json.Unmarshal(b, &m); err != nil {
			w.Header().Set("Content-Type", "application/json; charset=UTF-8")
			w.WriteHeader(422) // unprocessable entity
			if err := json.NewEncoder(w).Encode(err); err != nil {
				log.Print(err, len(m.Msisdn))
			}
		}
		if len(m.Msisdn) > 4 {
			enc := json.NewEncoder(w)
			enc.Encode(getRate(m.Msisdn))
		} else {
			w.Write([]byte("Incorrect MSISDN"))
		}
		//w.Write([]byte(getRate(m.Msisdn)))
	}
}

type rateStruct struct {
	Rate   string
	Trunk  string
	Prefix string
	RateID string
	Dest   string
}

func getRate(msisdn string) rateStruct {
	defer metrics.MeasureSince([]string{"getRate"}, time.Now())
	var rateResult rateStruct
	db, err := sql.Open("sqlite3", AppDb)
	if err != nil {
		log.Print("Open db err: ")
		panic(err)
	}
	defer db.Close()
	err = db.Ping()
	if err != nil {
		log.Print("Ping db err: ")
		panic(err.Error()) // proper error handling instead of panic in your app
	}

	client := redis.NewClient(&redis.Options{
		Addr:     AppRedis, //Addr:"redis:6379",
		Password: "",       // no password set
		DB:       0,        // use default DB
	})

	rateResultMap, err := client.HGetAll("RATE_CACHE:" + msisdn[0:5]).Result()
	mapstructure.Decode(rateResultMap, &rateResult)
	log.Print(rateResult)
	defer metrics.MeasureSince([]string{"getRateRedis"}, time.Now())
	if err != nil || rateResult.RateID == "" {
		log.Print(err, rateResult.RateID)
		sqlGetRate, _ := hex.DecodeString(client.HGet("TEMPLATE", "sql:getrate").Val())
		err = db.QueryRow(string(sqlGetRate), msisdn).Scan(&rateResult.Rate, &rateResult.Trunk, &rateResult.Prefix, &rateResult.RateID, &rateResult.Dest)
		defer metrics.MeasureSince([]string{"getRateDB"}, time.Now())
		if err != nil {
			log.Print(err.Error()) // proper error handling instead of panic in your app
		} else if rateResult.Rate != "" {
			client.HMSet("RATE_CACHE:"+msisdn[0:5], structs.Map(rateResult))
			client.Expire("RATE_CACHE:"+msisdn[0:5], 10*time.Second)
		}
	}
	return rateResult
}
