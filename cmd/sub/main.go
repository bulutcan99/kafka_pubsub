package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gofiber/fiber/v3"
	"log"
	"net/http"
	"sync"

	"github.com/bulutcan99/kafka_pubsub/models"

	"github.com/IBM/sarama"
)

const (
	ConsumerGroup      = "notifications-group"
	ConsumerTopic      = "notifications"
	ConsumerPort       = ":3030"
	KafkaServerAddress = "localhost:9094"
)

type UserNotifications map[int][]models.Notification

type NotificationStore struct {
	data UserNotifications
	mu   sync.RWMutex
}

func (ns *NotificationStore) Add(userID int, notification models.Notification) {
	ns.mu.Lock()
	defer ns.mu.Unlock()
	ns.data[userID] = append(ns.data[userID], notification)
}

func (ns *NotificationStore) Get(userID int) []models.Notification {
	ns.mu.RLock()
	defer ns.mu.RUnlock()
	return ns.data[userID]
}

type Consumer struct {
	store *NotificationStore
}

func (*Consumer) Setup(sarama.ConsumerGroupSession) error   { return nil }
func (*Consumer) Cleanup(sarama.ConsumerGroupSession) error { return nil }

func (consumer *Consumer) ConsumeClaim(groupSession sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for msg := range claim.Messages() {
		var notification models.Notification
		err := json.Unmarshal(msg.Value, &notification)
		if err != nil {
			log.Printf("failed to unmarshal notification: %v", err)
			continue
		}
		userId := notification.To.ID
		fmt.Println("CONSUMER: ", userId, notification)
		consumer.store.Add(userId, notification)
		fmt.Println("CONSUMER: ", consumer.store.Get(userId))
		groupSession.MarkMessage(msg, "")
	}
	return nil
}

func initializeConsumerGroup() (sarama.ConsumerGroup, error) {
	config := sarama.NewConfig()

	consumerGroup, err := sarama.NewConsumerGroup(
		[]string{KafkaServerAddress}, ConsumerGroup, config)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize consumer group: %w", err)
	}

	return consumerGroup, nil
}

func setupConsumerGroup(ctx context.Context, store *NotificationStore) {
	consumerGroup, err := initializeConsumerGroup()
	if err != nil {
		log.Printf("initialization error: %v", err)
	}
	defer consumerGroup.Close()

	consumer := &Consumer{
		store: store,
	}

	for {
		err = consumerGroup.Consume(ctx, []string{ConsumerTopic}, consumer)
		if err != nil {
			log.Printf("error from consumer: %v", err)
		}
		if ctx.Err() != nil {
			return
		}
	}
}

func handleNotifications(ctx fiber.Ctx, store *NotificationStore) error {
	userID, err := ctx.ParamsInt("user_id")
	if err != nil {
		return ctx.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "user id must be a number",
		})
	}

	notes := store.Get(userID)
	if len(notes) == 0 {
		return ctx.Status(http.StatusNoContent).JSON(fiber.Map{
			"error": true,
			"msg":   "no notifications found",
		})
	}

	return nil
}

func main() {
	store := &NotificationStore{
		data: make(UserNotifications),
	}

	ctx, cancel := context.WithCancel(context.Background())
	go setupConsumerGroup(ctx, store)
	defer cancel()

	app := fiber.New()
	app.Get("/:user_id/receive", func(ctx fiber.Ctx) error {
		err := handleNotifications(ctx, store)
		if err != nil {
			return err
		}
		return ctx.Status(http.StatusOK).JSON(fiber.Map{
			"error": false,
			"msg":   "notifications found",
			"data":  store.Get(2),
		})
	})

	fmt.Printf("Kafka Consumer group %s started at http://localhost:%s\n", ConsumerGroup, ConsumerPort)
	log.Fatal(app.Listen(":3030"))
}
