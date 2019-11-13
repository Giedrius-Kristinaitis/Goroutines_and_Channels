/*
 * Long functions and everything is in one file because I'm lazy right now and a bit short on time
 */

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"sort"
)

// some constants
var DATA_FILES = [...]string{
	"IFF-7-2_Kristinaitis_Giedrius_L1_dat_1.json",
	"IFF-7-2_Kristinaitis_Giedrius_L1_dat_2.json",
	"IFF-7-2_Kristinaitis_Giedrius_L1_dat_3.json",
}

var LETTERS = [...]string{
	"b", "c", "d", "f", "g", "h", "j", "k", "l", "m", "n", "p",
	"r", "s", "t", "v", "z",
}

const RESULT_FILE = "IFF-7-2_Kristinaitis_Giedrius_L2_rez.txt"
const WORKER_THREAD_COUNT = 4
const DATA_ELEMENT_COUNT = 25
const DATA_ARRAY_SIZE = 10

// Product struct
type Product struct {
	Title    string
	Price    float32
	Quantity int32
	Result   string
	Invalid  bool
}

// entry point of the program
func main() {
	prepareResultFile(RESULT_FILE)

	for i := 0; i < len(DATA_FILES); i++ {
		mainThreadAction(DATA_FILES[i])
	}
}

// spawns threads
func mainThreadAction(filename string) {
	// create required channels
	mainToDataChannel := make(chan Product)
	resultToMainChannel := make(chan Product)
	resultToMainElementCountChannel := make(chan int)
	dataToWorkerChannel := make(chan Product, DATA_ELEMENT_COUNT)
	dataToResultChannel := make(chan bool)
	workerToResultChannel := make(chan Product, DATA_ELEMENT_COUNT)
	addRequestChannel := make(chan string)
	removeRequestChannel := make(chan string)

	// spawn threads
	for i := 0; i < WORKER_THREAD_COUNT; i++ {
		go workerThreadAction(dataToWorkerChannel, workerToResultChannel, removeRequestChannel)
	}

	go dataThreadAction(dataToWorkerChannel, mainToDataChannel, dataToResultChannel, addRequestChannel, removeRequestChannel)
	go resultThreadAction(workerToResultChannel, resultToMainChannel, dataToResultChannel, resultToMainElementCountChannel)

	// read data
	data := readData(filename)

	// send data to data thread
	for i := 0; i < len(data); i++ {
		addRequestChannel <- "add"
		response := <-addRequestChannel

		if response != "ok" {
			if i >= 1 {
				i--
			}

			continue;
		}

		mainToDataChannel <- data[i]
	}

	// read data from result thread and print it
	resultElementCount := <-resultToMainElementCountChannel

	results := make([]Product, resultElementCount)

	for i := 0; i < resultElementCount; i++ {
		results[i] = <-resultToMainChannel
	}

	printResults(RESULT_FILE, filename, results)
}

// reads data from a file and passes it to worker threads
func dataThreadAction(dataToWorkerChannel chan Product, mainToDataChannel chan Product, dataToResultChannel chan bool, addRequestChannel chan string, removeRequestChannel chan string) {
	var data = make([]Product, DATA_ARRAY_SIZE)
	totalElementsReceived := 0
	totalElementsSent := 0
	index := -1

	for {
		if index > -1 && index < DATA_ARRAY_SIZE - 1 && totalElementsReceived < DATA_ELEMENT_COUNT {
			select {
				case addRequest := <-addRequestChannel:
					// receive a product from the main thread
					if addRequest == "add" {
						addRequestChannel <- "ok"
						product := <-mainToDataChannel
						index++
						totalElementsReceived++
						data[index] = product
					} else {
						addRequestChannel <- "not ok"
					}
				default:
					select {
						case removeRequest := <-removeRequestChannel:
							// send a product to a worker thread
							if removeRequest == "remove" {
								product := data[index]
								dataToWorkerChannel <- product
								index--
								totalElementsSent++
							}
						default:
							continue
					}
			}
		} else if index == -1 {
			addRequest := <-addRequestChannel

			// receive a product from the main thread
			if addRequest == "add" {
				addRequestChannel <- "ok"
				product := <-mainToDataChannel
				index++
				totalElementsReceived++
				data[index] = product
			} else {
				addRequestChannel <- "not ok"
			}
		} else if index > -1 {
			removeRequest := <-removeRequestChannel

			// send a product to a worker thread
			if removeRequest == "remove" {
				product := data[index]
				dataToWorkerChannel <- product
				index--
				totalElementsSent++
			}
		}

		if totalElementsSent == DATA_ELEMENT_COUNT {
			break
		}
	}

	// write invalid products to dataToWorkerChannel to stop worker threads
	for i := 0; i < WORKER_THREAD_COUNT; i++ {
		invalidProduct := Product{Invalid: true}
		dataToWorkerChannel <- invalidProduct
	}

	// notify result thread that there will be no more data
	dataToResultChannel <- true
}

