package log

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/alecthomas/units"
	"github.com/briandowns/spinner"
	"github.com/dustin/go-humanize"
	"github.com/sirupsen/logrus"
)

const (
	blockSize = 512
)

type Spinner struct {
	Spinner                                         *spinner.Spinner
	Ticker                                          *time.Ticker
	ProgressMessage, SuccessMessage, FailureMessage string
	FileToMonitor, DirToMonitor                     string
}

func NewSpinner(progressMessage, successMessage, failureMessage string) *Spinner {
	// Create and start spinner with message
	s := spinner.New(spinner.CharSets[9], 100*time.Millisecond, spinner.WithWriter(os.Stderr))
	s.Suffix = fmt.Sprintf(" %s", progressMessage)
	if err := s.Color("blue"); err != nil {
		logrus.Fatalln(err)
	}
	s.Start()

	wrapper := &Spinner{
		Spinner:         s,
		ProgressMessage: progressMessage,
		SuccessMessage:  successMessage,
		FailureMessage:  failureMessage,
	}

	wrapper.Ticker = time.NewTicker(1 * time.Second)
	go func() {
		for range wrapper.Ticker.C {
			size, err := getProgressSize(wrapper)
			if err != nil || size < uint64(units.MiB) {
				continue
			}

			s.Suffix = fmt.Sprintf(" %s (%s)", progressMessage, humanize.Bytes(size))
		}
	}()

	return wrapper
}

func getProgressSize(spinner *Spinner) (uint64, error) {
	var size uint64

	if spinner.FileToMonitor != "" {
		var stat syscall.Stat_t
		err := syscall.Stat(spinner.FileToMonitor, &stat)
		if err != nil {
			return 0, err
		}
		if strings.Contains(spinner.FileToMonitor, ".raw") {
			// Get actual size of raw sparse file
			size = uint64(stat.Blocks * blockSize)
		} else {
			size = uint64(stat.Size)
		}
	}

	if spinner.DirToMonitor != "" {
		if _, err := os.Stat(spinner.DirToMonitor); os.IsNotExist(err) {
			return 0, err
		}
		if err := filepath.Walk(spinner.DirToMonitor, func(path string, info os.FileInfo, err error) error {
			if info != nil && !info.IsDir() {
				size += uint64(info.Size())
			}
			return nil
		}); err != nil {
			return 0, err
		}
	}

	return size, nil
}

func StopSpinner(spinner *Spinner, err error) error {
	if spinner == nil {
		return err
	}
	spinner.Spinner.Stop()
	spinner.Ticker.Stop()
	if err != nil {
		logrus.Error(spinner.FailureMessage)
	} else {
		logrus.Info(spinner.SuccessMessage)
	}
	return err
}
