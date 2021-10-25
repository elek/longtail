package main

import (
	"fmt"
	"github.com/spf13/cobra"
	"os"
)

var RootCmd cobra.Command = cobra.Command{
	Use: "longtail",
}

func main() {
	err := RootCmd.Execute()
	if err != nil {
		fmt.Printf("%+v", err)
		os.Exit(-1)
	}
}