// processes data received from data thread and sends it to result thread
func workerThreadAction(dataToWorkerChannel chan Product, workerToResultChannel chan Product, removeRequestChannel chan string) {
	for {
		removeRequestChannel <- "remove"

		select {
			case product := <-dataToWorkerChannel:
				if product.Invalid {
					break
				}

				// process product
				result := ""

				for i := 0; i < int(product.Price*float32(product.Quantity)*float32(product.Quantity)); i++ {
					result = string(product.Title[i%(len(product.Title)-1)])
				}

				product.Result = result

				// filter result and write to result channel
				resultValid := false

				for _, letter := range LETTERS {
					if letter == result {
						resultValid = true
					}
				}

				if resultValid {
					workerToResultChannel <- product
				}
			default:
				continue
		}
	}
}

// takes results from worker threads and sends them to the main thread
func resultThreadAction(workerToResultChannel chan Product, resultToMainChannel chan Product, dataToResultChannel chan bool, resultToMainElementCountChannel chan int) {
	results := make([]Product, DATA_ELEMENT_COUNT)
	index := 0

	// read the results from worker threads
	NotSoInfiniteLoop:
	for {
		select {
			case product := <-workerToResultChannel:
				results[index] = product
				index++
			default:
				select {
					case noMoreData := <-dataToResultChannel:
						if noMoreData {
							break NotSoInfiniteLoop
						}
					default:
						continue
				}
		}
	}

	// trim results
	resultSlice := results[0:index]

	// send result count to the main thread
	resultToMainElementCountChannel <- index

	// sort the results
	sort.Slice(resultSlice, func(i, j int) bool {
		return resultSlice[i].Result < resultSlice[j].Result
	})

	// send the results to the main thread
	for i := 0; i < index; i++ {
		resultToMainChannel <- resultSlice[i]
	}
}

// parses JSON from a file into a Product array
func readData(file string) []Product {
	var products []Product

	data, _ := ioutil.ReadFile(file)

	_ = json.Unmarshal(data, &products)

	return products
}

// prints results to a file
func printResults(resultFilename string, dataFilename string, products []Product) {
	file, _ := os.OpenFile(resultFilename, os.O_APPEND|os.O_WRONLY, 0644)

	fmt.Fprintf(file, "----------------------------------------------------------\n")
	fmt.Fprintf(file, dataFilename + " Results\n")
	fmt.Fprintf(file, "----------------------------------------------------------\n")
	fmt.Fprintf(file, "%25s|%10s|%10s|%10s\n", "Title", "Price", "Quantity", "Result")
	fmt.Fprintf(file, "----------------------------------------------------------\n")

	if len(products) > 0 {
		for _, product := range products {
			fmt.Fprintf(file, "%25s|%10f|%10d|%10s\n", product.Title, product.Price, product.Quantity, product.Result)
		}
	} else {
		fmt.Fprintf(file, "No results - no elements match the filter\n")
	}

	fmt.Fprintf(file, "----------------------------------------------------------\n")

	file.Close()
}

// prepares result file for writing
func prepareResultFile(filename string) {
	// delete old file if exists
	_ = os.Remove(filename)

	// recreate the file
	file, _ := os.Create(RESULT_FILE)
	defer file.Close()
}
