package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"runtime"
	"strings"
)

var tableName string

func convert(str string) string {
	var modelName string

	// 全部转为小写
	str = strings.ToLower(str)

	// 转换数据表名为 model 名称
	matchTableName := regexp.MustCompile("create table `([a-z_0-9A-Z]+)`.*").FindAllStringSubmatch(str, -1)
	if len(matchTableName) > 0 {
		for _, row := range matchTableName {
			tableName = row[1]
			modelName = strings.ReplaceAll(strings.Title(strings.ReplaceAll(tableName, `_`, ` `)), ` `, "")
			str = strings.ReplaceAll(str, row[0], `type `+modelName+` struct {`)
			// 转换数据表结束符
			tableTail := regexp.MustCompile("\\) (engine|auto_increment=|row_format).*").FindAllString(str, 1)
			if len(tableTail) > 0 {
				str = strings.ReplaceAll(str, tableTail[0], "}")
			}
		}
	}

	// 转换为小写并且加上前缀;号
	str = strings.ReplaceAll(str, ` null`, `;null`)
	str = strings.ReplaceAll(str, ` not null`, `;not null`)
	str = strings.ReplaceAll(str, `not;null`, `;not null`)
	str = strings.ReplaceAll(str, ` auto_increment`, `;primaryKey;autoIncrement`)
	str = strings.ReplaceAll(str, ` unsigned`, `;unsigned`)
	str = regexp.MustCompile("character set (.*?) ").ReplaceAllString(str, ";default:''")
	str = regexp.MustCompile("(tinyint|smallint|mediumint|int|bigint|decimal|dec|numeric|fixed|float|double|double precision|real)(.*) default '(.*?)'").
		ReplaceAllString(str, `$1$2;default:$3`) // 去掉数值型默认值中的单引号
	str = regexp.MustCompile(` default '(.*?)'`).ReplaceAllString(str, `;default:'$1'`)
	str = strings.ReplaceAll(str, ` default current_timestamp`, `;default:current_timestamp`)
	str = strings.ReplaceAll(str, ` ;`, `;`)

	// 转换备注
	matchComment := regexp.MustCompile(" comment '(.*?)',").FindAllStringSubmatch(str, -1)
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
			newFieldName := strings.ReplaceAll(strings.Title(strings.ReplaceAll(fieldName, `_`, ` `)), ` `, "")
			str = strings.ReplaceAll(str, row[0], "`"+newFieldName+"` json:\""+fieldName+"\" gorm:\"column:"+fieldName+";"+attribute)
		}
	}

	// 转换属性
	str = regexp.MustCompile("`([a-z_0-9A-Z]+)` (.*?;)(bigint|mediumint|int|integer|smallint|tinyint)(.*),").
		ReplaceAllString(str, "$1    int    `${2}type:$3$4\"`")
	str = regexp.MustCompile("`([a-z_0-9A-Z]+)` (.*?;)(decimal|dec|numeric|fixed|float|double|double precision|real)(.*),").
		ReplaceAllString(str, "$1    float64    `${2}type:$3$4\"`")
	str = regexp.MustCompile("`([a-z_0-9A-Z]+)` (.*?;)(varchar|char|year)(.*),").
		ReplaceAllString(str, "$1    string    `${2}type:$3$4\"`")
	str = regexp.MustCompile("`([a-z_0-9A-Z]+)` (.*?;)(text|tinytext|mediumtext|longtext|enum)(.*),").
		ReplaceAllString(str, "$1    string    `${2}type:$3$4\"`")
	str = regexp.MustCompile("`([a-z_0-9A-Z]+)` (.*?;)(timestamp|datetime)(.*),").
		ReplaceAllString(str, "$1    *time.Time    `${2}type:$3$4\"`")

	// 删除不知道怎么转换的属性
	str = strings.ReplaceAll(str, ` on update current_timestamp`, "")
	str = regexp.MustCompile("\\s*(primary|unique)? key .*(,)?").ReplaceAllString(str, "")

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
			exportText = []byte(output)
			shellEcho(output, "ok")
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
