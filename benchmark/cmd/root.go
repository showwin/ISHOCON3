package cmd

import (
	"github.com/spf13/cobra"

	"github.com/showwin/ISHOCON3/benchmark/bench"
)

var (
	targetURL string

	rootCmd = &cobra.Command{
		Use:   "bench",
		Short: "A benchmark tool for ISHOCON3",
	}
)

// Execute executes the root command.
func Execute() {
	bench.Run(targetURL)
	return
}

func init() {
	rootCmd.Flags().StringVar(&targetURL, "target", "http://localhost:8080", "target URL for benchmark")
}
