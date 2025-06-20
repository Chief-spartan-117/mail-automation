package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/wneessen/go-mail"
)

type EmailStruct struct {
	Email   string `json:"email" form:"email"`
	Name    string `json:"name" form:"name"`
	Message string
}

type SMSRequest struct {
	RequestID    string `json:"requestId"`
	MobileNumber string `json:"mobileNumber"`
	Message      string `json:"message"`
	Channel      string `json:"channel"`
}

type SMSResponse struct {
	Message   string `json:"message"`
	Status    string `json:"status"`
	RequestID string `json:"requestId,omitempty"`
}

type SMSStruct struct {
	Name    string `json:"name" form:"name" validate:"required"`
	Number  string `json:"phoneNumber" form:"phoneNumber" validate:"required,numeric,min=10"`
	Message string `json:"message" form:"message" validate:"required,min=1,max=160"`
}

func main() {
	app := fiber.New()

	err := godotenv.Load()
	if err != nil {
		fmt.Println("Error loading .env file", err)
	}
	app.Static("/", "./src/template")
	api := app.Group("/api")

	api.Post("/email", PostEmail)
	api.Post("/sms", PostSMS)

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

func PostSMS(c *fiber.Ctx) error {
	startTime := time.Now()
	requestID := uuid.NewString()[:16]

	log.Printf("[%s] SMS request started", requestID)
	defer func() {
		log.Printf("[%s] SMS request completed in %v", requestID, time.Since(startTime))
	}()

	SMSApiKey := os.Getenv("SMS_APIKEY")
	SMSApiSecret := os.Getenv("SMS_APISECRET")
	SMSApiNonce := os.Getenv("SMS_NOUNCE")
	SMSUrl := os.Getenv("SMS_URL")

	if SMSApiKey == "" || SMSApiNonce == "" || SMSApiSecret == "" {
		log.Printf("[%s] Missing required environment variables", requestID)
		return c.Status(fiber.StatusInternalServerError).JSON(SMSResponse{
			Message: "Server configuration error",
			Status:  "error",
		})
	}

	var sms SMSStruct
	if err := c.QueryParser(&sms); err != nil {
		log.Printf("[%s] Query parsing error: %v", requestID, err)
		return c.Status(fiber.StatusBadRequest).JSON(SMSResponse{
			Message: "Invalid request parameters",
			Status:  "error",
		})
	}

	if sms.Name == "" || sms.Number == "" || sms.Message == "" {
		log.Printf("[%s] Missing required fields", requestID)
		return c.Status(fiber.StatusBadRequest).JSON(SMSResponse{
			Message: "Name, number and message are required",
			Status:  "error",
		})
	}

	requestBody := SMSRequest{
		RequestID:    requestID,
		MobileNumber: sms.Number,
		Message:      sms.Message,
		Channel:      sms.Name,
	}

	jsonValue, err := json.Marshal(requestBody)

	log.Printf("%s", string(jsonValue))

	if err != nil {
		log.Printf("[%s] JSON marshaling error: %v", requestID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(SMSResponse{
			Message: "Internal server error",
			Status:  "error",
		})
	}

	log.Printf("[%s] Sending SMS to %s via channel %s", requestID, sms.Number, sms.Name)

	client := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequest("POST", SMSUrl, bytes.NewBuffer(jsonValue))
	if err != nil {
		log.Printf("[%s] Request creation error: %v", requestID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(SMSResponse{
			Message: "Failed to create SMS request",
			Status:  "error",
		})
	}

	const separator = " "

	digestString := fmt.Sprintf("%s%s%s%s%s%s%s",
		separator,
		SMSApiKey,
		separator,
		SMSApiNonce,
		separator,
		string(jsonValue),
		separator)

	h := hmac.New(sha512.New, []byte(SMSApiSecret))
	h.Write([]byte(digestString))

	hash := h.Sum(nil)
	signature := base64.StdEncoding.EncodeToString(hash)

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("HmacSHA512 %s:%s:%s", SMSApiKey, SMSApiNonce, signature))

	// Send request
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[%s] API request failed: %v", requestID, err)
		return c.Status(fiber.StatusBadGateway).JSON(SMSResponse{
			Message: "Failed to connect to SMS service",
			Status:  "error",
		})
	}

	body, _ := io.ReadAll(resp.Body)
	log.Printf("[%s] API error response: %s", requestID, string(body))

	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var errorResponse map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&errorResponse); err != nil {
			log.Printf("[%s] Failed to decode error response: %v", requestID, err)
			return c.Status(fiber.StatusBadGateway).JSON(SMSResponse{
				Message: "SMS service returned an error",
				Status:  "error",
			})
		}

		log.Printf("[%s] SMS API error response: %+v", requestID, errorResponse)
		return c.Status(resp.StatusCode).JSON(SMSResponse{
			Message: fmt.Sprintf("SMS service error: %v", errorResponse),
			Status:  "error",
		})
	}

	return c.JSON(SMSResponse{
		Message:   "SMS sent successfully",
		Status:    "success",
		RequestID: requestID,
	})
}
