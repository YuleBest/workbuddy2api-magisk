package region

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// InteractiveMenu prints the numbered country list and returns the selected
// country. Empty, invalid, or EOF input returns nil.
func InteractiveMenu(
	countries []Country,
	detectedIOS2, title string,
	in io.Reader,
	out io.Writer,
) *Country {
	if len(countries) == 0 {
		fmt.Fprintln(out, "（无可选地区）")
		return nil
	}
	if title == "" {
		title = "国际版账号需完善注册地区"
	}
	fmt.Fprintf(out, "\n%s（请输入编号选择注册地区）：\n", title)
	for i, country := range countries {
		mark := ""
		if country.IOS2 == detectedIOS2 {
			mark = "  ◀ 检测到当前地区"
		}
		fmt.Fprintf(out, "%3d) %-4s %-8s %s%s\n",
			i+1, country.IOS2, country.Code, country.EnName, mark)
	}
	fmt.Fprint(out, "\n请选择注册地区编号（回车取消）: ")
	reader, ok := in.(*bufio.Reader)
	if !ok {
		reader = bufio.NewReader(in)
	}
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		fmt.Fprintln(out)
		return nil
	}
	raw := strings.TrimSpace(line)
	fmt.Fprintln(out)
	if raw == "" {
		return nil
	}
	index, err := strconv.Atoi(raw)
	if err != nil || index < 1 || index > len(countries) {
		fmt.Fprintf(out, "无效编号 %s\n", raw)
		return nil
	}
	return &countries[index-1]
}
