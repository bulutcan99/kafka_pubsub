package main

import (
	"encoding/json"
	"fmt"
	"github.com/IBM/sarama"
	"github.com/bulutcan99/kafka_pubsub/models"
	"github.com/gofiber/fiber/v3"
	"log"
	"strconv"
)

const (
	FiberPort          = ":8080"
	KafkaServerAddress = "localhost:9094"
	KafkaTopic         = "notifications"
)

type notificationRequest struct {
	ToUserId int    `json:"user_id"`
	Message  string `json:"message"`
}

func setupProducer() (sarama.SyncProducer, error) {
	config := sarama.NewConfig()
	config.Producer.Return.Successes = true
	producer, err := sarama.NewSyncProducer([]string{KafkaServerAddress},
		config)
	if err != nil {
		return nil, fmt.Errorf("failed to setup producer: %w", err)
	}
	return producer, nil
}

func findUserByID(id int, users []models.User) (models.User, error) {
	for _, user := range users {
		if user.ID == id {
			return user, nil
		}
	}
	return models.User{}, fmt.Errorf("user with id %d not found", id)
}

func sendKafkaMessage(producer sarama.SyncProducer,
	users []models.User, ctx fiber.Ctx, fromID, toID int, message string) error {
	fromUser, err := findUserByID(fromID, users)
	if err != nil {
		return err
	}

	fmt.Println("From user found:", fromUser)
	toUser, err := findUserByID(toID, users)
	if err != nil {
		return err
	}

	fmt.Println("To user found:", toUser)
	notification := models.Notification{
		From: fromUser,
		To:   toUser, Message: message,
	}

	notificationJSON, err := json.Marshal(notification)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	msg := &sarama.ProducerMessage{
		Topic: KafkaTopic,
		Key:   sarama.StringEncoder(strconv.Itoa(toUser.ID)),
		Value: sarama.StringEncoder(notificationJSON),
	}

	_, _, err = producer.SendMessage(msg)
	return err
}

func sendNotification(c fiber.Ctx, producer sarama.SyncProducer, users []models.User) error {
	userId, err := c.ParamsInt("user_id")
	if err != nil {
		return fmt.Errorf("user id must be a number: %w", err)
	}
	fmt.Println("Sending notification by user id:", userId)
	var reqBody notificationRequest
	body := c.Body()
	if err := json.Unmarshal(body, &reqBody); err != nil {
		return fmt.Errorf("failed to unmarshal request body: %w", err)
	}

	fmt.Println("Request body:", reqBody)
	if err := sendKafkaMessage(producer, users, c, userId, reqBody.ToUserId, reqBody.Message); err != nil {
		return fmt.Errorf("failed to send kafka message: %w", err)
	}

	return nil
}

func main() {
	users := []models.User{
		{ID: 1, Name: "Bulut"},
		{ID: 2, Name: "Can"},
		{ID: 3, Name: "Bulutcan"},
		{ID: 4, Name: "Gocer"},
	}
	producer, err := setupProducer()
	if err != nil {
		log.Fatalf("failed to setup producer: %v", err)
	}
	defer producer.Close()

	app := fiber.New()

	app.Post("/:user_id/send", func(c fiber.Ctx) error {
		if err := sendNotification(c, producer, users); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": err.Error()})
		}
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "Notification sent successfully!"})
	})

	log.Fatal(app.Listen(":3000"))
}
