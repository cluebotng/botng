package helpers

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"net"
	"strings"
)

var namespacesByName map[string]int64

func init() {
	namespacesByName = map[string]int64{
		"special":           -1,
		"media":             -2,
		"main":              0,
		"talk":              1,
		"user":              2,
		"user talk":         3,
		"wikipedia":         4,
		"wikipedia talk":    5,
		"file":              6,
		"file talk":         7,
		"mediawiki":         8,
		"mediawiki talk":    9,
		"template":          10,
		"template talk":     11,
		"help":              12,
		"help talk":         13,
		"category":          14,
		"category talk":     15,
		"portal":            100,
		"portal talk":       101,
		"draft":             118,
		"education program": 446,
		"timedtext":         710,
		"module":            828,
		"gadget":            2300,
		"gadget definition": 2302,
	}
}

func PageTitleWithoutNamespace(title string) string {
	for ns := range namespacesByName {
		pfx := strings.ToLower(fmt.Sprintf("%s:", ns))
		if strings.HasPrefix(strings.ToLower(title), pfx) {
			title = strings.Join(strings.Split(title, ":")[1:], ":")
			break
		}
	}
	return strings.ReplaceAll(title, " ", "_")
}

func PageTitle(namespace, title string) string {
	if namespace == "Main" {
		return title
	}
	return fmt.Sprintf("%v:%v", namespace, title)
}

func NameSpaceNameToId(ns string) int64 {
	return namespacesByName[strings.ToLower(ns)]
}

func FormatPlusOrMinus(value int64) string {
	if value < 0 {
		return fmt.Sprintf("%d", value)
	}
	return fmt.Sprintf("+%d", value)
}

func StringItemInSlice(item string, slice []string) bool {
	for _, i := range slice {
		if i == item {
			return true
		}
	}
	return false
}

func AivUserVandalType(user string) string {
	if net.ParseIP(user) != nil {
		logrus.Debugf("Parsed '%v' as IPvandal", user)
		return "IPvandal"
	}
	logrus.Debugf("Parsed '%v' as Vandal", user)
	return "Vandal"
}
