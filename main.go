// author: Gary A. Stafford
// site: https://programmaticponderings.com
// license: MIT License
// purpose: Provides fast natural language detection for various languages
//          by https://github.com/rylans/getlang

package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
	"github.com/sirupsen/logrus"
	"net/http"
	"os"
	"strings"
	"time"
)

// NLPText is the text in the request payload
type NlpText struct {
	Timestamp int64  `json:"timestamp"` // record date/time
	Hash      string `json:"hash"`      // MD5 hash of text
	Text      string `json:"text"`      // The text in the request
}

var (
	serverPort = ":" + getEnv("DYNAMO_PORT", "")
	apiKey     = getEnv("API_KEY", "")
	log        = logrus.New()

	// Echo instance
	e = echo.New()
)

func init() {
	log.Formatter = &logrus.JSONFormatter{
		TimestampFormat: time.RFC3339Nano,
	}
	log.Out = os.Stdout
	log.SetLevel(logrus.DebugLevel)
}

func main() {
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
	e.Logger.Fatal(e.Start(serverPort))
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func getHealth(c echo.Context) error {
	var response interface{}
	err := json.Unmarshal([]byte(`{"status":"UP"}`), &response)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err)
	}

	return c.JSON(http.StatusOK, response)
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
		return echo.NewHTTPError(http.StatusInternalServerError, err)
	}

	input := &dynamodb.PutItemInput{
		Item:      av,
		TableName: aws.String(tableName),
	}

	_, err = svc.PutItem(input)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err)
	}

	return c.JSON(http.StatusOK, "{}")
}
