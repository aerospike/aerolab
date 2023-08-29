package main

type parallelThreads struct {
	ParallelThreads int `short:"t" long:"threads" description:"Run on this many nodes in parallel" default:"50"`
}
