// Package sanepanic allows for central processing of panics. This may be desireable if you have a system that requires
// shutdown on panic, rather than command-line printing. For instance, if you have a graphical application using a window,
// you may want to (if possible) print the error to a console or error box informing the user what happened and then save a log file
// or serialize the known sane parts of the program state.
//
// Similar to the log package, this provides a PanicHandler type which allows for multiple "instances" of this package to be run at once,
// if for whatever reason you wish to split up a single process into multiple chunks that handle their panicking individually.
package sanepanic

import (
	"fmt"
	"os"
	"runtime"
	"sync"
)

// The PanicInfo struct roughly contains the data normally printed to terminal
// on a panic. Info is the exact data returned by recover (which in turn is the data passed into panic(data)).
//
// StackTrace is the information returned by runtime.Stack at the time Handler is called. Due to the way panic and defer
// work in Go, this stack trace will print the line your code panicked on.
type PanicInfo struct {
	Info       interface{}
	StackTrace string
}

var (
	internalPanicHandler *PanicHandler
	mu                   *sync.Mutex
)

// Automatically called when the package is imported (but only called once per program execution)
func init() {
	internalPanicHandler = NewPanicHandler(DefaultCleanupHandler)
	mu = &sync.Mutex{}
}

// Restart should be called if a panic is recovered from successfully and the program
// is deemed worthy to continue.
//
// Strictly speaking, it can be called whenever, but it will seem to an outside observer to be a no-op
func Restart() {
	mu.Lock()
	defer mu.Unlock()
	internalPanicHandler.Done()
	internalPanicHandler = NewPanicHandler(internalPanicHandler.cleanupFunc)
}

// Allows you to tailor your recovery function to the PanicInfo forwarded to the listener
// you should almost always set this, since the default handler is basically just panic() without the program termination
func SetCleanupHandler(newCleanupFunc func(PanicInfo)) {
	mu.Lock()
	defer mu.Unlock()
	internalPanicHandler.SetCleanupHandler(newCleanupFunc)
}

// Exits the listener if no panics have been received, or waits until panic handling has been done
// otherwise
func Done() {
	mu.Lock()
	defer mu.Unlock()
	internalPanicHandler.Done()
}

// At the beginning of any Goroutine, call "defer sanepanic.Handler()"
// to forward the panic to the package's listener and call your cleanup handling function
func Handle() {
	mu.Lock()
	defer mu.Unlock()
	internalPanicHandler.Handle()
}

// Prints a panic almost exactly like the runtime does, except the program doesn't exit.
func DefaultCleanupHandler(info PanicInfo) {
	fmt.Fprintf(os.Stderr, "Panic: %v\n%s", info.Info, info.StackTrace)
}

/* Actual implementation, to use if you want multiple central handlers for some reason */

// A central processor for panicking. This more or less duplicates the functionality of the package.
// The only missing function is Restart() which can be emulated by calling YourPanicHandler.Done() followed by creating
// a new one.
type PanicHandler struct {
	panicChan   chan PanicInfo
	wg          *sync.WaitGroup
	cleanupFunc func(PanicInfo)
	mu          *sync.Mutex
}

// Creates a new panic handler AND makes it start listening for panics.
func NewPanicHandler(cleanupFunc func(PanicInfo)) *PanicHandler {
	ph := &PanicHandler{panicChan: make(chan PanicInfo), wg: &sync.WaitGroup{}, cleanupFunc: cleanupFunc, mu: &sync.Mutex{}}
	go ph.listen()
	return ph
}

// Handles exactly one panic
func (ph *PanicHandler) listen() {
	ph.wg.Add(1)
	info, ok := <-ph.panicChan
	if ok {
		ph.handlepanic(info)
	} else {
		close(ph.panicChan)
		ph.wg.Done()
	}
}

func (ph *PanicHandler) handlepanic(info PanicInfo) {
	ph.mu.Lock()
	ph.cleanupFunc(info)
	close(ph.panicChan)
	ph.wg.Done()
	mu.Unlock()
}

// Stops the listener (if it has not already been used). If a panic has been detected, waits for the processing to be done
// before proceeding
func (ph *PanicHandler) Done() {
	select {
	case info, ok := <-ph.panicChan: // Handles the case where we somehow do this exactly when a panic is sent
		if ok {
			ph.handlepanic(info)
		}
	default: // Only executes if no panics were sent AND panicChan has yet to be closed
		close(ph.panicChan)
	}
	ph.wg.Wait()
}

// Swaps out the panic handling functions provided at construction.
func (ph *PanicHandler) SetCleanupHandler(newCleanupFunc func(PanicInfo)) {
	ph.mu.Lock()
	defer ph.mu.Unlock()
	ph.cleanupFunc = newCleanupFunc
}

// As with the package level function, calling defer YourPanicHandler.Handle()
// at the top of a panicky goroutine will allow it to be processed by this panic handler.
func (ph *PanicHandler) Handle() {
	err := recover()
	if err != nil {
		buf := make([]byte, 10000)
		traceSize := runtime.Stack(buf, true)
		buf = buf[:traceSize]
		select {
		case ph.panicChan <- PanicInfo{Info: err, StackTrace: string(buf)}:
		default:
		}
	}
}
