package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/docker/docker/client"
	"io"
	"log"
	"strings"
	"sync"
)

var containersRaw string
var containers []string
var parserCount int
var printerCount int
var iterations int
var delay int

func main() {
	flag.StringVar(&containersRaw, "Containers", "", "Container ids. Separated by comma.")
	flag.IntVar(&parserCount, "Parsers", 1, "Number of parsers.")
	flag.IntVar(&printerCount, "Printers", 1, "Number of printers.")
	flag.IntVar(&iterations, "Iterations", 5, "Number of iterations. -1 for infinite.")
	flag.IntVar(&delay, "Delay", -1, "Query delay in milliseconds. -1 no delay")

	flag.Parse()
	containers = strings.Split(containersRaw, ",")

	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatal(err)
	}
	defer func(cli *client.Client) {
		err := cli.Close()
		if err != nil {
			log.Fatal(err)
		}
	}(cli)

	var printerWaitGroup sync.WaitGroup
	var parserWaitGroup sync.WaitGroup
	var queryWaitGroup sync.WaitGroup

	printerWaitGroup.Add(printerCount)
	parserWaitGroup.Add(parserCount)
	queryWaitGroup.Add(len(containers))

	dataChan := make(chan string, len(containers))
	parsedDataChan := make(chan ContainerData, len(containers))

	for i := 0; i < len(containers); i++ {
		if len(containers) < 20 {
			go QueryContainerStream(containers[i], dataChan, ctx, cli, &queryWaitGroup)
		} else {
			go QueryContainer(containers[i], dataChan, ctx, cli, &queryWaitGroup)
		}
	}
	for i := 0; i < parserCount; i++ {
		go ParseData(dataChan, parsedDataChan, &parserWaitGroup)
	}
	for i := 0; i < printerCount; i++ {
		go PrintData(parsedDataChan, &printerWaitGroup)
	}

	queryWaitGroup.Wait()
	close(dataChan)
	parserWaitGroup.Wait()
	close(parsedDataChan)
	printerWaitGroup.Wait()
}

func PrintData(parsedDataChan <-chan ContainerData, waitgroup *sync.WaitGroup) {
	for value := range parsedDataChan {
		cpuDelta := value.CPUStats.CPUUsage.TotalUsage - value.PrecpuStats.CPUUsage.TotalUsage
		systemCpuDelta := value.CPUStats.SystemCPUUsage - value.PrecpuStats.SystemCPUUsage
		cpuUsage := (cpuDelta / systemCpuDelta) * int64(value.CPUStats.OnlineCpus) * 100.0

		usedMemory := value.MemoryStats.Usage - value.MemoryStats.Stats.Cache
		memoryUsage := (usedMemory / value.MemoryStats.Limit) * 100.0

		fmt.Printf("Name: %s CPU: %d%% Memory(%%): %d Memory(MB): %d\n", value.Name, cpuUsage, memoryUsage, usedMemory/1000000)
	}

	waitgroup.Done()
}

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
