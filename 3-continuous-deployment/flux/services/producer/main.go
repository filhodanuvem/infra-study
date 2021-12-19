package main

import (
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/filhodanuvem/producer/metric"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	uuid "github.com/satori/go.uuid"
)

const tracerName = "producer"
const eventType = "PAYMENT_ORDER_CREATED"

var nats_url = os.Getenv("NATS_URL")
var nats_subject = os.Getenv("NATS_SUBJECT")

var bmetric = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name: "payment_order_time_in_seconds",
	Help: "Duration time of an order creation",
}, metric.Labels)

type message struct {
	Amount  int               `json:"amount"`
	Headers map[string]string `json:"headers"`
}

func main() {
	prometheus.Register(bmetric)
	sc, err := nats.Connect(nats_url)
	if err != nil {
		log.Fatalf("Couldn't connect to nats %s, err: %s", nats_url, err)
	}
	defer sc.Close()

	go func() {
		http.Handle("/metrics", promhttp.Handler())
		http.ListenAndServe(":2222", nil)
	}()

	js, err := sc.JetStream()
	if err != nil {
		log.Fatalf("Couldn't connect to nats %s, err: %s", nats_url, err)
	}

	log.Println("Running crazy producer...")

	numbers := make(chan int)
	go func(js nats.JetStreamContext) {
		for {
			u1 := uuid.NewV4()
			amount := <-numbers
			m := message{
				Amount: amount,
				Headers: map[string]string{
					"x-trace-id": u1.String(),
				},
			}

			labels := metric.NewLabels(
				strconv.Itoa(m.Amount),
				m.Headers["x-trace-id"],
				eventType,
			)
			recorder := metric.NewRecorder().WithTimer(bmetric, labels)

			b, err := json.Marshal(m)
			if err != nil {
				log.Printf("Error on publishing to nats: %s\n", err)
				continue
			}

			if _, err := js.Publish(nats_subject, b); err != nil {
				log.Printf("Error on publishing to nats: %s\n", err)
				continue
			}

			recorder.RecordDuration()

			log.Println(m)
		}
	}(js)

	for {
		number := rand.Intn(100)
		numbers <- number
		time.Sleep(3 * time.Second)
	}
}
