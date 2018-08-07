package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/armon/go-metrics"
	"github.com/armon/go-metrics/prometheus"

	"github.com/go-redis/redis"
	_ "github.com/go-sql-driver/mysql"
	"github.com/gorilla/mux"
	_ "github.com/mattn/go-sqlite3"

	"github.com/dgrijalva/jwt-go"
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

// AppDb name
var AppDb = os.Getenv("APP_DB_NAME")

// AppRedis name
var AppRedis = os.Getenv("APP_REDIS_NAME")

type authSession struct {
	Session     string `json:"session"`
	Token       string `json:"token"`
	Address     string `json:"address"`
	Pin         string `json:"pin"`
	RequestType string `json:"requestType"`
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

	router.HandleFunc("/", authHandler)

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
		Addr:     "redis:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})
	probe, err := client.Set("readiness_probe", 0, 0).Result()
	log.Print(probe)
	if err != nil {
		http.Error(w, "Not Ready", http.StatusServiceUnavailable)
	}

	db, err := sql.Open("mysql", AppDb)
	if err != nil {
		http.Error(w, "Not Ready", http.StatusServiceUnavailable)
	}
	defer db.Close()
	err = db.Ping()

	if err != nil {
		http.Error(w, "Not Ready", http.StatusServiceUnavailable)
	}

	w.Write([]byte("200"))

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

func authHandler(w http.ResponseWriter, r *http.Request) {
	metrics.IncrCounter([]string{"requestCounter"}, 1)
	var m authSession
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
				log.Print(err, len(m.Token))
			}
		}
		if len(m.Address) > 0 {
			enc := json.NewEncoder(w)
			enc.Encode(auth(m))
		} else {
			w.Write([]byte("Incorrect Address"))
		}
		//w.Write([]byte(getRate(m.Msisdn)))
	}
}

func auth(p authSession) int {
	//func auth(session string, address string, token string, requestType string, pin string) int {
	defer metrics.MeasureSince([]string{"auth"}, time.Now())
	//var auth authSession

	client := redis.NewClient(&redis.Options{
		Addr:     AppRedis, //Addr:     "redis:6379",
		Password: "",       // no password set
		DB:       0,        // use default DB
	})

	// [SIG ONLY] ****************************************************************************
	authResult := client.SIsMember("ACCESS_LIST", p.Address).Val()
	if !authResult {
		log.Print("Not in ACCESS_LIST")
	} else {
		log.Print("SIG ACCESS")
		return 1
	}

	authResult, err := client.Expire("SESSION:"+p.Session+":"+p.Address, 10*time.Minute).Result()
	if err != nil {
		log.Print("Not in SESSION", err)
	} else if authResult {
		return 2
	}
	if p.Token == "" {
		return -1
	}

	digest := getTokenHandler(p.Address, p.Token, rand.Intn(100))
	log.Print(p.Address, p.Token, p.Session, digest)

	authResult, err = client.SIsMember("ACCESS_LIST", digest).Result()
	if err != nil {

	} else if p.RequestType == "API" && authResult {
		return 1
	}

	if p.Pin != "" {
		//	tokenPin := token + pin
	} else {
		//	tokenPin := token
	}

	if client.Set("SESSION:"+digest+":"+p.Address, p.Token, 10*time.Minute).Val() == "1" {

	}
	if err != nil {
		return -1
	} else {
		p.Session = digest
	}
	return 1

}

func getTokenHandler(address string, htmlToken string, rand int) string {
	var hmacSampleSecret = []byte("secret")
	// Create a new token object, specifying signing method and the claims
	// you would like it to contain.

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"foo": address,
		"nbf": time.Date(2015, 10, 10, 12, 0, 0, 0, time.UTC).Unix(),
	})

	// Sign and get the complete encoded token as a string using the secret
	tokenString, err := token.SignedString(hmacSampleSecret)
	if err != nil {
		panic(err)
	}
	return tokenString

}
