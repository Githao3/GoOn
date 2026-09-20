package main

import (
	"fmt"
	"os"
)

const version = "0.1.0"

// dispatch 路由子命令；返回要打印到 stdout 的文本与错误。
func dispatch(args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("usage: goon <init|distill|salvage|resume|list|status|new|finalize|version>")
	}
	switch args[0] {
	case "version":
		return version, nil
	default:
		return "", fmt.Errorf("subcommand %q not implemented yet", args[0])
	}
}

func main() {
	out, err := dispatch(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "goon:", err)
		os.Exit(1)
	}
	if out != "" {
		fmt.Println(out)
	}
}
