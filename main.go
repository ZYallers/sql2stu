package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
)

var tableName string

func convert(str string) string {
	var modelName string

	// 转换数据表名为 model 名称
	matchTableName := regexp.MustCompile("CREATE TABLE `([a-z_0-9]+)`.*").FindAllStringSubmatch(str, -1)
	if len(matchTableName) > 0 {
		for _, row := range matchTableName {
			tableName = row[1]
			modelName = strings.ReplaceAll(strings.Title(strings.ReplaceAll(tableName, `_`, ` `)), ` `, ``)
			str = strings.ReplaceAll(str, row[0], `type `+modelName+` struct {`)

			// 转换数据表结束符
			tableTail := regexp.MustCompile("\\) (ENGINE|AUTO_INCREMENT=|ROW_FORMAT).*").FindAllString(str, 1)
			if len(tableTail) > 0 {
				str = strings.ReplaceAll(str, tableTail[0], "}")
			}
		}
	}

	// 转换为小写并且加上前缀;号
	str = strings.ReplaceAll(str, ` NOT NULL`, `;not null`)
	str = strings.ReplaceAll(str, ` NULL`, `;null`)
	str = strings.ReplaceAll(str, ` AUTO_INCREMENT`, `;AUTO_INCREMENT`)
	str = strings.ReplaceAll(str, ` unsigned`, `;unsigned`)
	str = regexp.MustCompile(" DEFAULT '(.*?)'").ReplaceAllString(str, `;default:'$1'`)
	str = strings.ReplaceAll(str, ` DEFAULT CURRENT_TIMESTAMP`, `;default:CURRENT_TIMESTAMP`)

	// 转换备注
	matchComment := regexp.MustCompile(" COMMENT '(.*?)',").FindAllStringSubmatch(str, -1)
	if len(matchComment) > 1 {
		for _, row := range matchComment {
			// 英文逗号转中文，要不然会正则匹配有问题，再把中文逗号转成|符号
			comment := strings.ReplaceAll(strings.ReplaceAll(row[1], `,`, `，`), `，`, `|`)
			str = strings.ReplaceAll(str, row[0], ",  // "+comment)
		}
	}

	// 转换字段名
	matchFieldName := regexp.MustCompile("`([a-z_0-9A-Z]+)` (.*,)").FindAllStringSubmatch(str, -1)
	if len(matchFieldName) > 1 {
		for _, row := range matchFieldName {
			fieldName := row[1]
			attribute := row[2]
			newFieldName := strings.ReplaceAll(strings.Title(strings.ReplaceAll(fieldName, `_`, ` `)), ` `, ``)
			str = strings.ReplaceAll(str, row[0], "`"+newFieldName+"` json:\""+fieldName+"\" gorm:\"column:"+fieldName+";"+attribute)
		}
	}

	// 转换属性
	str = regexp.MustCompile("`([a-z_0-9A-Z]+)` (.*?;)(bigint|mediumint|int|integer|smallint|tinyint)(.*),").ReplaceAllString(str, "$1    int    `${2}type:$3$4\"`")
	str = regexp.MustCompile("`([a-z_0-9A-Z]+)` (.*?;)(decimal|float)(.*),").ReplaceAllString(str, "$1    float64    `${2}type:$3$4\"`")
	str = regexp.MustCompile("`([a-z_0-9A-Z]+)` (.*?;)(varchar|char)(.*),").ReplaceAllString(str, "$1    string    `${2}type:$3$4\"`")
	str = regexp.MustCompile("`([a-z_0-9A-Z]+)` (.*?;)(text)(.*),").ReplaceAllString(str, "$1    string    `${2}type:$3$4\"`")
	str = regexp.MustCompile("`([a-z_0-9A-Z]+)` (.*?;)(timestamp;)(.*),").ReplaceAllString(str, "$1    time.Time    `${2}type:$3$4\"`")

	// 删除不知道怎么转换的属性
	str = strings.ReplaceAll(str, ` ON UPDATE CURRENT_TIMESTAMP`, ``)
	str = regexp.MustCompile("\\s*(PRIMARY|UNIQUE)? KEY .*(,)?").ReplaceAllString(str, ``)

	// 增加表名的常量定义
	str = fmt.Sprintf("const %sTN = `%s`\n\n%s", modelName, tableName, str)

	// 增加 package 和 import
	if regexp.MustCompile(`time.Time`).MatchString(str) {
		str = "package table\n\nimport \"time\"\n\n" + str
	} else {
		str = "package table\n\n" + str
	}

	return str
}

