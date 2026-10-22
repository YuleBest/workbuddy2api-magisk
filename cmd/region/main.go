// global_region completes the registration region for a WorkBuddy global account.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"workbuddy2api/internal/region"
)

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, os.Stdin, os.Stdout, os.Stderr, nil))
}

func run(
	args []string,
	getenv func(string) string,
	stdin io.Reader,
	stdout, stderr io.Writer,
	client *region.Client,
) int {
	flags := flag.NewFlagSet("global_region", flag.ContinueOnError)
	flags.SetOutput(stderr)
	uid := flags.String("uid", "", "账号 UID（必填）")
	auto := flags.String("auto", "", "直接指定 IOS2（如 SG），跳过交互菜单")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "国际版注册地区自动完善（调试）")
		fmt.Fprintln(stderr, "用法: global_region --uid UID [--auto IOS2]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if strings.TrimSpace(*uid) == "" {
		fmt.Fprintln(stderr, "global_region: --uid 必填")
		flags.Usage()
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(stderr, "global_region: 不支持的参数 %q\n", flags.Arg(0))
		return 2
	}
	token := getenv("WORKBUDDY_TOKEN")
	if token == "" {
		fmt.Fprintln(stderr, "需要 WORKBUDDY_TOKEN 环境变量")
		return 1
	}
	if client == nil {
		client = region.NewClient(nil)
	}
	ctx := context.Background()

	err := client.ActivateRegion(ctx, token, *uid)
	if err == nil {
		fmt.Fprintln(stdout, "register: 成功")
		return 0
	}
	if !region.IsRegionRequired(err) {
		fmt.Fprintf(stdout, "register: 失败 %v\n", err)
		return 1
	}

	countries, err := client.FetchCountries(ctx, true)
	if err != nil {
		fmt.Fprintf(stdout, "拉取地区列表失败: %v\n", err)
		return 1
	}
	var pick *region.Country
	if *auto != "" {
		for i := range countries {
			if countries[i].IOS2 == *auto {
				pick = &countries[i]
				break
			}
		}
		if pick == nil {
			all, _ := client.FetchCountries(ctx, false)
			for i := range all {
				if all[i].IOS2 == *auto {
					pick = &all[i]
					break
				}
			}
		}
		if pick == nil {
			fmt.Fprintf(stdout, "未找到 %s\n", *auto)
			return 1
		}
	} else {
		detected, _ := client.DetectRegion(ctx, token)
		pick = region.InteractiveMenu(
			countries,
			detected.IOS2,
			"",
			menuInput(stdin),
			stdout,
		)
		if pick == nil {
			fmt.Fprintln(stdout, "已取消")
			return 1
		}
	}

	if err := client.CompleteFlow(ctx, token, *uid, pick, true); err != nil {
		fmt.Fprintf(stdout, "complete: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, "complete: 地区已完善，register 成功")
	return 0
}

func menuInput(stdin io.Reader) io.Reader {
	if file, ok := stdin.(*os.File); ok {
		if info, err := file.Stat(); err == nil && info.Mode()&os.ModeCharDevice != 0 {
			return stdin
		}
	}
	tty, err := os.Open("/dev/tty")
	if err == nil {
		return tty
	}
	return strings.NewReader("")
}
