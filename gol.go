package main

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

type Activity int

const (
	lives Activity = 0
	dies  Activity = 1
	halos int = 2
)

//this is the main part to code
// distributor divides the work between workers and interacts with other goroutines.
func distributor(p golParams, d distributorChans, alive chan []cell,worker []chan uint8, key <-chan rune,t []chan int, b []chan bool,pause []chan bool) {
	world := makeWorld(p.imageHeight,p.imageWidth)

	// Request the io goroutine to read in the image with the given filename.
	d.io.command <- ioInput
	d.io.filename <- strings.Join([]string{strconv.Itoa(p.imageWidth), strconv.Itoa(p.imageHeight)}, "x")

	// The io goroutine sends the requested image byte by byte, in rows.
	createWorld(p,d,world)

	// ticker timer for ticker thread
	ticker := time.NewTicker(2*time.Second)

	// New state of the board is achieved after each turn of gol logic
	for turns := 0; turns < p.turns; turns++ {
		for i:= range t {
			t[i] <- turns
		}
		select {
		case x := <-key:
			switch x {
			case 's':
				setBooleanChannel(b,true)
				setBooleanChannel(pause,false)
				reconstructWorld(p,worker,world)
				outputWorld(p, world, d, turns)
			 //generate a PGM file with the current state of the board.
			case 'p':
				fmt.Println(turns)
				setBooleanChannel(b,false)
				setBooleanChannel(pause,true)
				y := <-key
				for y != 'p' {
					y = <-key
				}
				setBooleanChannel(pause,false)
				fmt.Println("Continuing")
			case 'q':
				setBooleanChannel(b,true)
				setBooleanChannel(pause, false)
				reconstructWorld(p,worker,world)
				outputWorld(p, world, d, turns)
				d.io.command <- ioCheckIdle
				<-d.io.idle
				StopControlServer()
				os.Exit(0) //0 means nothing went wrong
				//generate a PGM file with the final state of the board and then terminate the program.
			}

			case <-ticker.C:
			  	setBooleanChannel(b,true)
				setBooleanChannel(pause,false)
				reconstructWorld(p,worker,world)
			  	count := getNumberOfAliveCells(p,world)
			  	fmt.Println("The number of alive cells:", count)


		default:
			setBooleanChannel(b,false)
			setBooleanChannel(pause, false)
			if turns==0 {
				// split the world into slices between the worker threads
				splitWorld(p,worker,world)
			}

			// each worker send the  slice of the world back to distributor  to
			// be assembled into the full board
            if turns == p.turns-1 {
				reconstructWorld(p,worker,world)
			}
		}
	}

//-------------------------------------------------------------------------------------------------------------------------

	//Create an empty slice to store coordinates of cells that are still alive after p.turns are done.
	// Go through the world and append the cells that are still alive.
	finalAlive := appendAliveCells(p,world)
	// output the pgm files
	outputWorld(p, world, d, p.turns)

	// Make sure that the Io has finished any output before exiting.
	d.io.command <- ioCheckIdle
	<-d.io.idle

	// Return the coordinates of cells that are still alive.
	alive <- finalAlive //FOR TESTING FRAMEWORK

}

// function to output the world in a pgm file using the channel
func outputWorld(p golParams, world [][] byte, d distributorChans,turns int) {
	  d.io.command <- ioOutput
	  d.io.filename <- strings.Join([]string{strconv.Itoa(p.imageWidth), strconv.Itoa(p.imageHeight), strconv.Itoa(turns)}, "x")
	  for y := 0; y < p.imageHeight; y++ {
		 for x := 0; x < p.imageWidth; x++ {
			  d.io.outputVal <- world[y][x]
		 }
	  }

  }
//-----------------------------------------------------------------------------------------------------------------------------------------------

// go routine
// worker go routine function that deals with the part of the world
// allocated and performs game of life logic
func worker(index int,p golParams , sliceWorld chan  uint8, startY int , endY int, prev chan uint8, next chan uint8 ,t chan int,b chan bool,ps chan bool) {
	height := (endY - startY) + halos
	workerWorld := makeWorld(height, p.imageWidth)

	for {
		// receiving turns channel
		turns := <-t
		// receiving gol input signal  channel
		sendToDis := <-b
		// receiving pause channel
		pause := <-ps

		if pause {
			for pause == true {
				pause = <-ps
			}
		}

		// At beginning of gol workers receives the world at  first turn
		if turns == 0 {
			receiveWorldByteByByte(sliceWorld, workerWorld, height, p.imageWidth)
		}
		preSlice := makeWorld(height, p.imageWidth)
		for i := range preSlice {
			copy(preSlice[i], workerWorld[i])
		}
		// Updates the state of the cells on the worker world
		setCellState(p, preSlice, workerWorld, height-1, p.imageWidth)

		//Sends back the slice to distributor on last turn , user input and every two seconds
		if turns == p.turns-1 || sendToDis {
			sendWorldByteByByte(sliceWorld, workerWorld, height-1, p.imageWidth)
		} else {
			// Halo exchange between workers
			if index%2 == 0 { // even
				sendHalo(prev, next, workerWorld, p.imageWidth, endY, startY)
				receiveHalo(prev, next, workerWorld, p.imageWidth, endY, startY)
			} else { // odd
				receiveHalo(prev, next, workerWorld, p.imageWidth, endY, startY)
				sendHalo(prev, next, workerWorld, p.imageWidth, endY, startY)
			}
		}
	}
}


