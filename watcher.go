package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/docker/docker/client"
	"io"
	"log"
	"sync"
)

func ParseData(dataChan <-chan string, parsedDataChan chan<- ContainerData, waitgroup *sync.WaitGroup) {
	var parsedData ContainerData
	for value := range dataChan {
		err := json.Unmarshal([]byte(value), &parsedData)
		if err != nil {
			fmt.Println("Failed to parse")
			continue
		}
		parsedDataChan <- parsedData
	}

	waitgroup.Done()
}

func QueryContainerStream(
	container string,
	dataChan chan<- string,
	ctx context.Context,
	cli *client.Client,
	waitgroup *sync.WaitGroup,
) {
	containerStats, _ := cli.ContainerStats(ctx, container, true)

	buf := make([]byte, 1)
	buf2 := make([]byte, 0)
	for i := 0; i < iterations; i++ {

		for !json.Valid(buf2) || len(buf2) < 5 {
			_, err := io.ReadFull(containerStats.Body, buf)
			if err != nil {
				log.Fatal(err)
			}
			buf2 = append(buf2, buf...)
		}

		dataChan <- string(buf2)

		buf = make([]byte, 1)
		buf2 = make([]byte, 0)
		if iterations == -1 {
			i = -2
		}
	}

	waitgroup.Done()
}

func QueryContainer(
	container string,
	dataChan chan<- string,
	ctx context.Context,
	cli *client.Client,
	waitgroup *sync.WaitGroup,
) {
	buf := new(bytes.Buffer)

	for i := 0; i < iterations; i++ {
		containerStats, err := cli.ContainerStats(ctx, container, false)
		if err != nil {
			log.Fatal(err)
		}
		_, err = buf.ReadFrom(containerStats.Body)
		if err != nil {
			log.Fatal(err)
		}

		dataChan <- buf.String()
		buf.Reset()

		if iterations == -1 {
			i = -2
		}
	}

	waitgroup.Done()
}
