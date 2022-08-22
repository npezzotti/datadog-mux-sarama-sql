package main

import (
	"context"
	"log"

	"github.com/Shopify/sarama"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/ext"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"

	saramatrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/Shopify/sarama"
	sqltrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/database/sql"

	"github.com/lib/pq"
)

func main() {
	tracer.Start()
	defer tracer.Stop()

	sqltrace.Register("postgres", &pq.Driver{})

	db, err := sqltrace.Open("postgres", "postgres://datadog:datadog@db/mux-sarama-app-db?sslmode=disable")
	if err != nil {
		log.Fatal(err)
	} else {
		log.Printf("Connected to DB")
	}

	consumer, err := sarama.NewConsumer([]string{"broker:29092"}, nil)
	if err != nil {
		panic(err)
	}

	consumer = saramatrace.WrapConsumer(consumer)

	defer consumer.Close()

	partitionConsumer, err := consumer.ConsumePartition("test", 0, sarama.OffsetNewest)
	if err != nil {
		panic(err)
	}

	defer partitionConsumer.Close()

	for msg := range partitionConsumer.Messages() {
		log.Printf("Consumed message \"%s\" from topic \"%s\" at offset %d\n", msg.Value, msg.Topic, msg.Offset)

		spanctx, err := tracer.Extract(saramatrace.NewConsumerMessageCarrier(msg))
		if err != nil {
			log.Fatal(err)
		}

		span, ctx := tracer.StartSpanFromContext(context.Background(), "db-init",
			tracer.SpanType(ext.SpanTypeSQL),
			tracer.ServiceName("postgres.db"),
			tracer.ResourceName("initial-access"),
			tracer.ChildOf(spanctx),
		)
		result, err := db.ExecContext(ctx, `INSERT INTO messages(message) VALUES($1)`, msg.Value)
		if err != nil {
			log.Fatal(err)
		}

		rows, err := result.RowsAffected()
		if err != nil {
			log.Fatal(err)
		}
		if rows != 1 {
			log.Fatalf("Affected %d", rows)
		}
		span.Finish(tracer.WithError(err))
	}
}
