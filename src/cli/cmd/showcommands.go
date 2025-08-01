package cmd

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
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

	"github.com/aerospike/aerolab/pkg/utils/contextio"
	"github.com/aerospike/aerolab/pkg/utils/shutdown"
	"github.com/gabriel-vasile/mimetype"
)

type ShowCommand string

const (
	ShowCommandSysinfo    ShowCommand = "showsysinfo"
	ShowCommandConf       ShowCommand = "showconf"
	ShowCommandInterrupts ShowCommand = "showinterrupts"
)

func ShowcommandsBusybox() {
	_, command := path.Split(os.Args[0])
	showCommandsWait := new(sync.WaitGroup)
	fileNameEnd, out, interruptsErr, err := getFileNameEnd(ShowCommand(command), showCommandsWait)
	if err != nil {
		shutdown.WaitJobs()
		log.Fatal(err)
	}

	if len(os.Args) != 2 {
		fmt.Printf("Usage: %s collect_info_file.tgz\n", os.Args[0])
		shutdown.WaitJobs()
		os.Exit(1)
	}
	err = showCommands(os.Args[1], fileNameEnd, out, showCommandsWait, interruptsErr)
	if err != nil {
		shutdown.WaitJobs()
		log.Fatal(err)
	}
}

func ShowCommands(fileName string, command ShowCommand) error {
	showCommandsWait := new(sync.WaitGroup)
	fileNameEnd, out, interruptsErr, err := getFileNameEnd(command, showCommandsWait)
	if err != nil {
		return err
	}
	return showCommands(fileName, fileNameEnd, out, showCommandsWait, interruptsErr)
}

func showCommands(collectName string, fileNameEnd string, out io.Writer, showCommandsWait *sync.WaitGroup, interruptsErr chan error) error {
	zipContents, err := handleCollectInfo(collectName, fileNameEnd, out, showCommandsWait, interruptsErr)
	if err != nil {
		return err
	}
	if len(zipContents) > 0 {
		zipfd, err := zip.NewReader(bytes.NewReader(zipContents), int64(len(zipContents)))
		if err != nil {
			return fmt.Errorf("error opening zip for reading: %s", err)
		}
		for _, file := range zipfd.File {
			if strings.HasSuffix(file.Name, fileNameEnd) {
				zipfdfile, err := file.Open()
				if err != nil {
					return fmt.Errorf("error opening zip contents for reading: %s", err)
				}
				defer zipfdfile.Close()
				_, err = io.Copy(out, zipfdfile)
				if err != nil && err != io.EOF && err != context.Canceled {
					return fmt.Errorf("error while reading zip file contents: %s", err)
				}
				showCommandsWait.Wait()
				select {
				case err := <-interruptsErr:
					return err
				default:
				}
				fmt.Println("\nEND")
				return nil
			}
		}
		return errors.New("found zip in collectinfo, but it did not contain the named file")
	}
	return nil
}

func handleCollectInfo(collectName string, fileNameEnd string, out io.Writer, showCommandsWait *sync.WaitGroup, interruptsErr chan error) ([]byte, error) {
	fileNameEndZip := fileNameEnd + ".zip"
	if _, err := os.Stat(collectName); err != nil {
		return nil, fmt.Errorf("could not access file: %s", err)
	}

	fd, err := os.Open(collectName)
	if err != nil {
		return nil, fmt.Errorf("could not open file for reading: %s", err)
	}
	defer fd.Close()
	var tarReader io.Reader = fd

	ntype, err := mimetype.DetectFile(collectName)
	if err != nil || ntype.Is("application/gzip") {
		fdgzip, err := gzip.NewReader(fd)
		if err != nil {
			return nil, fmt.Errorf("could not open file as gzip: %s", err)
		}
		defer fdgzip.Close()
		tarReader = fdgzip
	}

	tr := tar.NewReader(tarReader)

	ret := []byte{}

	for {
		header, err := tr.Next()

		switch {
		case err == io.EOF:
			return nil, fmt.Errorf("requested file not found in archive")
		case err != nil:
			return nil, fmt.Errorf("error reading tar archive: %s", err)
		case header == nil:
			continue
		}

		if header.Typeflag != tar.TypeReg {
			continue
		}

		if strings.HasSuffix(header.Name, fileNameEnd) {
			_, err = io.Copy(out, tr)
			if err != nil && err != io.EOF && err != context.Canceled {
				return nil, fmt.Errorf("error while reading file contents: %s", err)
			}
			showCommandsWait.Wait()
			select {
			case err := <-interruptsErr:
				return nil, err
			default:
			}
			fmt.Println("\nEND")
			break
		}
		if strings.HasSuffix(header.Name, fileNameEndZip) {
			ret, err = io.ReadAll(tr)
			if err != nil && err != io.EOF {
				return nil, fmt.Errorf("error while reading zip file contents: %s", err)
			}
			break
		}

	}
	return ret, nil
}

func getFileNameEnd(command ShowCommand, showCommandsWait *sync.WaitGroup) (fileNameEnd string, out io.Writer, interruptsErr chan error, err error) {
	interruptsErr = make(chan error)
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
			return "", nil, nil, err
		}
		ctx, cancel := context.WithCancel(context.Background())
		out = contextio.NewWriter(ctx, pw)
		showCommandsWait.Add(1)
		go func() {
			err := processInterrupts(cancel, pr, showCommandsWait)
			if err != nil {
				interruptsErr <- err
			}
		}()
	default:
		return "", nil, nil, fmt.Errorf("command %s not understood", command)
	}
	return fileNameEnd, out, interruptsErr, nil
}

func processInterrupts(cancel context.CancelFunc, in io.ReadCloser, showCommandsWait *sync.WaitGroup) error {
	rd := bufio.NewScanner(in)
	header := false
	ntype := false
	abort := false
	firstline := true
	cpucount := 0
	usemap := [][]string{}
	for rd.Scan() {
		if err := rd.Err(); err != nil {
			return fmt.Errorf("error reading interrupts: %s", err)
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
				showCommandsWait.Done()
			}
			header = true
		} else if header {
			if line == "['cat /proc/interrupts']" || line == "cat /proc/interrupts" {
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
	return nil
}
