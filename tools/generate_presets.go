package main

import (
	"fmt"
	"go/format"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const chartsDir = "charts"
const outputFile = "pkg/generated/presets.go"

func main() {
	filesMap := make(map[string]string)
	var chartNames []string

	err := filepath.Walk(chartsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.Mode()&os.ModeSymlink != 0 {
			return filepath.SkipDir
		}

		if info.IsDir() && path != chartsDir && filepath.Dir(path) == chartsDir {
			fmt.Printf("read: %s\n", path)
			if info.Name() != "talm" {
				chartNames = append(chartNames, info.Name())
			}
			return nil
		}

		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(chartsDir, path)
		if err != nil {
			return err
		}
		content, err := ioutil.ReadFile(path)
		if err != nil {
			return err
		}

		if filepath.Base(path) == "Chart.yaml" {
			regex := regexp.MustCompile(`(name|version): \S+`)
			content = regex.ReplaceAll(content, []byte("$1: %s"))
		}

		filesMap[relPath] = string(content)

		return nil
	})

	if err != nil {
		log.Fatal(err)
	}

	generateCode(filesMap, chartNames)
}

func generateCode(filesMap map[string]string, chartNames []string) {
	var builder strings.Builder

	// Sort keys
	keys := make([]string, 0, len(filesMap))
	for k := range filesMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	builder.WriteString("package generated\n\n")

	builder.WriteString("var PresetFiles = map[string]string{\n")
	for _, key := range keys {
		content := filesMap[key]
		escapedContent := strings.ReplaceAll(content, "`", "` + \"`\" + `")
		builder.WriteString(`    "` + key + `": ` + "`" + escapedContent + "`,\n")
	}
	builder.WriteString("}\n\n")

	builder.WriteString("var AvailablePresets = []string{\n")
	// Generic chart should be first
	for _, name := range chartNames {
		if name == "generic" {
			builder.WriteString(`    "` + name + `",` + "\n")
		}
	}
	for _, name := range chartNames {
		if name != "generic" {
			builder.WriteString(`    "` + name + `",` + "\n")
		}
	}
	builder.WriteString("}\n")

	formattedSource, err := format.Source([]byte(builder.String()))
	if err != nil {
		log.Fatal(err)
	}

	err = ioutil.WriteFile(outputFile, []byte(formattedSource), 0644)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("written: %s\n", outputFile)
}
