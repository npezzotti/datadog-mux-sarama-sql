package main

import (
	"log"
	"net/http"
	"time"

	"github.com/Shopify/sarama"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"

	saramatrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/Shopify/sarama"
	muxtrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/gorilla/mux"
)

func MessageHandler(w http.ResponseWriter, r *http.Request) {
	cfg := sarama.NewConfig()
	cfg.Producer.Return.Successes = true

	producer, err := sarama.NewSyncProducer([]string{"broker:29092"}, cfg)
	if err != nil {
		log.Fatalln(err)
	}

	producer = saramatrace.WrapSyncProducer(cfg, producer)

	defer func() {
		if err := producer.Close(); err != nil {
			log.Fatalln(err)
		}
	}()
	
	msg := &sarama.ProducerMessage{Topic: "test", Value: sarama.StringEncoder("Hello World!")}
	
	ctx := r.Context()
	if span, ok := tracer.SpanFromContext(ctx); ok {
		carrier := saramatrace.NewProducerMessageCarrier(msg)
		tracer.Inject(span.Context(), carrier)
	}

	partition, offset, err := producer.SendMessage(msg)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Printf("Failed to send message: %s\n", err)
	} else {
		w.WriteHeader(http.StatusOK)
		log.Printf("Message \"%s\" sent to partition %d at offset %d\n", partition, offset)
	}
}

func main() {
	tracer.Start()
	defer tracer.Stop()

	r := muxtrace.NewRouter()
	r.HandleFunc("/message", MessageHandler).Methods("GET")

	srv := &http.Server{
		Handler:      r,
		Addr:         "0.0.0.0:8000",
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	log.Fatal(srv.ListenAndServe())
}
