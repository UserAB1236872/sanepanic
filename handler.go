// Package sanepanic allows for central processing of panics. This may be desireable if you have a system that requires
// shutdown on panic, rather than command-line printing. For instance, if you have a graphical application using a window,
// you may want to (if possible) print the error to a console or error box informing the user what happened and then save a log file
// or serialize the known sane parts of the program state.
//
// Similar to the log package, this provides a PanicHandler type which allows for multiple "instances" of this package to be run at once,
// if for whatever reason you wish to split up a single process into multiple chunks that handle their panicking individually.
//
// One note about the handler is that if the handler ceases (as with calling Done or returning "false" in your HandlerFunc)
// any panics in functions running the handler will pass silently without being acknowledged.
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
type Info struct {
	Info       interface{}
	StackTrace string
}

// A HandlerFunc handles a panic and returns true if the panic
// handler should continue running
type HandlerFunc func(Info) (keepHandling bool)

var (
	internalPanicHandler *Handler
	mu                   *sync.Mutex
)

// Automatically called when the package is imported (but only called once per program execution)
func init() {
	internalPanicHandler = NewHandler(DefaultHandlerFunc)
	mu = &sync.Mutex{}
}

// Restart should be called if the handler is inadvertantly cancelled.
// It automatically registers the same HandlerFunc the previous Handler was using
func Restart() {
	mu.Lock()
	defer mu.Unlock()
	internalPanicHandler.Done()
	internalPanicHandler = NewHandler(internalPanicHandler.handle)
}

// Allows you to tailor your recovery function to the PanicInfo forwarded to the listener
// you should almost always set this, since the default handler is basically just panic() without the program termination
func SetHandlerFunc(newHandler HandlerFunc) {
	mu.Lock()
	defer mu.Unlock()
	internalPanicHandler.SetHandlerFunc(newHandler)
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
func Forward() {
	mu.Lock()
	defer mu.Unlock()
	err := recover() // Have to do recover directly in deferred function
	internalPanicHandler.forward(err)
}

// Prints a panic almost exactly like the runtime does, except the program doesn't exit.
func DefaultHandlerFunc(info Info) bool {
	fmt.Fprintf(os.Stderr, "Panic: %v\n%s", info.Info, info.StackTrace)
	return true
}

/* Actual implementation, to use if you want multiple central handlers for some reason */

// A central processor for panicking. This more or less duplicates the functionality of the package.
// The only missing function is Restart() which can be emulated by calling YourPanicHandler.Done() followed by creating
// a new one.
type Handler struct {
	panicChan chan Info
	quit      chan struct{}
	handle    HandlerFunc
	mu        *sync.Mutex
}

// Creates a new panic handler AND makes it start listening for panics.
func NewHandler(handler HandlerFunc) *Handler {
	ph := &Handler{panicChan: make(chan Info), handle: handler, mu: &sync.Mutex{}, quit: make(chan struct{})}
	go ph.listen()
	return ph
}

// Handles panics
func (ph *Handler) listen() {
	for info := range ph.panicChan {
		if !ph.handleForwardedPanic(info) {
			close(ph.quit)
			break
		}
	}
}

func (ph *Handler) handleForwardedPanic(info Info) bool {
	ph.mu.Lock()
	defer ph.mu.Unlock()
	return ph.handle(info)
}

// Stops the listener (if it has not already been used). If a panic has been detected, waits for the processing to be done
// before proceeding
func (ph *Handler) Done() {
	select {
	case info, ok := <-ph.panicChan: // Handles the case where we somehow do this exactly when a panic is sent
		if ok {
			close(ph.quit)
			close(ph.panicChan)
			ph.mu.Lock()
			defer ph.mu.Unlock()
			ph.handleForwardedPanic(info)
		}
	default: // Only executes if no panics were sent AND panicChan has yet to be closed
		close(ph.panicChan)
	}
}

// Swaps out the panic handling functions provided at construction.
func (ph *Handler) SetHandlerFunc(newHandler HandlerFunc) {
	ph.mu.Lock()
	defer ph.mu.Unlock()
	ph.handle = newHandler
}

// As with the package level function, calling defer YourPanicHandler.Forward()
// at the top of a panicky goroutine will allow it to be processed by this panic handler.
func (ph *Handler) Forward() {
	err := recover()
	ph.forward(err)
}

func (ph *Handler) forward(err interface{}) {
	if err != nil {
		buf := make([]byte, 10000)
		traceSize := runtime.Stack(buf, true)
		buf = buf[:traceSize]
		select {
		case ph.panicChan <- Info{Info: err, StackTrace: string(buf)}:
		case <-ph.quit:
		}
	}
}
