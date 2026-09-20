package main

import (
	"fmt"
	"os"

	"goon/internal/cli"
)

const version = "0.1.0"

// dispatch 路由子命令；返回要打印到 stdout 的文本与错误。
func dispatch(args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("usage: goon <init|new|save|distill|salvage|resume|list|status|finalize|version>")
	}
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	app := cli.New(cwd)
	switch args[0] {
	case "version":
		return version, nil
	case "init":
		return "", app.Init()
	case "list":
		return app.List()
	case "status":
		return app.Status()
	case "new", "save":
		src := "manual"
		if len(args) > 1 {
			src = args[1]
		}
		return app.New(src)
	case "distill":
		if len(args) < 3 {
			return "", fmt.Errorf("usage: goon distill <source> <session-file>")
		}
		return app.DistillFile(args[2], args[1])
	case "salvage":
		if len(args) < 3 {
			return "", fmt.Errorf("usage: goon salvage <source> <session-file>")
		}
		return app.SalvageFile(args[2], args[1])
	case "resume":
		id := ""
		if len(args) > 1 {
			id = args[1]
		}
		return app.Resume(id)
	case "finalize":
		if len(args) < 2 {
			return "", fmt.Errorf("usage: goon finalize <id>")
		}
		return "", app.Finalize(args[1])
	default:
		return "", fmt.Errorf("unknown subcommand %q", args[0])
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
