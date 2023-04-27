package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"github.com/docker/docker/client"
	"github.com/shirou/gopsutil/cpu"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

var containersRaw string
var containers []string
var parserCount int
var printerCount int
var iterations int
var delay int
var outputType OutputType
var cpuEnergyUsage int

func main() {
	prepareFlags()
	initialize()

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

	doWork(ctx, cli)
}

func initialize() {
	loadCpuEnergyUsage()
}

func doWork(ctx context.Context, cli *client.Client) {
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
	if outputType == Console || outputType == ConsoleApi {
		for i := 0; i < printerCount; i++ {
			go printToConsole(parsedDataChan, &printerWaitGroup)
		}
	}
	if outputType == Api || outputType == ConsoleApi {
		storedDataChan := make(chan ComputedData)
		go updateApiData(parsedDataChan, storedDataChan)
		go StartApi(storedDataChan)
	}

	queryWaitGroup.Wait()
	close(dataChan)
	parserWaitGroup.Wait()
	close(parsedDataChan)
	printerWaitGroup.Wait()
}

func prepareFlags() {
	flag.StringVar(&containersRaw, "Containers", "", "Container ids. Separated by comma.")
	flag.IntVar(&parserCount, "Parsers", 1, "Number of parsers.")
	flag.IntVar(&printerCount, "Printers", 1, "Number of printers.")
	flag.IntVar(&iterations, "Iterations", 5, "Number of iterations. -1 for infinite.")
	flag.IntVar(&delay, "Delay", -1, "Query delay in milliseconds. -1 no delay")
	flag.IntVar((*int)(&outputType), "Output", 0, "Output type. 0 - Console, 1 - Api(/data), 2 - both")

	flag.Parse()
	containers = strings.Split(containersRaw, ",")
}

func updateApiData(parsedDataChan <-chan ContainerData, storedDataChan chan<- ComputedData) {
	for value := range parsedDataChan {
		var computedData = getComputedData(value)
		storedDataChan <- computedData
	}
	close(storedDataChan)
}

func printToConsole(parsedDataChan <-chan ContainerData, waitgroup *sync.WaitGroup) {
	for value := range parsedDataChan {
		var computedData = getComputedData(value)

		fmt.Printf(
			"Name: %s CPU: %f%% Memory(%%): %f Memory(MB): %d Energy Usage: %f\n",
			computedData.Name,
			computedData.CpuUsagePerc,
			computedData.MemoryUsagePerc,
			computedData.UsedMemory/1000000.0,
			computedData.CpuEnergy,
		)
	}

	waitgroup.Done()
}

func loadCpuEnergyUsage() {
	fmt.Println("Loading cpu energy usage")
	cmd := exec.Command("./HardwareMonitor.exe", "1")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Println("Error creating StdoutPipe for command", err)
		return
	}

	if err := cmd.Start(); err != nil {
		fmt.Println("Error starting command", err)
		return
	}

	count := 0
	sum := 0

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		convertedValue, err := strconv.Atoi(scanner.Text())
		if err != nil {
			continue
		}
		count += 1
		sum += convertedValue
	}

	cpuEnergyUsage = sum / count

	if err := cmd.Wait(); err != nil {
		fmt.Println("HardwareMonitor finished with error", err)
		return
	}

	fmt.Println("Finished loading cpu energy usage")
	fmt.Printf("CPU Energy Usage: %d\n", cpuEnergyUsage)
}

func getComputedData(value ContainerData) ComputedData {
	cpuDelta := value.CPUStats.CPUUsage.TotalUsage - value.PrecpuStats.CPUUsage.TotalUsage
	systemCpuDelta := value.CPUStats.SystemCPUUsage - value.PrecpuStats.SystemCPUUsage
	//cpuUsage := (float64(cpuDelta) / float64(systemCpuDelta)) * float64(value.CPUStats.OnlineCpus) * 100.0
	cpuUsage := (float64(cpuDelta) / float64(systemCpuDelta)) * 100.0

	usedMemory := value.MemoryStats.Usage - value.MemoryStats.Stats.Cache
	var memoryUsage float64
	if value.MemoryStats.Limit != 0 {
		memoryUsage = (float64(usedMemory) / float64(value.MemoryStats.Limit)) * 100.0
	} else {
		memoryUsage = 0
	}

	numberCpus := len(value.CPUStats.CPUUsage.PercpuUsage)
	cpuPerc, _ := cpu.Percent(0, false)
	cpuEnergy := (float64(cpuUsage) / 100) * float64(cpuEnergyUsage) * cpuPerc[0] / 100
	if cpuEnergy < 0 {
		cpuEnergy = 0
	}

	return ComputedData{
		Name:            value.Name[1:],
		CpuDelta:        cpuDelta,
		SystemCpuDelta:  systemCpuDelta,
		CpuUsagePerc:    cpuUsage,
		UsedMemory:      usedMemory,
		MemoryUsagePerc: memoryUsage,
		NumberCpus:      numberCpus,
		CpuEnergy:       cpuEnergy,
		TimeStamp:       time.Now(),
	}
}
