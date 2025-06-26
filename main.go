package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/google/go-github/v73/github"
	"github.com/pufferpanel/github-runner-scaler/env"
	"github.com/redis/go-redis/v9"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

var Label = env.Get("github.label")

var rdb = redis.NewClient(&redis.Options{
	Addr:     env.Get("redis.host"),
	Password: env.Get("redis.password"),
	DB:       0, // use default DB
})

var GithubSecretToken = []byte(env.Get("github.secret"))
var webLogger = log.New(os.Stdout, "[Web] ", log.LstdFlags|log.Lmicroseconds)

func main() {
	r := gin.Default()

	r.POST("/queue", func(c *gin.Context) {
		var signature = c.GetHeader("X-Hub-Signature-256")
		if signature == "" {
			webLogger.Printf("No signature")
			c.Status(http.StatusUnauthorized)
			return
		}
		signature = strings.TrimPrefix(signature, "sha256=")

		original := io.LimitReader(c.Request.Body, 1024*1024)

		source := new(bytes.Buffer)
		_, err := io.Copy(source, original)

		if err != nil {
			webLogger.Printf("Error reading body: %s", err)
			c.Status(http.StatusInternalServerError)
			return
		}

		data := source.Bytes()
		hash, err := generateSha256(GithubSecretToken, data)
		if err != nil {
			webLogger.Printf("Error calclating hash: %s", err)
			c.Status(http.StatusInternalServerError)
			return
		}

		if subtle.ConstantTimeCompare(hash, []byte(signature)) != 1 {
			webLogger.Printf("Invalid signature (expected %s, got %s)", signature, hash)
			c.Status(http.StatusUnauthorized)
			return
		}

		request := &github.WorkflowJobEvent{}
		err = json.NewDecoder(bytes.NewReader(data)).Decode(request)
		if err != nil {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		onWorkflowJob(request)
		c.Status(http.StatusAccepted)
	})

	StartWorkers()
	err := r.Run()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		panic(err)
	}
}

func onWorkflowJob(request *github.WorkflowJobEvent) {
	if request.WorkflowJob == nil {
		return
	}

	if !contains(request.WorkflowJob.Labels, Label) {
		return
	}

	var queue = ""
	if *request.Action == "queued" {
		queue = QueueName
	} else if *request.Action == "completed" {
		queue = DeleteQueueName
	}

	if queue == "" {
		return
	}

	//this is a job we care about, let's start our queue stuff
	//push it to redis, it will handle the queue
	webLogger.Printf("Adding %d to queue", *request.WorkflowJob.RunID)
	rdb.RPush(context.Background(), queue, fmt.Sprintf("%d", *request.WorkflowJob.RunID))
}

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

func generateSha256(token []byte, payload []byte) ([]byte, error) {
	h := hmac.New(sha256.New, token)
	_, err := h.Write(payload)
	if err != nil {
		return nil, err
	}
	result := h.Sum(nil)

	res := make([]byte, hex.EncodedLen(len(result)))
	_ = hex.Encode(res, result)
	return res, nil
}
