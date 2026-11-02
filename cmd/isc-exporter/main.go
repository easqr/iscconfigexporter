package main

import (
	"flag"
	"fmt"
	exporter "github.com/elianderson/iscutilities/configexporter"
	"os"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	p := flag.String("config", "config.json", "configuration file")
	validate := flag.Bool("validate", false, "validate local configuration without credentials or network")
	flag.Parse()
	f, e := os.Open(*p)
	if e != nil {
		return e
	}
	defer f.Close()
	_, e = exporter.LoadConfig(f)
	if e != nil {
		return e
	}
	if *validate {
		fmt.Println("configuration valid")
		return nil
	}
	return fmt.Errorf("export execution is not wired yet")
}
