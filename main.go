package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/google/go-github/v73/github"
	"github.com/pufferpanel/github-runner-scaler/env"
	"github.com/redis/go-redis/v9"
	"net/http"
)

var Label = env.Get("github.label")

var rdb = redis.NewClient(&redis.Options{
	Addr:     env.Get("redis.host"),
	Password: env.Get("redis.password"),
	DB:       0, // use default DB
})

func main() {
	r := gin.Default()

	r.POST("/queue", func(c *gin.Context) {
		request := &github.WorkflowJobEvent{}
		err := c.BindJSON(request)
		if err != nil {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		onWorkflowJob(request)
		c.Status(http.StatusAccepted)
	})

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
