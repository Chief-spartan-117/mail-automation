package main

import (
	"fmt"
	"html/template"
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/joho/godotenv"
	"github.com/wneessen/go-mail"
)

type EmailStruct struct {
	Email   string `json:"email" form:"email"`
	Name    string `json:"name" form:"name"`
	Message string
}

func main() {
	app := fiber.New()

	err := godotenv.Load()
	if err != nil {
		fmt.Println("Error loading .env file")
	}
	app.Static("/", "./src/template")
	api := app.Group("/api")

	api.Post("/email", PostEmail)

	app.Listen(":3000")

}
func PostEmail(c *fiber.Ctx) error {
	email := new(EmailStruct)

	gmailHost := os.Getenv("GMAIL_HOST")
	gmailUsername := os.Getenv("GMAIL_USERNAME")
	gmailPassword := os.Getenv("GMAIL_PASSWORD")

	if gmailHost == "" || gmailUsername == "" || gmailPassword == "" {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Environment variables not set",
		})
	}

	if err := c.BodyParser(email); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	message := mail.NewMsg()
	if err := message.FromFormat("Software Club", gmailUsername); err != nil {
		log.Fatalf("failed to set From address: %s", err)
	}

	if err := message.To(email.Email); err != nil {
		log.Fatalf("failed to set To adress: %s", err)
	}

	message.Subject("Welcome to Software club")
	t, _ := template.ParseFiles("./src/template/email.html")
	message.AddAlternativeHTMLTemplate(t, email)

	client, err := mail.NewClient(gmailHost, mail.WithSMTPAuth(mail.SMTPAuthPlain), mail.WithUsername(gmailUsername), mail.WithPassword(gmailPassword))

	if err != nil {
		log.Fatalf("failed to create mail client: %s", err)
	}
	if err := client.DialAndSend(message); err != nil {
		log.Fatalf("failed to send mail: %s", err)
	}

	return c.JSON(fiber.Map{
		"message": "Email Sent Successfully",
		"status":  "success",
	})
}
