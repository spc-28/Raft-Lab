package main

import (
	"raft-lab/api"
	"raft-lab/raft"
)

func main() {
	controller := raft.NewClusterController(5)

	controller.Start()

	server := api.NewServer(controller)
	server.Run(":8080")
}
