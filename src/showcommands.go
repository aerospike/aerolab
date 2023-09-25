package main

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/aerospike/aerolab/contextio"
)

var wait = new(sync.WaitGroup)

func showcommands() {
	fileNameEnd, out := getFileNameEnd(os.Args[0])

	if len(os.Args) != 2 {
		fmt.Printf("Usage: %s collect_info_file.tgz\n", os.Args[0])
		os.Exit(1)
	}

	collectName := os.Args[1]
	zipContents := handleCollectInfo(collectName, fileNameEnd, out)
	if len(zipContents) > 0 {
		zipfd, err := zip.NewReader(bytes.NewReader(zipContents), int64(len(zipContents)))
		if err != nil {
			log.Fatalf("Error opening zip for reading: %s", err)
		}
		for _, file := range zipfd.File {
			if strings.HasSuffix(file.Name, fileNameEnd) {
				zipfdfile, err := file.Open()
				if err != nil {
					log.Fatalf("Error opening zip contents for reading: %s", err)
				}
				defer zipfdfile.Close()
				_, err = io.Copy(out, zipfdfile)
				if err != nil && err != io.EOF && err != context.Canceled {
					log.Fatalf("Error while reading zip file contents: %s", err)
				}
				wait.Wait()
				fmt.Println("\nEND")
				return
			}
		}
		log.Fatal("Found zip in collectinfo, but it did not contain the named file!")
	}
}

func handleCollectInfo(collectName string, fileNameEnd string, out io.Writer) []byte {
	fileNameEndZip := fileNameEnd + ".zip"
	if _, err := os.Stat(collectName); err != nil {
		log.Fatalf("Could not access file: %s", err)
	}

	fd, err := os.Open(collectName)
	if err != nil {
		log.Fatalf("Could not open file for reading: %s", err)
	}
	defer fd.Close()

	fdgzip, err := gzip.NewReader(fd)
	if err != nil {
		log.Fatalf("Could not open file as gzip: %s", err)
	}
	defer fdgzip.Close()

	tr := tar.NewReader(fdgzip)

	ret := []byte{}

	for {
		header, err := tr.Next()

		switch {
		case err == io.EOF:
			log.Fatal("Requested file not found in archive")
		case err != nil:
			log.Fatalf("Error reading tar archive: %s", err)
		case header == nil:
			continue
		}

		if header.Typeflag != tar.TypeReg {
			continue
		}

		if strings.HasSuffix(header.Name, fileNameEnd) {
			_, err = io.Copy(out, tr)
			if err != nil && err != io.EOF && err != context.Canceled {
				log.Fatalf("Error while reading file contents: %s", err)
			}
			wait.Wait()
			fmt.Println("\nEND")
			break
		}
		if strings.HasSuffix(header.Name, fileNameEndZip) {
			ret, err = io.ReadAll(tr)
			if err != nil && err != io.EOF {
				log.Fatalf("Error while reading zip file contents: %s", err)
			}
			break
		}

	}
	return ret
}

func getFileNameEnd(args string) (fileNameEnd string, out io.Writer) {
	_, command := path.Split(os.Args[0])
	switch command {
	case "showsysinfo":
		fileNameEnd = "_sysinfo.log"
		out = os.Stdout
	case "showconf":
		fileNameEnd = "aerospike.conf"
		out = os.Stdout
	case "showinterrupts":
		fileNameEnd = "_sysinfo.log"
		pr, pw, err := os.Pipe()
		if err != nil {
			log.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		out = contextio.NewWriter(ctx, pw)
		wait.Add(1)
		go processInterrupts(ctx, cancel, pr)
	default:
		log.Fatalf("Command %s not understood", command)
	}
	return
}

func processInterrupts(ctx context.Context, cancel context.CancelFunc, in io.ReadCloser) {
	rd := bufio.NewScanner(in)
	header := false
	ntype := false
	abort := false
	firstline := true
	cpucount := 0
	usemap := [][]string{}
	for rd.Scan() {
		if err := rd.Err(); err != nil {
			log.Fatal(err)
		}
		if abort {
			continue
		}
		line := strings.TrimRight(rd.Text(), "\r\n\t ")
		if line == "" {
			continue
		}
		if strings.Trim(line, "\r\n\t ") == "====ASCOLLECTINFO====" {
			if ntype {
				firstColumnSize := 4
				for _, n := range usemap {
					if len(n[0]) > firstColumnSize {
						firstColumnSize = len(n[0])
					}
				}
				formatter := "%" + strconv.Itoa(firstColumnSize) + "s %s"
				tensHeader := ""
				for i := 0; i < cpucount; i++ {
					j := i % 10
					switch j {
					case 0:
						tensHeader = tensHeader + strconv.Itoa(int(math.Floor(float64(i)/10)))
					case 9:
						tensHeader = tensHeader + "┐"
					default:
						tensHeader = tensHeader + "─"
					}
				}
				fmt.Printf(formatter+"\n", "   :", tensHeader)
				cpustring := ""
				for i := 0; i < cpucount; i++ {
					j := i % 10
					cpustring = cpustring + strconv.Itoa(j)
				}
				fmt.Printf(formatter+"\n", "CPU:", cpustring)
				formatter = formatter + " %s"
				for _, n := range usemap {
					curCpuCount := cpucount + 1
					if len(n) < cpucount+1 {
						curCpuCount = len(n)
					}
					fmt.Printf(formatter+"\n", n[0], strings.Join(n[1:curCpuCount], ""), strings.Join(n[curCpuCount:], " "))
				}
				cancel()
				abort = true
				wait.Done()
			}
			header = true
		} else if header {
			if line == "['cat /proc/interrupts']" {
				ntype = true
			}
			header = false
		} else if ntype {
			if firstline && strings.HasPrefix(strings.TrimLeft(line, " \t"), "CPU0") {
				firstline = false
				cpucount = len(strings.Fields(line))
			} else {
				inuse := strings.Fields(line)
				ilist := []int{}
				totals := 0
				for i := range inuse {
					if i == 0 {
						continue
					}
					if i > cpucount {
						break
					}
					if inuse[i] == "0" {
						inuse[i] = "."
					} else {
						ilist = append(ilist, i)
						total, _ := strconv.Atoi(inuse[i])
						totals += total
					}
				}
				stepping := []int{
					totals / cpucount,
					totals / 100,
					totals / 20,
					totals / 10,
					totals / 5,
				}
				sort.Ints(stepping)
				chars := []string{"▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"}
				for _, i := range ilist {
					total, _ := strconv.Atoi(inuse[i])
					if total > stepping[len(stepping)-1] {
						inuse[i] = string(chars[7])
					} else {
						for si, s := range stepping {
							if total <= s {
								inuse[i] = string(chars[si])
								break
							}
						}
					}
				}
				usemap = append(usemap, inuse)
			}
		}
	}
}
