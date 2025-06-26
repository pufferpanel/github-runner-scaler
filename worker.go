package main

import (
	"context"
	"log"
	"os"
	"time"
)

const QueueName = "workflow_queue"
const DeleteQueueName = "workflow_delete_queue"

func StartWorkers() {
	//kick off queue processor
	go runWorker()

	//only run a single deleter
	go deleteWorker()
}

func runWorker() {
	logger := log.New(os.Stdout, "[Runner] ", log.LstdFlags|log.Lmicroseconds)
	var numVms int
	var err error
	for {
		//check how many VMs we have running, only permit a limit
		//if the limit is reached, sleep and then check later
		numVms, err = getNumVMs()
		if numVms >= NumWorkers || err != nil {
			if err != nil {
				logger.Printf("Failed to get number of running VMs: %s", err)
			} else {
				logger.Printf("Number of VMs exceeded (%d of %d), sleeping", numVms, NumWorkers)
			}
			time.Sleep(time.Minute)
			continue
		}

		cmd := rdb.BLPop(context.Background(), 0, QueueName)
		if cmd.Err() != nil {
			logger.Printf("Error: %s", cmd.Err())
			continue
		}

		id := cmd.Val()[1]
		logger.Printf("Processing job: %s", id)

		//create VM
		err = cloneVM(id)
		if err != nil {
			logger.Printf("Failed to create vm: %s", err)
		}
	}
}

func deleteWorker() {
	logger := log.New(os.Stdout, "[Deleter] ", log.LstdFlags|log.Lmicroseconds)
	for {
		cmd := rdb.BLPop(context.Background(), 0, DeleteQueueName)
		if cmd.Err() != nil {
			logger.Printf("Error: %s", cmd.Err())
			continue
		}

		id := cmd.String()
		logger.Printf("Processing delete job: %s", id)
	}
}
