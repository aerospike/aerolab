package parallelize

import (
	"sync"
)

func ForEach[T any](arr []T, fn func(T)) {
	wg := &sync.WaitGroup{}
	wg.Add(len(arr))

	for _, item := range arr {
		go func(x T) {
			defer wg.Done()

			fn(x)
		}(item)
	}

	wg.Wait()
}

func ForEachLimit[T any](arr []T, limit int, fn func(T)) {
	wg := &sync.WaitGroup{}
	wg.Add(len(arr))

	limiter := make(chan bool, limit)

	for _, item := range arr {
		limiter <- true
		go func(x T) {
			defer wg.Done()

			fn(x)
			<-limiter
		}(item)
	}

	wg.Wait()
}

func Map[T1 any, T2 any](arr []T1, fn func(T1) T2) []T2 {
	wg := &sync.WaitGroup{}
	wg.Add(len(arr))

	output := make([]T2, len(arr))

	for i := range arr {
		go func(index int, x T1) {
			defer wg.Done()

			result := fn(x)
			output[index] = result

		}(i, arr[i])
	}

	wg.Wait()
	return output
}

func MapLimit[T1 any, T2 any](arr []T1, limit int, fn func(T1) T2) []T2 {
	wg := &sync.WaitGroup{}
	wg.Add(len(arr))

	output := make([]T2, len(arr))
	limiter := make(chan bool, limit)

	for i := range arr {
		limiter <- true
		go func(index int, x T1) {
			defer wg.Done()

			result := fn(x)
			output[index] = result

			<-limiter
		}(i, arr[i])
	}

	wg.Wait()
	return output
}
