// author: Gary A. Stafford
// site: https://programmaticponderings.com
// license: MIT License
// purpose: RESTful Go implementation of github.com/aws/aws-sdk-go/service/dynamodb package
//          Provides ability to put text in request payload to DynamoDB table
//          by https://github.com/aws/aws-sdk-go
// modified: 2021-06-13

package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/labstack/gommon/log"
)

type NlpText struct {
	Timestamp int64  `json:"timestamp"` // record date/time
	Hash      string `json:"hash"`      // MD5 hash of text
	Text      string `json:"text"`      // The text in the request
}

var (
	logLevel   = getEnv("LOG_LEVEL", "1") // DEBUG
	serverPort = getEnv("DYNAMO_PORT", ":8080")
	apiKey     = getEnv("API_KEY", "ChangeMe")
	e          = echo.New()
)

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func getHealth(c echo.Context) error {
	healthStatus := struct {
		Status string `json:"status"`
	}{"Up"}
	return c.JSON(http.StatusOK, healthStatus)
}

func getMD5Hash(text string) string {
	hash := md5.New()
	hash.Write([]byte(text))

	return hex.EncodeToString(hash.Sum(nil))
}

func writeToDynamo(c echo.Context) error {
	//Initialize a session that the SDK will use to load
	//credentials from the shared credentials file ~/.aws/credentials
	//and region from the shared configuration file ~/.aws/config.
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))

	// Create DynamoDB client
	svc := dynamodb.New(sess)

	tableName := "NLPText"

	var nlpText NlpText
	jsonMap := make(map[string]interface{})
	err := json.NewDecoder(c.Request().Body).Decode(&jsonMap)
	if err != nil {
		log.Errorf("json.NewDecoder Error: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, err)
	}

	text := (jsonMap["text"]).(string)
	nlpText.Hash = getMD5Hash(text)
	// truncate long text inputs
	if len(text) > 1000 {
		text = text[0:1000] + "..."
	}
	nlpText.Timestamp = time.Now().Unix()
	nlpText.Text = text

	av, err := dynamodbattribute.MarshalMap(nlpText)
	if err != nil {
		log.Errorf("dynamodbattribute.MarshalMap Error: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, err)
	}

	input := &dynamodb.PutItemInput{
		Item:      av,
		TableName: aws.String(tableName),
	}

	_, err = svc.PutItem(input)
	if err != nil {
		log.Errorf("svc.PutItem Error: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, err)
	}

	return c.JSON(http.StatusOK, nil)
}

func run() error {
	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	e.Use(middleware.KeyAuthWithConfig(middleware.KeyAuthConfig{
		KeyLookup: "header:X-API-Key",
		Skipper: func(c echo.Context) bool {
			if strings.HasPrefix(c.Request().RequestURI, "/health") {
				return true
			}
			return false
		},
		Validator: func(key string, c echo.Context) (bool, error) {
			log.Debugf("API_KEY: %v", apiKey)
			return key == apiKey, nil
		},
	}))

	// Routes
	e.GET("/health", getHealth)
	e.POST("/record", writeToDynamo)

	// Start server
	return e.Start(serverPort)
}

func init() {
	level, _ := strconv.Atoi(logLevel)
	e.Logger.SetLevel(log.Lvl(level))
}

func main() {
	if err := run(); err != nil {
		e.Logger.Fatal(err)
		os.Exit(1)
	}
}
