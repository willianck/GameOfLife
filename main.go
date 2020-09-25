package main

import (
	"flag"
	"math"
)

// golParams provides the details of how to run the Game of Life and which image to load.
type golParams struct {
	turns       int
	threads     int
	imageWidth  int
	imageHeight int

}

// ioCommand allows requesting behaviour from the io (pgm) goroutine.
type ioCommand uint8

// This is a way of creating enums in Go.
// It will evaluate to:
//		ioOutput 	= 0
//		ioInput 	= 1
//		ioCheckIdle = 2
const (
	ioOutput ioCommand = iota
	ioInput
	ioCheckIdle
)

// cell is used as the return type for the testing framework.
type cell struct {
	x, y int
}

// distributorToIo defines all chans that the distributor goroutine will have to communicate with the io goroutine.
// Note the restrictions on chans being send-only or receive-only to prevent bugs.
// here you receive the input values from io and then send the output values to io
type distributorToIo struct {
	command chan<- ioCommand
	idle    <-chan bool

	filename  chan<- string
	inputVal  <-chan uint8 // left arrow means receive , right arrow means send


	// added code here . channel to output image
	outputVal chan<-  uint8
}

// ioToDistributor defines all chans that the io goroutine will have to communicate with the distributor goroutine.
// Note the restrictions on chans being send-only or receive-only to prevent bugs.
// here you send the input values to io and receive the output values from distributor
type ioToDistributor struct {
	command <-chan ioCommand
	idle    chan<- bool

	filename  <-chan string
	inputVal  chan<- uint8
	// added code here . channel to output image
	outputVal <-chan uint8
}

// distributorChans stores all the chans that the distributor goroutine will use.
type distributorChans struct {
	io distributorToIo
}

// ioChans stores all the chans that the io goroutine will use.
type ioChans struct {
	distributor ioToDistributor
}

// gameOfLife is the function called by the testing framework.
// It makes some channels and starts relevant goroutines.
// It places the created channels in the relevant structs.
// It returns an array of alive cells returned by the distributor.
func gameOfLife(p golParams, keyChan <-chan rune) []cell { //gets called when tests are called
	var dChans distributorChans
	var ioChans ioChans

	ioCommand := make(chan ioCommand)
	dChans.io.command = ioCommand
	ioChans.distributor.command = ioCommand //tell io what to do

	ioIdle := make(chan bool)
	dChans.io.idle = ioIdle
	ioChans.distributor.idle = ioIdle

	ioFilename := make(chan string)
	dChans.io.filename = ioFilename
	ioChans.distributor.filename = ioFilename

	inputVal := make(chan uint8)
	dChans.io.inputVal = inputVal
	ioChans.distributor.inputVal = inputVal //input values recieved on io

	// channel to output the image of the board
	outputVal := make(chan uint8)
	dChans.io.outputVal = outputVal
	ioChans.distributor.outputVal = outputVal

	aliveCells := make(chan []cell)


	// Array of byte Channel to send and receive  the world between the workers and distributor
	var wChan = make([] chan uint8,p.threads)
	for i:=0; i<p.threads; i++ {
		wChan[i] = make(chan  uint8)
	}

	//Array of channel for turns in gol
 var tChan = make([]chan int, p.threads )
 for i:=0; i<p.threads; i++ {
 	 tChan[i]= make(chan int )
 }

	//  array of  byte Channel to send the updated halos between workers after each turn
	var hChan = make([]chan uint8,p.threads)
	for i:=0; i<p.threads; i++{
		hChan[i]= make(chan uint8)
	}


	// array of bool Channel to notify when workers reconstruct the world
	var bChan = make([] chan bool, p.threads)
	for i:=0; i<p.threads; i++{
		bChan[i] = make(chan bool )
	}


	// array of bool Channel to notify when workers should pause the world or continue
	var pChan = make([] chan bool, p.threads)
	for i:=0; i<p.threads; i++{
		pChan[i] = make(chan bool )
	}


	go distributor(p, dChans, aliveCells,wChan,keyChan,tChan,bChan,pChan)
	go pgmIo(p, ioChans)
	// Worker threads here
	for i:=0; i<p.threads; i++ {
		// startY and endY are the starting height and end height of the board  each
		// worker is allocated  to
		// the rounding takes into consideration multiple of 2 threads.
		startY:=int(math.Floor(float64(p.imageHeight)/float64(p.threads)*float64(i)))
		endY:=int(math.Floor((float64(p.imageHeight)/float64(p.threads))*(float64(i)+1)))
		var m = modulo(i+1,p.threads)
		// worker go routine which performs gol logic and used a range of channels created
		// for communicating with the  world , receiving turns, signals  and halo exchange
		go worker(i,p,wChan[i],startY,endY,hChan[i],hChan[m],tChan[i],bChan[i],pChan[i])
	}

	alive := <-aliveCells
	return alive
}

// main is the function called when starting Game of Life with 'make gol'
// Do not edit until Stage 2.
func main() {
	var params golParams

	flag.IntVar(
		&params.threads,
		"t",
		8,
		"Specify the number of worker threads to use. Defaults to 8.")

	flag.IntVar(
		&params.imageWidth,
		"w",
		512,
		"Specify the width of the image. Defaults to 512.")

	flag.IntVar(
		&params.imageHeight,
		"h",
		512,
		"Specify the height of the image. Defaults to 512.")

	flag.Parse()

	params.turns = 10000000000
	keyChan := make(chan rune)
	startControlServer(params)
	go getKeyboardCommand(keyChan)
	gameOfLife(params, keyChan)
	StopControlServer()
}
