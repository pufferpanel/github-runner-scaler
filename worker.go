package main

import (
	"context"
	"log"
	"time"
)

const QueueName = "workflow_queue"
const DeleteQueueName = "workflow_delete_queue"

func init() {
	//kick off queue processor
	go runWorker()

	//only run a single deleter
	go deleteWorker()
}

func runWorker() {
	var numVms int
	var err error
	for {
		//check how many VMs we have running, only permit a limit
		//if the limit is reached, sleep and then check later
		numVms, err = getNumVMs()
		if numVms >= NumWorkers || err != nil {
			if err != nil {
				log.Printf("Failed to get number of running VMs: %s", err)
			} else {
				log.Printf("Number of VMs exceeded (%d of %d), sleeping", numVms, NumWorkers)
			}
			time.Sleep(time.Minute)
			continue
		}

		cmd := rdb.BLPop(context.Background(), 0, QueueName)
		if cmd.Err() != nil {
			log.Printf("Error: %s", cmd.Err())
			continue
		}

		id := cmd.String()
		log.Printf("Processing job: %s", id)

		//create VM
		err = cloneVM(id)
		if err != nil {
			log.Fatalf("Failed to create vm: %s", err)
		}
	}
}

func deleteWorker() {
	for {
		cmd := rdb.BLPop(context.Background(), 0, DeleteQueueName)
		if cmd.Err() != nil {
			log.Printf("Error: %s", cmd.Err())
			continue
		}

		id := cmd.String()
		log.Printf("Processing delete job: %s", id)
	}
}
