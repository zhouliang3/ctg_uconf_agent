package fileutils

import (
	"bufio"
	"io"
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
				checkErr(err)
			}
		}
	}
	f, err := os.Create(path)
	defer f.Close()
	checkErr(err)
	bufwriter := bufio.NewWriter(f)
	bufwriter.Write(data)
	bufwriter.Flush()
}

func CopyFile(dstName, srcName string) (written int64, err error) {
	if _, err := os.Stat(dstName); err != nil {
		if os.IsNotExist(err) {
			//创建所有的父目录
			if err := os.MkdirAll(filepath.Dir(dstName), 0755); err != nil {
				checkErr(err)
			}
		}
	}
	src, err := os.Open(srcName)
	if err != nil {
		return
	}
	defer src.Close()
	dst, err := os.OpenFile(dstName, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return
	}
	defer dst.Close()
	return io.Copy(dst, src)
}
func checkErr(e error) {
	if e != nil {
		glog.Fatalf("文件处理异常:", e)
		panic(e)
	}
}