func shellEcho(str, msgType string) {
	switch msgType {
	case "ok":
		fmt.Printf("\033[32m%s\033[0m\n", str)
	case "err":
		fmt.Printf("\033[31m%s\033[0m\n", str)
	case "tip":
		fmt.Printf("\033[33m%s\033[0m\n", str)
	case "title":
		fmt.Printf("\033[42;34m%s\033[0m\n", str)
	default:
		fmt.Printf("%s\n", str)
	}
}

func execShell(name string, arg ...string) ([]byte, error) {
	// 函数返回一个*Cmd，用于使用给出的参数执行name指定的程序
	cmd := exec.Command(name, arg...)

	// 读取io.Writer类型的cmd.Stdout，再通过bytes.Buffer(缓冲byte类型的缓冲器)将byte类型转化为[]byte类型
	var out bytes.Buffer
	cmd.Stdout = &out

	// Run执行c包含的命令，并阻塞直到完成。这里stdout被取出，cmd.Wait()无法正确获取stdin,stdout,stderr，则阻塞在那了。
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func showHelpMsg() {
	shellEcho("->:p		--(print)显示已输入内容；", "tip")
	shellEcho("->:r		--(reset)清空已输入内容；", "tip")
	shellEcho("->:c		--(convert)转义已输入内容；", "tip")
	shellEcho("->:cp		--(convert & export)转义已输入内容并导出Go文件；", "tip")
	shellEcho("->:q		--(quit)退出程序；", "tip")
	shellEcho("->:h		--(help)显示帮助信息！", "tip")
}

func main() {
	reader := bufio.NewReader(os.Stdin)
	showHelpMsg()
	shellEcho(`请输入您的内容：`, "title")
	var buf bytes.Buffer

LOOP:
	for {
		text, _ := reader.ReadString('\n')
		if runtime.GOOS == "windows" {
			text = strings.Replace(text, "\r\n", "", -1)
		} else {
			text = strings.Replace(text, "\n", "", -1)
		}
		switch text {
		case ":h":
			showHelpMsg()
		case ":c", ":cp":
			var exportText []byte
			shellEcho("Convert Result:", "title")
			shellEcho("-----------BEGIN-----------", "tip")
			output := convert(buf.String())
			dir, _ := os.Getwd()
			cacheFile := dir + "/sql2stu.cache"
			if err := ioutil.WriteFile(cacheFile, []byte(output), os.ModePerm); err != nil {
				shellEcho("ioutil.WriteFile Error: "+err.Error(), "err")
			} else {
				if _, err := execShell("gofmt", "-l", "-w", "-s", cacheFile); err != nil {
					shellEcho("execShell Error: "+err.Error(), "err")
				} else {
					if body, err := ioutil.ReadFile(cacheFile); err != nil {
						shellEcho("ioutil.ReadFile Error: "+err.Error(), "err")
					} else {
						if err := os.Remove(cacheFile); err != nil {
							shellEcho("os.Remove Error: "+err.Error(), "err")
						} else {
							exportText = body
							shellEcho(string(body), "ok")
						}
					}
				}
			}
			shellEcho("------------END------------", "tip")
			if text == ":cp" && exportText != nil && tableName != "" {
				exportFile := fmt.Sprintf("%s/%s.go", dir, tableName)
				if err := ioutil.WriteFile(exportFile, exportText, os.ModePerm); err != nil {
					shellEcho("ioutil.WriteFile Error: "+err.Error(), "err")
				} else {
					tableName, exportText = "", nil
					shellEcho(fmt.Sprintf("convert result have exported to \"%s\" file", exportFile), "tip")
				}
			}
		case ":q":
			shellEcho("已退出！", "ok")
			break LOOP
		case ":r":
			buf.Reset()
			shellEcho("已清空！", "ok")
			shellEcho("请重新输入您的内容：", "title")
		case ":p":
			shellEcho("您已输入的内容：", "title")
			shellEcho(buf.String(), "ok")
		default:
			buf.WriteString(text + "\n")
		}
	}
}
