package sanepanic_test

import (
	"github.com/Jragonmiris/sanepanic"
	"sync"
	"testing"
)

func TestSomething(t *testing.T) {
	defer func() {
		err := recover()
		if err == nil {
			t.Fatal("No panic")
		}
	}()

	panic("Panic!")
}

func TestCatchAll(t *testing.T) {
	mu := &sync.Mutex{}
	i := 0
	wg := &sync.WaitGroup{}

	handler := func(info sanepanic.Info) bool {
		blankInfo := sanepanic.Info{}
		if info == blankInfo {
			t.Errorf("No panic info exists")
		} else {
			t.Logf("Received valid panic data: %v", info)
		}

		mu.Lock()
		defer mu.Unlock()
		i++

		return true
	}

	sanepanic.SetHandlerFunc(handler)
	wg.Add(5)
	for i := 0; i < 5; i++ {
		go func() {
			defer wg.Done()
			defer sanepanic.Forward()
			panic("Arghlbarg")
		}()
	}

	wg.Wait()
}

func TestCatchSome(t *testing.T) {
	mu := &sync.Mutex{}
	i := 0
	wg := &sync.WaitGroup{}

	handler := func(info sanepanic.Info) bool {
		blankInfo := sanepanic.Info{}
		if info == blankInfo {
			t.Errorf("No panic info exists")
		} else {
			t.Logf("Received valid panic data: %v", info)
		}

		mu.Lock()
		defer mu.Unlock()
		i++
		if i == 5 {
			return false
		} else if i > 5 {
			t.Fatalf("Forwarded too many panics")
		}

		return true
	}

	sanepanic.SetHandlerFunc(handler)

	wg.Add(10)
	for i := 0; i < 10; i++ {
		go func() {
			defer wg.Done()
			defer sanepanic.Forward()
			panic("Arghlbarg")
		}()
	}

	wg.Wait()
}

func TestNested(t *testing.T) {
	genHandler := func(quit chan struct{}) sanepanic.HandlerFunc {
		return func(sanepanic.Info) bool {
			close(quit)
			return false
		}
	}

	wg := &sync.WaitGroup{}

	// Create 10 "servers" that spawn 10 "workers"
	// one of which panics
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			quit := make(chan struct{})
			ph := sanepanic.NewHandler(genHandler(quit))
			for j := 0; j < 10; j++ {
				j := j
				wg.Add(1)

				go func() {
					defer wg.Done()
					defer ph.Forward()
					if j == 9 {
						panic(j)
					}

					<-quit
				}()
			}

			wg.Done()
		}()
	}

	wg.Wait() // Will deadlock if test fails
}