//function that generates the cell activity based on neighbouring cells
//  Modified to work with workers and their halos
func CellActivity(p golParams ,x int ,y int, world [][] byte ) Activity {
	aliveNeighbour := 0
	for i := -1; i < 2; i++ {
		for j := -1; j < 2; j++ {
			if !(i == 0 && j == 0 ) {
				if world[(y+i)][modulo(x+j, p.imageWidth)] == 0xFF { aliveNeighbour++ }
			}
		}
	}
	if aliveNeighbour < 2 || aliveNeighbour >3  {return dies}
	if aliveNeighbour == 2 && world[y][x]==0 {return dies}
	return lives
}

// Return the arithmetic modulo of a number
func modulo(x int , mod int ) int {
	if x>=mod { x= x%mod }
	if x<0 { x= x + mod }
	return x
}

// function that reconstruct the world from the slices sent by the worker threads
func reconstructWorld(p golParams, worker  []chan uint8, world [][]byte  ) {
	for i := 0; i < p.threads; i++ {
		start := int(math.Floor(float64(p.imageHeight) / float64(p.threads) * float64(i)))
		end := int(math.Floor((float64(p.imageHeight) / float64(p.threads)) * (float64(i) + 1)))
		for y := start; y < end; y++ {
			for x := 0; x < p.imageWidth; x++ {
				world[y][x] = <-worker[i]
			}
		}
	}
}

// function that split the world into the different slices sent to the worker threads
func splitWorld(p golParams, worker []chan uint8, world [][] byte){
	for i := 0; i < p.threads; i++ {
		start := int(math.Floor(float64(p.imageHeight) / float64(p.threads) * float64(i)))
		end := int(math.Floor((float64(p.imageHeight) / float64(p.threads)) * (float64(i) + 1)))
		for y := start - 1; y < end+1; y++ {
			for x := 0; x < p.imageWidth; x++ {
				worker[i] <- world[modulo(y, p.imageHeight)][x] // general case including 0 and 15
			}
		}
	}
}

// function  which reads the pgm file and initializes the board  at the beginning of  gol
func createWorld(p golParams, d distributorChans, world [][]byte){
	for y := 0; y < p.imageHeight; y++ {
		for x := 0; x < p.imageWidth; x++ {
			val := <-d.io.inputVal
			if val != 0 {
				fmt.Println("Alive cell at", x, y)
				world[y][x] = val
			}
		}
	}
}

// passes the boolean  value to the array of channel
func setBooleanChannel(bChan []chan bool, bVal bool) {
	for i:= range bChan {
		bChan[i] <- bVal
	}
}

// Get the count of alive cells in the gol board
func getNumberOfAliveCells(p golParams, world [][]byte) int{
	count := 0
	for y := 0; y < p.imageHeight; y++ {
		for x := 0; x < p.imageWidth; x++ {
			if world[y][x] != 0x00 {
				count = count + 1
			}
		}
	}
	return count
}


func appendAliveCells(p golParams, world [][]byte) []cell {
	var finalAlive []cell
	// Go through the world and append the cells that are still alive.
	for y := 0; y < p.imageHeight; y++ {
		for x := 0; x < p.imageWidth; x++ {
			if world[y][x] != 0x00{
				finalAlive = append(finalAlive, cell{x: x, y: y})
			}
		}
	}
	return finalAlive
}

//a  function which creates a new world
func makeWorld(y int, x int) [][]byte {
	world := make([][]byte, y)
	for i := range world {
		world[i] = make([]byte, x)
	}
	return world
}

// a function to  send the world byte by byte via a Channel
func sendWorldByteByByte(wChan chan uint8, world [][]byte , height int, width int){
	for y := 1; y < height; y++ {
		for x := 0; x < width; x++ {
			wChan <- world[y][x]
		}
	}
}

// a function used to receives the world byte by byte via a Channel
func receiveWorldByteByByte(wChan chan uint8, world [][]byte , height int, width int){
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			world[y][x] = <-wChan
		}
	}
}

// Changes the state of the cell on the board , flipping them to either dead or alive
func setCellState(p golParams, preSlice [][]byte, workerWorld [][]byte ,height int, width int,){
	for y := 1; y < height; y++ {
		for x := 0; x < width; x++ {
			// Placeholder for the actual Game of Life logic: flips alive cells to dead and dead cells to alive
			if CellActivity(p, x, y, preSlice) == lives {
				workerWorld[y][x] = 0xFF
			} // cell lives
			if CellActivity(p, x, y, preSlice) == dies {
				workerWorld[y][x] = 0x00
			} //  cell dies
		}
	}
}

// a function to send the halos from the current  worker  to previous and next worker
func sendHalo(prev chan uint8, next chan uint8, workerWorld [][]byte, width int, endY int, startY int){
	for x := 0; x < width; x++ {
		//  top row  sent to previous worker
		prev <- workerWorld[1][x]
		//  bottom row sent to next worker
		next <- workerWorld[(endY - startY)][x]

	}
}

// a function for the current worker to receive the halos from the next and previous worker
func receiveHalo(prev chan uint8, next chan uint8, workerWorld [][]byte, width int, endY int, startY int) {
	for x := 0; x < width; x++ {
		// current worker receives row into its bottom halo
		workerWorld[(endY-startY)+1][x] = <-next
		// current worker receives row into its top halo
		workerWorld[0][x] = <-prev
	}
}


