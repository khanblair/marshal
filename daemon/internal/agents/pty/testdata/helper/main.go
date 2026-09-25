// Command helper is the small program that the PTY adapter tests run inside a terminal. It uses
// the standard library only, so it builds the same way on macOS, Linux, and Windows, and it needs
// no shell. The first argument picks what it does.
package main

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const longSleep = time.Hour

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: helper <mode> [arguments]")
		os.Exit(2)
	}
	mode, rest := os.Args[1], os.Args[2:]
	run, ok := modes()[mode]
	if !ok {
		fmt.Printf("unknown mode %q\n", mode)
		os.Exit(2)
	}
	run(rest)
}

// modes lists what the helper can do.
func modes() map[string]func(args []string) {
	return map[string]func(args []string){
		"echo":      echo,
		"lines":     lines,
		"burst":     burst,
		"trickle":   trickle,
		"upper":     upper,
		"stubborn":  stubborn,
		"interrupt": interrupt,
		"graceful":  graceful,
		"exit":      exit,
		"args":      printArgs,
		"pwd":       pwd,
		"env":       env,
		"size":      size,
	}
}

// echo says ready, then repeats each line it reads with a prefix, until its input ends.
func echo([]string) {
	fmt.Println("ready")
	in := bufio.NewScanner(os.Stdin)
	for in.Scan() {
		fmt.Println("echo: " + strings.TrimSpace(in.Text()))
	}
}

// lines prints N numbered lines and exits.
func lines(args []string) {
	for i := 1; i <= number(args, 0); i++ {
		fmt.Printf("line %d\n", i)
	}
}

// burst prints N bytes of letters in large writes, without a line break so that the terminal
// adds none, and then a marker line. With "hold" as the second argument it then stays alive.
func burst(args []string) {
	total := number(args, 0)
	block := make([]byte, 64<<10)
	for i := range block {
		block[i] = byte('a' + i%26)
	}
	for written := 0; written < total; {
		n := min(len(block), total-written)
		if _, err := os.Stdout.Write(block[:n]); err != nil {
			os.Exit(1)
		}
		written += n
	}
	fmt.Println()
	fmt.Println("BURST-END")
	if len(args) > 1 && args[1] == "hold" {
		time.Sleep(longSleep)
	}
}

// trickle prints N small pieces, a millisecond apart, and then exits.
func trickle(args []string) {
	for i := 0; i < number(args, 0); i++ {
		fmt.Print("piece;")
		time.Sleep(time.Millisecond)
	}
	fmt.Println()
	fmt.Println("TRICKLE-END")
}

// upper reads one line, prints it in capitals, and exits.
func upper([]string) {
	fmt.Println("ready")
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	fmt.Println(strings.ToUpper(strings.TrimSpace(line)))
}

// stubborn ignores every polite way of being told to end, and only a kill stops it.
func stubborn([]string) {
	signal.Ignore(os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	fmt.Println("ready")
	time.Sleep(longSleep)
}

// interrupt waits for Ctrl-C, says so, and exits.
func interrupt([]string) {
	got := make(chan os.Signal, 1)
	signal.Notify(got, os.Interrupt)
	fmt.Println("ready")
	<-got
	fmt.Println("got interrupt")
}

// graceful says goodbye when it is asked to end.
func graceful([]string) {
	got := make(chan os.Signal, 1)
	signal.Notify(got, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	fmt.Println("ready")
	<-got
	fmt.Println("goodbye")
}

// exit prints a line and ends with the code that it is given.
func exit(args []string) {
	fmt.Println("exiting")
	os.Exit(number(args, 0))
}

// printArgs prints the arguments after the mode, then waits for input so that a session stays up.
func printArgs(args []string) {
	fmt.Println("args: " + strings.Join(args, " "))
	fmt.Println("ready")
	time.Sleep(longSleep)
}

// pwd prints the working folder.
func pwd([]string) {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Println("pwd error: " + err.Error())
		return
	}
	fmt.Println("pwd: " + dir)
}

// env prints the value of each variable that it is given, as NAME=value.
func env(args []string) {
	for _, name := range args {
		fmt.Printf("env %s=[%s]\n", name, os.Getenv(name))
	}
}

// size prints the size of its terminal now, and again each time the size changes.
func size([]string) {
	if runtime.GOOS == "windows" {
		fmt.Println("size: unsupported")
		return
	}
	changed := make(chan os.Signal, 1)
	notifySizeChange(changed)
	printSize()
	for range changed {
		printSize()
	}
}

// printSize prints one line with the columns and the rows.
func printSize() {
	cols, rows, err := terminalSize()
	if err != nil {
		fmt.Println("size error: " + err.Error())
		return
	}
	fmt.Printf("size: %dx%d\n", cols, rows)
}

// number reads argument i as a number, or exits with a message.
func number(args []string, i int) int {
	if i >= len(args) {
		return 0
	}
	n, err := strconv.Atoi(args[i])
	if err != nil {
		fmt.Printf("bad number %q\n", args[i])
		os.Exit(2)
	}
	return n
}
