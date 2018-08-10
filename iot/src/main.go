package main

import (
	"bytes"
	"database/sql"
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"time"

	"github.com/armon/go-metrics"
	"github.com/armon/go-metrics/prometheus"
	"github.com/go-redis/redis"
	_ "github.com/go-sql-driver/mysql"
	"github.com/gorilla/mux"
	_ "github.com/mattn/go-sqlite3"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	pubnub "github.com/pubnub/go"
	"github.com/streadway/amqp"
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
var AppDb = os.Getenv("APP_DB_SERVER")

// AppRmq name
var AppRmq = os.Getenv("APP_RMQ_SERVER")

// RmqCH name
var RmqCH = os.Getenv("APP_RMQ_CHANNEL")

// RmqID credentials
var RmqID = os.Getenv("RABBITMQ_DEFAULT_USER") + ":" + os.Getenv("RABBITMQ_DEFAULT_PASS")

// AppRedis name
var AppRedis = os.Getenv("APP_REDIS_SERVER") + ":" + os.Getenv("APP_REDIS_PORT")

// PublishKey key
var PublishKey = os.Getenv("APP_PN_PUBKEY")

// SubscribeKey key
var SubscribeKey = os.Getenv("APP_PN_SUBKEY")

// PnChannel name
var PnChannel = os.Getenv("APP_PN_CHANNEL")

// PnUrl
var PnUrl = "https://ps.pndsn.com/publish/"

type apiStruct struct {
	APIVersion string `json:"apiVersion"`
}

// PnMessage ...
type PnMessage struct {
	Iot []byte `json:"iot"`
}

func main() {

	log.Print(Revision)
	sink, _ := prometheus.NewPrometheusSink()
	metrics.NewGlobal(metrics.DefaultConfig("API"), sink)

	config := pubnub.NewConfig()
	config.PublishKey = PublishKey
	config.SubscribeKey = SubscribeKey

	//pn := pubnub.NewPubNub(config)

	conn, err := amqp.Dial("amqp://" + RmqID + "@" + AppRmq + ":5672/")
	failOnError(err, "Failed to connect to RabbitMQ")
	defer conn.Close()

	ch, err := conn.Channel()
	failOnError(err, "Failed to open a channel")
	defer ch.Close()

	q, err := ch.QueueDeclare(
		RmqCH, // name
		false, // durable
		false, // delete when usused
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)

	failOnError(err, "Failed to declare a queue")
	msgs, err := ch.Consume(
		q.Name, // queue
		"",     // consumer
		true,   // auto-ack
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)

	failOnError(err, "Failed to register a consumer")

	go func() {
		var PnMessages PnMessage
		buf := new(bytes.Buffer)

		for d := range msgs {
			log.Printf("Received a message: %s", d.Body)
			PnMessages.Iot, _ = b64.URLEncoding.DecodeString(string(d.Body))
			fmt.Println(PnMessages.Iot)
			//if len(m.APIVersion) > 4 {
			enc := json.NewEncoder(buf)
			enc.Encode(PnMessages)

			log.Printf("Convert a message: %s", PnMessages.Iot)
			var a string
			req, err := http.NewRequest("GET", "https://ps.pndsn.com/publish/pub-c-356be33a-08e5-4678-a737-f3f0527a0a64/sub-c-5e07b1f2-9ba2-11e8-9a92-b209910313bb/0/channel-mapbox/myCallback/%7B%22text%22%3A%22hey%22%7D?store=0&uuid=db9c5e39-7c95-40f5-8d71-125765b6f561", bytes.NewBuffer([]byte(a)))
			log.Print(err)
			client := &http.Client{Timeout: time.Second * 5}
			resp, _ := client.Do(req)
			log.Print(resp)
			log.Println(rest(PnUrl+PublishKey+"/"+SubscribeKey+"/0/"+PnChannel+"/0", string(PnMessages.Iot)))
			/** message := struct {
							Latlng [2]float64 `json:"latlng"`
						}{[2]float64{50.456114, 30.567914}}
			**/
			//a := `{"latlng":[50.4581,30.487437999999997]}`
			/**
						res, status, err := pn.Publish().
							Channel(PnChannel).
							Message(map[string]interface{}{
								"latlng": "[50.4581,30.487437999999997]",
							}).
							UsePost(true).
							Execute()

			**/
			// handle publish result
			//fmt.Println(res, status, err)
			buf.Reset()
		}
	}()

	//log.Printf(" [*] Waiting for messages. To exit press CTRL+C")

	router := mux.NewRouter().StrictSlash(true)
	router.HandleFunc("/version", versionHandler)
	router.HandleFunc("/healthz", healthzHandler)
	router.HandleFunc("/readinez", readinessHandler)
	router.Handle("/metrics", promhttp.Handler())

	//	router.HandleFunc("/", appHandler)

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
		http.Error(w, "Not Ready", http.StatusServiceUnavailable)
	}

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
		log.Print(err)
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

func appHandler(w http.ResponseWriter, r *http.Request) {
	metrics.IncrCounter([]string{"requestCounter"}, 1)
	var m apiStruct
	switch r.Method {
	case "GET":
		log.Printf("Get GET Request!")
		u, err := url.Parse(r.RequestURI)
		if err != nil {
			log.Fatal(err)
		}
		Q := u.Query()

		r, err := regexp.Compile(`^\*(?P<USSD_CODE>\d{3})[\*|\#](?P<USSD_DEST>\D{0,}\d{0,}).?(?P<USSD_EXT>.{0,}).?`)
		if err != nil {
			log.Print(err)
		}
		r2 := r.FindAllStringSubmatch(Q.Get("calldestination"), -1)
		fmt.Println(r2[0][1])

		w.Write([]byte("Please use POST"))

	case "POST":
		b, _ := ioutil.ReadAll(io.LimitReader(r.Body, 1048576))
		if err := json.Unmarshal(b, &m); err != nil {
			w.Header().Set("Content-Type", "application/json; charset=UTF-8")
			w.WriteHeader(422) // unprocessable entity
			if err := json.NewEncoder(w).Encode(err); err != nil {
				log.Print(err, len(m.APIVersion))
			}
		}
		if len(m.APIVersion) > 4 {
			enc := json.NewEncoder(w)
			enc.Encode(API(m.APIVersion))
		} else {
			w.Write([]byte("Incorrect Api Version"))
		}
		//w.Write([]byte(getRate(m.Msisdn)))
	}
}

// API func
func API(verson string) apiStruct {

	defer metrics.MeasureSince([]string{"API"}, time.Now())

	var apiResult apiStruct

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
		Addr:     AppRedis, //Addr:     "redis:6379",
		Password: "",       // no password set
		DB:       0,        // use default DB
	})
	client.Ping()
	return apiResult
}
func failOnError(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %s", msg, err)
		panic(fmt.Sprintf("%s: %s", msg, err))
	}
}
