package fileutils

import (
	"bufio"
	"os"
	"path/filepath"

	"github.com/golang/glog"
)

// creates a file or a directory only if it does not already exist.
func WriteFile(path string, data []byte) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			//创建所有的父目录
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				check(err)
			}
		}
	}
	f, err := os.Create(path)
	defer f.Close()
	check(err)
	bufwriter := bufio.NewWriter(f)
	bufwriter.Write(data)
	bufwriter.Flush()
}

func check(e error) {
	if e != nil {
		glog.Fatalf("文件处理异常:", e)
		panic(e)
	}
}
