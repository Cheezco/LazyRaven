package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/docker/docker/client"
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
	PrepareFlags()

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

	DoWork(ctx, cli)
}

func DoWork(ctx context.Context, cli *client.Client) {
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
		go PrintToConsole(parsedDataChan, &printerWaitGroup)
	}

	queryWaitGroup.Wait()
	close(dataChan)
	parserWaitGroup.Wait()
	close(parsedDataChan)
	printerWaitGroup.Wait()
}

func PrepareFlags() {
	flag.StringVar(&containersRaw, "Containers", "", "Container ids. Separated by comma.")
	flag.IntVar(&parserCount, "Parsers", 1, "Number of parsers.")
	flag.IntVar(&printerCount, "Printers", 1, "Number of printers.")
	flag.IntVar(&iterations, "Iterations", 5, "Number of iterations. -1 for infinite.")
	flag.IntVar(&delay, "Delay", -1, "Query delay in milliseconds. -1 no delay")

	flag.Parse()
	containers = strings.Split(containersRaw, ",")
}

func PrintToConsole(parsedDataChan <-chan ContainerData, waitgroup *sync.WaitGroup) {
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
