package main

type parallelThreadsCmd struct {
	ParallelThreads int `short:"t" long:"threads" description:"Run on this many nodes in parallel" default:"50"`
}

type parallelThreadsLongCmd struct {
	ParallelThreads int `long:"threads" description:"Run on this many nodes in parallel" default:"50"`
}
