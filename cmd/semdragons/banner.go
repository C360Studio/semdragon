package main

import "fmt"

func printBanner() {
	fmt.Print(`
  ___                     _
 / __| ___ _ __  _ __  __| |_ _ __ _ __ _ ___ _ _  ___
 \__ \/ -_) '  \| '_ \/ _' | '_/ _' / _' / _ \ ' \(_-<
 |___/\___|_|_|_| .__/\__,_|_| \__,_\__, \___/_||_/__/
                |_|                  |___/
  Agentic Workflow Coordination Framework   v` + Version + `

`)
}
